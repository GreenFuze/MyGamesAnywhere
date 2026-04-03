import { useState, useMemo, useEffect, useCallback } from 'react'
import { useQuery } from '@tanstack/react-query'
import {
  listPlugins,
  createIntegration,
  isOAuthRequired,
  updateIntegration,
  DuplicateIntegrationError,
  type Integration,
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
import { ArrowLeft, Check } from 'lucide-react'

// ═══════════════════════════════════════════════════════════════════════════
// Add Integration Wizard
// ═══════════════════════════════════════════════════════════════════════════

type WizardStep = 'category' | 'plugin' | 'config' | 'label' | 'oauth' | 'browse'

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
  const [oauthError, setOauthError] = useState<string | null>(null)
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
    return config
  }, [schema, configFields])

  // Check if the selected plugin supports folder browsing.
  const supportsBrowse = selectedPlugin?.provides?.includes('source.browse') ?? false

  // After successful integration creation, either go to browse step or finish.
  const finishCreate = useCallback((integrationId: string) => {
    if (supportsBrowse) {
      setCreatedIntegrationId(integrationId)
      setStep('browse')
    } else {
      onSaved()
    }
  }, [supportsBrowse, onSaved])

  const handleCreate = async () => {
    if (!selectedPlugin || !label || !integrationType) return
    setError('')
    setSaving(true)

    try {
      const result = await createIntegration({
        plugin_id: selectedPlugin.plugin_id,
        label,
        integration_type: integrationType,
        config: buildConfig(),
      })

      // OAuth consent required — open the authorize URL in a new tab.
      if (isOAuthRequired(result)) {
        setOauthState(result.state)
        setOauthError(null)
        setStep('oauth')
        window.open(result.authorize_url, '_blank')
        return
      }

      finishCreate(result.id)
    } catch (err) {
      if (err instanceof DuplicateIntegrationError) {
        setError(err.message)
      } else {
        setError(err instanceof Error ? err.message : 'Failed to create integration')
      }
    } finally {
      setSaving(false)
    }
  }

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
        retryCreateAfterOAuth()
      }
    })

    const unsubError = subscribe('oauth_error', (data: unknown) => {
      const d = data as { state?: string; error?: string }
      if (d.state === oauthState) {
        setOauthError(d.error ?? 'Authentication failed')
      }
    })

    return () => { unsubComplete(); unsubError() }
  }, [step, oauthState, subscribe, retryCreateAfterOAuth])

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
        <button
          type="button"
          onClick={goBack}
          className="flex items-center gap-1 text-xs text-mga-muted hover:text-mga-text mb-4 transition-colors"
        >
          <ArrowLeft size={14} /> Back
        </button>
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
                      ? 'No configuration needed'
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

          <ConfigFieldsRenderer
            schema={schema}
            values={configFields}
            onChange={(key, value) => setConfigFields((prev) => ({ ...prev, [key]: value }))}
            browsePluginId={supportsBrowse ? selectedPlugin.plugin_id : null}
          />

          {error && <p className="text-sm text-red-400">{error}</p>}

          <div className="flex justify-end pt-2">
            <Button size="sm" onClick={advanceToLabel}>
              Next
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
        <div className="space-y-4 text-center py-6">
          <div className="flex items-center justify-center gap-2 mb-2">
            <PluginIcon pluginId={selectedPlugin.plugin_id} size={24} className="text-mga-accent" />
            <span className="text-sm font-medium text-mga-text">
              {pluginLabel(selectedPlugin.plugin_id)}
            </span>
          </div>

          {saving ? (
            <p className="text-mga-muted">Creating integration...</p>
          ) : oauthError ? (
            <div className="space-y-3">
              <p className="text-sm text-red-400">{oauthError}</p>
              <Button
                size="sm"
                onClick={() => {
                  setStep('label')
                  setOauthError(null)
                  setOauthState(null)
                }}
              >
                Try Again
              </Button>
            </div>
          ) : (
            <div className="animate-pulse">
              <p className="text-mga-text font-medium">Waiting for sign-in...</p>
              <p className="text-xs text-mga-muted mt-2">
                A new browser tab has been opened for authentication.
                <br />Complete the sign-in and this will update automatically.
              </p>
            </div>
          )}
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
  const [oauthError, setOauthError] = useState<string | null>(null)

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
    }
    return config
  }, [configFields, schema, secretMask])

  const saveChanges = useCallback(async () => {
    try {
      const result = await updateIntegration(integration.id, {
        label,
        integration_type: integrationType,
        config: buildConfig(),
      })
      if (isOAuthRequired(result)) {
        setOauthState(result.state)
        setOauthError(null)
        window.open(result.authorize_url, '_blank')
        return
      }
      onSaved()
    } catch (err) {
      if (err instanceof DuplicateIntegrationError) {
        setError(err.message)
      } else {
        setError(err instanceof Error ? err.message : 'Failed to save')
      }
    }
  }, [buildConfig, integration.id, integrationType, label, onSaved])

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

        {oauthState && !oauthError && (
          <div className="rounded-mga border border-amber-500/30 bg-amber-500/10 p-3 text-sm text-amber-200">
            Waiting for sign-in in the browser. This dialog will retry the save automatically when authentication completes.
          </div>
        )}
        {oauthError && (
          <div className="rounded-mga border border-red-500/30 bg-red-500/10 p-3 text-sm text-red-300">
            {oauthError}
          </div>
        )}

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
