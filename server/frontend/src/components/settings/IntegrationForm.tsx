import { useState, useMemo, useEffect, useCallback, useRef } from 'react'
import { useQuery } from '@tanstack/react-query'
import {
  listPlugins,
  createIntegration,
  checkPluginConfig,
  importOAuthCallback,
  isOAuthRequired,
  updateIntegration,
  DuplicateIntegrationError,
  type Integration,
  type OAuthRequiredResponse,
  type PluginInfo,
} from '@/api/client'
import { FolderBrowser } from './FolderBrowser'
import { useSSE } from '@/hooks/useSSE'
import { useDateTimeFormat } from '@/hooks/useDateTimeFormat'
import {
  isFilesystemSourcePlugin,
  normalizeFilesystemIncludePaths,
  pluginLabel,
  parsePluginConfigSchema,
  CAPABILITY_META,
  CAPABILITY_ORDER,
} from '@/lib/gameUtils'
import { Dialog } from '@/components/ui/dialog'
import { Input } from '@/components/ui/input'

import { Button } from '@/components/ui/button'
import { Badge } from '@/components/ui/badge'
import { PluginIcon } from './PluginIcon'
import { ConfigFieldsRenderer } from './ConfigFieldsRenderer'
import { OAuthCallbackPanel } from './OAuthCallbackPanel'
import { ArrowLeft, Check } from 'lucide-react'

// ═══════════════════════════════════════════════════════════════════════════
// Add Integration Wizard
// ═══════════════════════════════════════════════════════════════════════════

type WizardStep = 'category' | 'plugin' | 'config' | 'label' | 'oauth' | 'browse'
type OAuthPurpose = 'create' | 'verify_config'

interface AddIntegrationWizardProps {
  onClose: () => void
  onSaved: () => void
}

export function AddIntegrationWizard({ onClose, onSaved }: AddIntegrationWizardProps) {
  const { data: plugins = [] } = useQuery({ queryKey: ['plugins'], queryFn: listPlugins })

  // Wizard state.
  const [step, setStep] = useState<WizardStep>('category')
  const [selectedCategory, setSelectedCategory] = useState<string | null>(null)
  const [selectedPluginId, setSelectedPluginId] = useState<string | null>(null)
  const [configFields, setConfigFields] = useState<Record<string, unknown>>({})
  const [label, setLabel] = useState('')
  const [integrationType, setIntegrationType] = useState('')
  const [saving, setSaving] = useState(false)
  const [error, setError] = useState('')

  // OAuth flow state.
  const [oauthState, setOauthState] = useState<string | null>(null)
  const [oauthResponse, setOauthResponse] = useState<OAuthRequiredResponse | null>(null)
  const [oauthError, setOauthError] = useState<string | null>(null)
  const [oauthPurpose, setOauthPurpose] = useState<OAuthPurpose>('create')
  const [configVerified, setConfigVerified] = useState(false)
  const oauthWindowRef = useRef<Window | null>(null)
  const { subscribe } = useSSE()

  // Browse state (for plugins that provide source.browse).
  const [createdIntegrationId, setCreatedIntegrationId] = useState<string | null>(null)

  // Derived data.
  const selectedPlugin = plugins.find((p) => p.plugin_id === selectedPluginId)
  const schema = useMemo(
    () => parsePluginConfigSchema(selectedPlugin?.config as Record<string, unknown> | undefined),
    [selectedPlugin],
  )
  const hasConfig = schema.length > 0

  // Group plugins by capability for step 1.
  const capabilityGroups = useMemo(() => {
    const groups = new Map<string, PluginInfo[]>()
    for (const plugin of plugins) {
      const cap = plugin.capabilities?.[0] ?? 'other'
      if (!groups.has(cap)) groups.set(cap, [])
      groups.get(cap)!.push(plugin)
    }
    return groups
  }, [plugins])

  // Plugins filtered by selected category.
  const filteredPlugins = selectedCategory ? (capabilityGroups.get(selectedCategory) ?? []) : []

  // Step navigation.
  const goBack = () => {
    setError('')
    if (step === 'plugin') setStep('category')
    else if (step === 'config') setStep('plugin')
    else if (step === 'label') setStep(hasConfig ? 'config' : 'plugin')
  }

  const selectCategory = (cap: string) => {
    setSelectedCategory(cap)
    setStep('plugin')
  }

  const selectPlugin = (plugin: PluginInfo) => {
    setSelectedPluginId(plugin.plugin_id)
    setConfigFields({})
    setConfigVerified(false)
    setError('')

    // Pre-fill defaults from schema.
    const parsed = parsePluginConfigSchema(plugin.config as Record<string, unknown> | undefined)
    const defaults: Record<string, unknown> = {}
    for (const { key, field } of parsed) {
      if (field.default != null) defaults[key] = field.default
    }
    if (isFilesystemSourcePlugin(plugin.plugin_id)) {
      defaults.include_paths = normalizeFilesystemIncludePaths(plugin.plugin_id, {})
    }
    setConfigFields(defaults)

    // Auto-derive integration type.
    const caps = plugin.capabilities ?? []
    setIntegrationType(caps[0] ?? '')

    // Auto-suggest label.
    setLabel(pluginLabel(plugin.plugin_id))

    // Skip config step if no config needed.
    if (parsed.length === 0) {
      setStep('label')
    } else {
      setStep('config')
    }
  }

  const advanceToLabel = () => {
    setError('')
    setStep('label')
  }

  // Build a typed config object from the form fields.
  const buildConfig = useCallback(() => {
    const config: Record<string, unknown> = {}
    for (const { key, field } of schema) {
      const raw = configFields[key]
      if (field.type === 'boolean') {
        config[key] = raw === true || raw === 'true' || raw === '1'
      } else if (field.type === 'number' || field.type === 'integer') {
        config[key] = typeof raw === 'number' ? raw : Number(raw ?? 0)
      } else if (field.type === 'array') {
        config[key] = Array.isArray(raw) ? raw : []
      } else {
        config[key] = typeof raw === 'string' ? raw : (raw ?? '')
      }
    }
    if (selectedPlugin && isFilesystemSourcePlugin(selectedPlugin.plugin_id)) {
      delete config.path
      delete config.root_path
      delete config.exclude_paths
    }
    return config
  }, [schema, configFields, selectedPlugin])

  // Check if the selected plugin supports folder browsing.
  const supportsBrowse = selectedPlugin?.provides?.includes('source.browse') ?? false
  const supportsOAuth = selectedPlugin?.provides?.includes('auth.oauth.callback') ?? false
  const requiresVerifiedConfigBeforeBrowse = supportsOAuth && supportsBrowse && hasConfig
  const browseDisabledReason = 'Connect this integration first so MGA can browse remote folders.'

  const mergeDraftConfigUpdates = useCallback((updates?: Record<string, unknown>) => {
    if (!updates || Object.keys(updates).length === 0) return
    setConfigFields((prev) => ({ ...prev, ...updates }))
  }, [])

  // After successful integration creation, either go to browse step or finish.
  const finishCreate = useCallback((integrationId: string) => {
    if (supportsBrowse && !hasConfig && selectedPlugin?.capabilities?.includes('source')) {
      setCreatedIntegrationId(integrationId)
      setStep('browse')
    } else {
      onSaved()
    }
  }, [hasConfig, selectedPlugin, supportsBrowse, onSaved])

  const handleCreate = async () => {
    if (!selectedPlugin || !label || !integrationType) return
    setError('')
    setSaving(true)
    const authWindow = selectedPlugin.provides?.includes('auth.oauth.callback') ? window.open('', '_blank') : null
    oauthWindowRef.current = authWindow

    try {
      const result = await createIntegration({
        plugin_id: selectedPlugin.plugin_id,
        label,
        integration_type: integrationType,
        config: buildConfig(),
      })

      // OAuth consent required — open the authorize URL in a new tab.
      if (isOAuthRequired(result)) {
        setOauthPurpose('create')
        setOauthState(result.state)
        setOauthResponse(result)
        setOauthError(null)
        setStep('oauth')
        if (authWindow) {
          authWindow.location.href = result.authorize_url
        } else {
          setOauthError('Browser blocked the sign-in popup. Allow popups for MGA and try again.')
        }
        return
      }

      authWindow?.close()
      finishCreate(result.id)
    } catch (err) {
      authWindow?.close()
      if (err instanceof DuplicateIntegrationError) {
        setError(err.message)
      } else {
        setError(err instanceof Error ? err.message : 'Failed to create integration')
      }
    } finally {
      setSaving(false)
    }
  }

  const handleVerifyConfig = useCallback(async () => {
    if (!selectedPlugin) return
    setError('')
    setOauthError(null)
    setSaving(true)
    const authWindow = supportsOAuth ? window.open('', '_blank') : null
    oauthWindowRef.current = authWindow

    try {
      const result = await checkPluginConfig(selectedPlugin.plugin_id, buildConfig())
      if (isOAuthRequired(result)) {
        setOauthPurpose('verify_config')
        setOauthState(result.state)
        setOauthResponse(result)
        setOauthError(null)
        setStep('oauth')
        if (authWindow) {
          authWindow.location.href = result.authorize_url
        } else {
          setOauthError('Browser blocked the sign-in popup. Allow popups for MGA and try again.')
        }
        return
      }

      authWindow?.close()
      if (result.status && result.status !== 'ok') {
        setError(result.message || result.status)
        return
      }
      mergeDraftConfigUpdates(result.config_updates)
      setConfigVerified(true)
    } catch (err) {
      authWindow?.close()
      setError(err instanceof Error ? err.message : 'Failed to verify integration')
    } finally {
      setSaving(false)
    }
  }, [buildConfig, mergeDraftConfigUpdates, selectedPlugin, supportsOAuth])

  const retryVerifyConfigAfterOAuth = useCallback(async () => {
    if (!selectedPlugin) return
    setSaving(true)
    setOauthError(null)
    setError('')

    try {
      const result = await checkPluginConfig(selectedPlugin.plugin_id, buildConfig())
      if (isOAuthRequired(result)) {
        setOauthError('Authentication incomplete. Please try again.')
        return
      }
      if (result.status && result.status !== 'ok') {
        setOauthError(result.message || result.status)
        return
      }
      mergeDraftConfigUpdates(result.config_updates)
      setConfigVerified(true)
      oauthWindowRef.current?.close()
      oauthWindowRef.current = null
      setOauthState(null)
      setOauthResponse(null)
      setOauthError(null)
      setStep('config')
    } catch (err) {
      setOauthError(err instanceof Error ? err.message : 'Failed to verify integration')
    } finally {
      setSaving(false)
    }
  }, [buildConfig, mergeDraftConfigUpdates, selectedPlugin])

  // After OAuth completes, retry creating the integration (tokens are now valid).
  const retryCreateAfterOAuth = useCallback(async () => {
    if (!selectedPlugin || !label || !integrationType) return
    setSaving(true)
    setOauthError(null)

    try {
      const result = await createIntegration({
        plugin_id: selectedPlugin.plugin_id,
        label,
        integration_type: integrationType,
        config: buildConfig(),
      })

      if (isOAuthRequired(result)) {
        setOauthError('Authentication incomplete. Please try again.')
        return
      }

      oauthWindowRef.current?.close()
      oauthWindowRef.current = null
      setOauthResponse(null)
      finishCreate(result.id)
    } catch (err) {
      setOauthError(err instanceof Error ? err.message : 'Failed to create integration')
    } finally {
      setSaving(false)
    }
  }, [selectedPlugin, label, integrationType, buildConfig, finishCreate])

  // Listen for OAuth completion/error SSE events while on the oauth step.
  useEffect(() => {
    if (step !== 'oauth' || !oauthState) return

    const unsubComplete = subscribe('oauth_complete', (data: unknown) => {
      const d = data as { state?: string }
      if (d.state === oauthState) {
        if (oauthPurpose === 'verify_config') {
          void retryVerifyConfigAfterOAuth()
        } else {
          void retryCreateAfterOAuth()
        }
      }
    })

    const unsubError = subscribe('oauth_error', (data: unknown) => {
      const d = data as { state?: string; error?: string }
      if (d.state === oauthState) {
        setOauthError(d.error ?? 'Authentication failed')
      }
    })

    return () => { unsubComplete(); unsubError() }
  }, [step, oauthState, oauthPurpose, subscribe, retryCreateAfterOAuth, retryVerifyConfigAfterOAuth])

  const reopenOAuthWindow = useCallback(() => {
    if (!oauthResponse) return
    const authWindow = window.open('', '_blank')
    oauthWindowRef.current = authWindow
    if (authWindow) {
      authWindow.location.href = oauthResponse.authorize_url
      setOauthError(null)
    } else {
      setOauthError('Browser blocked the sign-in popup. Allow popups for MGA and try again.')
    }
  }, [oauthResponse])

  const submitPastedOAuthCallback = useCallback(async (callbackUrl: string) => {
    if (!selectedPlugin || !oauthResponse) return
    setSaving(true)
    setOauthError(null)
    try {
      await importOAuthCallback(selectedPlugin.plugin_id, callbackUrl)
    } catch (err) {
      setOauthError(err instanceof Error ? err.message : 'Failed to import callback URL')
    } finally {
      setSaving(false)
    }
  }, [oauthResponse, selectedPlugin])

  // Step titles for the dialog.
  const stepTitles: Record<WizardStep, string> = {
    category: 'Add Integration — Choose Type',
    plugin: 'Add Integration — Choose Plugin',
    config: 'Add Integration — Configure',
    label: 'Add Integration — Finish',
    oauth: 'Add Integration — Sign In',
    browse: 'Add Integration — Select Folder',
  }

  return (
    <Dialog open onClose={onClose} title={stepTitles[step]} className="max-w-2xl">
      {/* Back button (not shown on first step, during OAuth, or while browsing) */}
      {step !== 'category' && step !== 'oauth' && step !== 'browse' && (
        <Button type="button" variant="outline" size="sm" onClick={goBack} className="mb-4">
          <ArrowLeft size={14} /> Back
        </Button>
      )}

      {/* Step 1: Choose category */}
      {step === 'category' && (
        <div className="grid grid-cols-2 gap-3">
          {CAPABILITY_ORDER.filter((cap) => capabilityGroups.has(cap)).map((cap) => {
            const meta = CAPABILITY_META[cap]
            const count = capabilityGroups.get(cap)!.length
            return (
              <button
                key={cap}
                type="button"
                onClick={() => selectCategory(cap)}
                className="flex flex-col items-center gap-2 p-6 border border-mga-border rounded-mga bg-mga-surface hover:border-mga-accent hover:bg-mga-elevated transition-all text-center"
              >
                <PluginIcon capability={cap} size={32} className="text-mga-accent" />
                <span className="font-medium text-mga-text">{meta?.label ?? cap}</span>
                <span className="text-xs text-mga-muted">
                  {count} plugin{count !== 1 ? 's' : ''} available
                </span>
              </button>
            )
          })}
        </div>
      )}

      {/* Step 2: Choose plugin */}
      {step === 'plugin' && (
        <div className="space-y-2">
          {filteredPlugins.map((plugin) => {
            const pSchema = parsePluginConfigSchema(plugin.config as Record<string, unknown> | undefined)
            return (
              <button
                key={plugin.plugin_id}
                type="button"
                onClick={() => selectPlugin(plugin)}
                className="w-full flex items-center gap-3 p-4 border border-mga-border rounded-mga bg-mga-surface hover:border-mga-accent hover:bg-mga-elevated transition-all text-left"
              >
                <PluginIcon pluginId={plugin.plugin_id} size={24} className="text-mga-accent shrink-0" />
                <div className="flex-1 min-w-0">
                  <div className="flex items-center gap-2">
                    <span className="font-medium text-mga-text">{pluginLabel(plugin.plugin_id)}</span>
                    <Badge variant="muted">v{plugin.plugin_version}</Badge>
                  </div>
                  <p className="text-xs text-mga-muted mt-0.5">
                    {pSchema.length === 0
                      ? (plugin.provides?.includes('auth.oauth.callback') ? 'Browser sign-in required' : 'No configuration needed')
                      : `${pSchema.length} config field${pSchema.length !== 1 ? 's' : ''}`}
                  </p>
                </div>
                {plugin.capabilities.map((cap) => (
                  <Badge key={cap} variant="accent">{cap}</Badge>
                ))}
              </button>
            )
          })}
        </div>
      )}

      {/* Step 3: Configure */}
      {step === 'config' && selectedPlugin && (
        <div className="space-y-4">
          <div className="flex items-center gap-2 mb-2">
            <PluginIcon pluginId={selectedPlugin.plugin_id} size={18} className="text-mga-accent" />
            <span className="text-sm font-medium text-mga-text">
              {pluginLabel(selectedPlugin.plugin_id)}
            </span>
          </div>

          {requiresVerifiedConfigBeforeBrowse && !configVerified ? (
            <div className="rounded-mga border border-mga-border bg-mga-surface/70 p-4">
              <p className="text-sm font-medium text-mga-text">Connect before choosing folders</p>
              <p className="mt-1 text-sm text-mga-muted">
                MGA needs browser sign-in before it can browse your Google Drive folders. After the connection is verified, this dialog will unlock the shared folder browser so you can choose an existing save folder or create a new path.
              </p>
            </div>
          ) : (
            <ConfigFieldsRenderer
              schema={schema}
              values={configFields}
              onChange={(key, value) => setConfigFields((prev) => ({ ...prev, [key]: value }))}
              browsePluginId={supportsBrowse ? selectedPlugin.plugin_id : null}
              browseDisabled={requiresVerifiedConfigBeforeBrowse && !configVerified}
              browseDisabledReason={browseDisabledReason}
            />
          )}

          {error && <p className="text-sm text-red-400">{error}</p>}

          <div className="flex justify-end pt-2">
            <Button
              size="sm"
              onClick={requiresVerifiedConfigBeforeBrowse && !configVerified ? handleVerifyConfig : advanceToLabel}
              disabled={saving}
            >
              {saving
                ? (requiresVerifiedConfigBeforeBrowse && !configVerified ? 'Connecting...' : 'Working...')
                : (requiresVerifiedConfigBeforeBrowse && !configVerified ? 'Connect' : 'Next')}
            </Button>
          </div>
        </div>
      )}

      {/* Step 4: Label + create */}
      {step === 'label' && selectedPlugin && (
        <div className="space-y-4">
          <div className="flex items-center gap-2 mb-2">
            <PluginIcon pluginId={selectedPlugin.plugin_id} size={18} className="text-mga-accent" />
            <span className="text-sm font-medium text-mga-text">
              {pluginLabel(selectedPlugin.plugin_id)}
            </span>
          </div>

          <Input
            label="Label"
            value={label}
            onChange={(e) => setLabel(e.target.value)}
            placeholder="Give this integration a name..."
          />

          {/* Integration type — auto-derived from first capability, shown read-only */}
          <div className="flex flex-col gap-1">
            <span className="text-sm font-medium text-mga-text">Integration Type</span>
            <div className="flex gap-1.5">
              {selectedPlugin.capabilities.map((cap) => (
                <Badge key={cap} variant="accent" className="w-fit">{cap}</Badge>
              ))}
            </div>
          </div>

          {error && <p className="text-sm text-red-400">{error}</p>}

          <div className="flex justify-end gap-3 pt-2">
            <Button variant="outline" size="sm" onClick={onClose}>
              Cancel
            </Button>
            <Button
              size="sm"
              onClick={handleCreate}
              disabled={saving || !label || !integrationType}
            >
              {saving ? 'Creating...' : 'Create Integration'}
            </Button>
          </div>
        </div>
      )}

      {/* Step 5: OAuth consent in progress */}
      {step === 'oauth' && selectedPlugin && (
        <div className="space-y-4 py-6">
          <div className="flex items-center justify-center gap-2 mb-2">
            <PluginIcon pluginId={selectedPlugin.plugin_id} size={24} className="text-mga-accent" />
            <span className="text-sm font-medium text-mga-text">
              {pluginLabel(selectedPlugin.plugin_id)}
            </span>
          </div>

          {oauthResponse ? (
            <OAuthCallbackPanel
              providerLabel={pluginLabel(selectedPlugin.plugin_id)}
              authorizeUrl={oauthResponse.authorize_url}
              remoteBrowserHint={oauthResponse.remote_browser_hint}
              pasteCallbackSupported={oauthResponse.paste_callback_supported}
              busy={saving}
              error={oauthError}
              onOpenSignIn={reopenOAuthWindow}
              onSubmitCallback={submitPastedOAuthCallback}
              onCancel={() => {
                setStep(oauthPurpose === 'verify_config' ? 'config' : 'label')
                setOauthError(null)
                setOauthState(null)
                setOauthResponse(null)
              }}
            />
          ) : null}
        </div>
      )}

      {/* Step 6: Folder browser (for plugins that support source.browse) */}
      {step === 'browse' && selectedPlugin && createdIntegrationId && (
        <div className="space-y-4">
          <div className="flex items-center gap-2 mb-2">
            <PluginIcon pluginId={selectedPlugin.plugin_id} size={18} className="text-mga-accent" />
            <span className="text-sm font-medium text-mga-text">
              {pluginLabel(selectedPlugin.plugin_id)} — Choose a folder to scan
            </span>
          </div>

          <FolderBrowser
            pluginId={selectedPlugin.plugin_id}
            onSelect={async (path) => {
              try {
                await updateIntegration(createdIntegrationId, {
                  config: { include_paths: [{ path, recursive: true }] },
                })
              } catch {
                // Non-fatal: integration was created, folder preference just wasn't saved.
              }
              onSaved()
            }}
            onSkip={() => onSaved()}
          />
        </div>
      )}
    </Dialog>
  )
}

// ═══════════════════════════════════════════════════════════════════════════
// Edit Integration Dialog
// ═══════════════════════════════════════════════════════════════════════════

interface EditIntegrationDialogProps {
  integration: Integration
  onClose: () => void
  onSaved: () => void
}

export function EditIntegrationDialog({ integration, onClose, onSaved }: EditIntegrationDialogProps) {
  const { data: plugins = [] } = useQuery({ queryKey: ['plugins'], queryFn: listPlugins })
  const plugin = plugins.find((p) => p.plugin_id === integration.plugin_id)
  const { format: formatDT } = useDateTimeFormat()
  const { subscribe } = useSSE()

  const schema = useMemo(
    () => parsePluginConfigSchema(plugin?.config as Record<string, unknown> | undefined),
    [plugin],
  )

  // Parse existing config.
  const existingConfig = useMemo<Record<string, unknown>>(() => {
    try {
      const parsed = JSON.parse(integration.config_json) as Record<string, unknown>
      if (isFilesystemSourcePlugin(integration.plugin_id)) {
        parsed.include_paths = normalizeFilesystemIncludePaths(integration.plugin_id, parsed)
        delete parsed.path
        delete parsed.root_path
        delete parsed.exclude_paths
      }
      return parsed
    } catch {
      return {}
    }
  }, [integration.config_json, integration.plugin_id])

  // Form state.
  const [label, setLabel] = useState(integration.label)
  const [integrationType] = useState(integration.integration_type)
  const [configFields, setConfigFields] = useState<Record<string, unknown>>(existingConfig)
  const [saving, setSaving] = useState(false)
  const [error, setError] = useState('')
  const [oauthState, setOauthState] = useState<string | null>(null)
  const [oauthResponse, setOauthResponse] = useState<OAuthRequiredResponse | null>(null)
  const [oauthError, setOauthError] = useState<string | null>(null)
  const editOAuthWindowRef = useRef<Window | null>(null)

  // Folder browsing for plugins that support source.browse.
  const editSupportsBrowse = plugin?.provides?.includes('source.browse') ?? false

  // Secret fields — start masked, reveal on "Change" click.
  const [revealedSecrets, setRevealedSecrets] = useState<Set<string>>(new Set())
  const secretMask = useMemo(() => {
    const masked = new Set<string>()
    for (const { key, field } of schema) {
      if (field['x-secret'] && !revealedSecrets.has(key)) {
        masked.add(key)
      }
    }
    return masked
  }, [schema, revealedSecrets])

  const handleRevealSecret = (key: string) => {
    setRevealedSecrets((prev) => new Set([...prev, key]))
  }

  const buildConfig = useCallback(() => {
    const config: Record<string, unknown> = {}
    for (const { key, field } of schema) {
      // Skip masked secrets — don't send them (keep server value).
      if (secretMask.has(key)) continue

      const raw = configFields[key] ?? ''
      if (field.type === 'boolean') {
        config[key] = raw === true || raw === 'true' || raw === '1'
      } else if (field.type === 'number' || field.type === 'integer') {
        config[key] = typeof raw === 'number' ? raw : Number(raw)
      } else if (field.type === 'array') {
        config[key] = Array.isArray(raw) ? raw : []
      } else {
        config[key] = typeof raw === 'string' ? raw : (raw ?? '')
      }
    }

    // Include config fields not in schema (preserve unknown fields).
    for (const [k, v] of Object.entries(configFields)) {
      if (!(k in config) && !secretMask.has(k)) config[k] = v
    }
    if (isFilesystemSourcePlugin(integration.plugin_id)) {
      delete config.path
      delete config.root_path
      delete config.exclude_paths
    }
    return config
  }, [configFields, schema, secretMask])

  const saveChanges = useCallback(async () => {
    const authWindow = plugin?.provides?.includes('auth.oauth.callback') ? window.open('', '_blank') : null
    editOAuthWindowRef.current = authWindow
    try {
      const result = await updateIntegration(integration.id, {
        label,
        integration_type: integrationType,
        config: buildConfig(),
      })
      if (isOAuthRequired(result)) {
        setOauthState(result.state)
        setOauthResponse(result)
        setOauthError(null)
        if (authWindow) {
          authWindow.location.href = result.authorize_url
        } else {
          setOauthError('Browser blocked the sign-in popup. Allow popups for MGA and try again.')
        }
        return
      }
      authWindow?.close()
      onSaved()
    } catch (err) {
      authWindow?.close()
      if (err instanceof DuplicateIntegrationError) {
        setError(err.message)
      } else {
        setError(err instanceof Error ? err.message : 'Failed to save')
      }
    }
  }, [buildConfig, integration.id, integrationType, label, onSaved, plugin])

  const retrySaveAfterOAuth = useCallback(async () => {
    setSaving(true)
    setOauthError(null)
    setError('')
    try {
      const result = await updateIntegration(integration.id, {
        label,
        integration_type: integrationType,
        config: buildConfig(),
      })
      if (isOAuthRequired(result)) {
        setOauthError('Authentication incomplete. Please try again.')
        return
      }
      editOAuthWindowRef.current?.close()
      editOAuthWindowRef.current = null
      setOauthResponse(null)
      onSaved()
    } catch (err) {
      setOauthError(err instanceof Error ? err.message : 'Failed to save')
    } finally {
      setSaving(false)
    }
  }, [buildConfig, integration.id, integrationType, label, onSaved])

  useEffect(() => {
    if (!oauthState) return

    const unsubComplete = subscribe('oauth_complete', (data: unknown) => {
      const d = data as { state?: string }
      if (d.state === oauthState) {
        void retrySaveAfterOAuth()
      }
    })

    const unsubError = subscribe('oauth_error', (data: unknown) => {
      const d = data as { state?: string; error?: string }
      if (d.state === oauthState) {
        setSaving(false)
        setOauthError(d.error ?? 'Authentication failed')
      }
    })

    return () => {
      unsubComplete()
      unsubError()
    }
  }, [oauthState, retrySaveAfterOAuth, subscribe])

  const reopenEditOAuthWindow = useCallback(() => {
    if (!oauthResponse) return
    const authWindow = window.open('', '_blank')
    editOAuthWindowRef.current = authWindow
    if (authWindow) {
      authWindow.location.href = oauthResponse.authorize_url
      setOauthError(null)
    } else {
      setOauthError('Browser blocked the sign-in popup. Allow popups for MGA and try again.')
    }
  }, [oauthResponse])

  const submitEditPastedOAuthCallback = useCallback(async (callbackUrl: string) => {
    if (!oauthResponse) return
    setSaving(true)
    setOauthError(null)
    try {
      await importOAuthCallback(integration.plugin_id, callbackUrl)
    } catch (err) {
      setOauthError(err instanceof Error ? err.message : 'Failed to import callback URL')
    } finally {
      setSaving(false)
    }
  }, [integration.plugin_id, oauthResponse])

  const handleSave = async () => {
    setError('')
    setOauthError(null)
    setSaving(true)
    try {
      await saveChanges()
    } finally {
      setSaving(false)
    }
  }

  return (
    <Dialog open onClose={onClose} title="Edit Integration" className="max-w-2xl">
      <div className="space-y-5">
        {/* Read-only header info */}
        <div className="flex items-center gap-3 pb-3 border-b border-mga-border">
          <PluginIcon pluginId={integration.plugin_id} size={24} className="text-mga-accent" />
          <div className="flex-1">
            <div className="flex items-center gap-2">
              <span className="font-medium text-mga-text">{pluginLabel(integration.plugin_id)}</span>
              {plugin && <Badge variant="muted">v{plugin.plugin_version}</Badge>}
            </div>
            <p className="text-xs text-mga-muted mt-0.5 font-mono">{integration.id}</p>
          </div>
        </div>

        {/* Timestamps */}
        <div className="flex gap-6 text-xs text-mga-muted">
          <span>Created: {formatDT(integration.created_at)}</span>
          <span>Updated: {formatDT(integration.updated_at)}</span>
        </div>

        {/* Editable fields */}
        <Input
          label="Label"
          value={label}
          onChange={(e) => setLabel(e.target.value)}
        />

        {/* Integration type — read-only, shows all capabilities */}
        <div className="flex flex-col gap-1">
          <span className="text-sm font-medium text-mga-text">Integration Type</span>
          <div className="flex gap-1.5">
            {(plugin?.capabilities ?? [integrationType]).map((cap) => (
              <Badge key={cap} variant="accent" className="w-fit">{cap}</Badge>
            ))}
          </div>
        </div>

        {/* Config fields */}
        {schema.length > 0 && (
          <div>
            <h4 className="text-xs uppercase tracking-wider text-mga-muted font-medium mb-3">
              Configuration
            </h4>
            <ConfigFieldsRenderer
              schema={schema}
              values={configFields}
              onChange={(key, value) => setConfigFields((prev) => ({ ...prev, [key]: value }))}
              secretMask={secretMask}
              onRevealSecret={handleRevealSecret}
              browsePluginId={editSupportsBrowse ? integration.plugin_id : null}
            />
          </div>
        )}

        {oauthResponse ? (
          <OAuthCallbackPanel
            providerLabel={pluginLabel(integration.plugin_id)}
            authorizeUrl={oauthResponse.authorize_url}
            remoteBrowserHint={oauthResponse.remote_browser_hint}
            pasteCallbackSupported={oauthResponse.paste_callback_supported}
            busy={saving}
            error={oauthError}
            onOpenSignIn={reopenEditOAuthWindow}
            onSubmitCallback={submitEditPastedOAuthCallback}
            onCancel={() => {
              setOauthState(null)
              setOauthResponse(null)
              setOauthError(null)
            }}
          />
        ) : null}

        {/* Error */}
        {error && <p className="text-sm text-red-400">{error}</p>}

        {/* Actions */}
        <div className="flex justify-end gap-3 pt-2">
          <Button variant="outline" size="sm" onClick={onClose}>
            Cancel
          </Button>
          <Button
            size="sm"
            onClick={handleSave}
            disabled={saving || !label || !integrationType}
          >
            <Check size={14} />
            {saving ? 'Saving...' : 'Save Changes'}
          </Button>
        </div>
      </div>
    </Dialog>
  )
}
