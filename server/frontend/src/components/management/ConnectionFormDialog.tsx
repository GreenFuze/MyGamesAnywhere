import { useEffect, useRef, useState } from 'react'
import { useMutation } from '@tanstack/react-query'
import { ChevronLeft, ChevronRight, ExternalLink, LoaderCircle } from 'lucide-react'
import {
  browsePlugin,
  createIntegration,
  isOAuthRequired,
  updateIntegration,
  type Integration,
  type PluginInfo,
} from '@/api/client'
import { Button } from '@/components/ui/button'
import { Input } from '@/components/ui/input'
import { ConfigFieldsRenderer } from '@/components/settings/ConfigFieldsRenderer'
import { PluginIcon } from '@/components/settings/PluginIcon'
import { FormDialog } from '@/components/management/ManagementActions'
import { useSSE } from '@/hooks/useSSE'
import { parsePluginConfigSchema, type PluginConfigField } from '@/lib/gameUtils'
import { ProviderCatalog, type ProviderDescriptor } from '@/lib/providerCatalog'
import { explainConnectionFailure } from '@/lib/connectionErrors'
import { describeMissingFields, initialConfigValues, missingRequiredFields } from '@/lib/connectionValidation'
import { cn } from '@/lib/utils'

type Step = 'category' | 'provider' | 'configure'

/**
 * Adds or edits one connection.
 *
 * Providers are not equivalent — a storefront redirects for consent, a network
 * share needs a host and credentials, a metadata service needs an API key — so
 * the operator chooses what kind of connection they are making, then which
 * provider supplies it, and only then sees fields that apply to that provider.
 */
export function ConnectionFormDialog({
  plugins, existing, onClose, onSaved, connections,
}: {
  plugins: PluginInfo[]
  existing?: Integration
  onClose: () => void
  /** Receives the new connection so the caller can read it straight away. */
  onSaved: (created?: Integration) => Promise<void>
  /** Connections already on this profile, so a second Drive source can warn
   *  about overlapping with the first rather than silently duplicating it. */
  connections?: Integration[]
}) {
  const catalog = new ProviderCatalog(plugins)
  const editingProvider = existing ? catalog.find(existing.plugin_id) : undefined
  const hasDriveApiConnection = (connections ?? []).some((item) => item.plugin_id === 'game-source-google-drive')

  // Editing keeps the provider fixed; only a new connection walks the steps.
  const [step, setStep] = useState<Step>(existing ? 'configure' : 'category')
  const [category, setCategory] = useState<string | null>(null)
  const [provider, setProvider] = useState<ProviderDescriptor | null>(editingProvider ?? null)
  const [label, setLabel] = useState(existing?.label ?? '')
  const [values, setValues] = useState<Record<string, unknown>>(() => {
    if (!existing?.config_json) return {}
    try {
      return JSON.parse(existing.config_json) as Record<string, unknown>
    } catch {
      // Unparseable stored configuration is shown as empty rather than
      // silently discarding whatever the operator types next.
      return {}
    }
  })
  // A provider needing consent answers the first request with an authorize URL
  // and a state token. The connection is only created when that same state is
  // sent back, so both halves are kept until the handshake finishes.
  const [consent, setConsent] = useState<{ url: string; state: string } | null>(null)
  const [consentError, setConsentError] = useState<string | null>(null)
  const [awaitingConsent, setAwaitingConsent] = useState(false)
  const { subscribe } = useSSE()

  const schema: Array<{ key: string; field: PluginConfigField }> = provider
    ? parsePluginConfigSchema(provider.plugin.config as Record<string, unknown> | undefined)
    : []

  const save = useMutation({
    mutationFn: async (oauthState?: string) => {
      if (!provider) throw new Error('Choose a provider first.')
      if (existing) {
        return updateIntegration(existing.id, { label: label.trim(), config: values })
      }
      return createIntegration({
        plugin_id: provider.pluginId,
        label: label.trim(),
        integration_type: provider.integrationType,
        config: values,
        oauth_state: oauthState,
      })
    },
    onSuccess: async (result) => {
      if (isOAuthRequired(result) && result.authorize_url) {
        setConsent({ url: result.authorize_url, state: result.state })
        setConsentError(null)
        setAwaitingConsent(false)
        return
      }
      await onSaved(existing ? undefined : (result as Integration))
    },
  })

  // Retrying must use the latest values, so read them through a ref rather
  // than capturing them in the subscription closure.
  const retry = useRef<(state: string) => void>(() => {})
  retry.current = (state: string) => {
    setAwaitingConsent(false)
    save.mutate(state)
  }

  // The provider redirects back to the server, not to this page, so the server
  // reports the outcome over SSE. Without this the operator finishes sign-in
  // and nothing happens.
  useEffect(() => {
    if (!consent) return
    const done = subscribe('oauth_complete', (data: unknown) => {
      if ((data as { state?: string }).state === consent.state) retry.current(consent.state)
    })
    const failed = subscribe('oauth_error', (data: unknown) => {
      const payload = data as { state?: string; error?: string }
      if (payload.state !== consent.state) return
      setAwaitingConsent(false)
      setConsentError(payload.error ?? 'The provider rejected the sign-in.')
    })
    return () => { done(); failed() }
  }, [consent, subscribe])

  const chooseProvider = (descriptor: ProviderDescriptor) => {
    setProvider(descriptor)
    // Suggest the provider's own name so the operator renames rather than types
    // from nothing, and start from a clean configuration for that provider.
    setLabel((current) => current.trim() === '' ? descriptor.name : current)
    setValues(initialConfigValues(parsePluginConfigSchema(
      descriptor.plugin.config as Record<string, unknown> | undefined,
    )))
    setConsent(null)
    setConsentError(null)
    setAwaitingConsent(false)
    setStep('configure')
  }

  const goBack = () => {
    if (step === 'configure' && !existing) setStep('provider')
    else if (step === 'provider') setStep('category')
  }

  const title = existing
    ? `Edit ${existing.label}`
    : step === 'category' ? 'What are you connecting?'
      : step === 'provider' ? 'Choose a provider'
        : `Configure ${provider?.name ?? 'connection'}`

  // Only the final step submits; earlier steps navigate.
  const isFinalStep = step === 'configure'

  // Some server rejections are protective and cannot be retried; explain those
  // rather than showing the raw sentence next to a button that will fail again.
  const failure = explainConnectionFailure(save.error, provider?.name ?? 'This provider')

  // A provider that needs a folder must not be creatable without one. The
  // server refuses it anyway, but only after a round trip and in its own words.
  const missing = missingRequiredFields(schema, values, { editing: Boolean(existing) })
  const missingMessage = describeMissingFields(missing)

  return (
    <FormDialog
      open
      onClose={onClose}
      title={title}
      submitLabel={existing ? 'Save changes' : 'Create connection'}
      submitting={save.isPending}
      error={failure ? undefined : save.error}
      disabled={!isFinalStep || !provider || label.trim() === '' || missing.length > 0 || failure?.terminal === true}
      onSubmit={() => save.mutate(consent?.state)}
      hideSubmit={!isFinalStep}
      leading={step !== 'category' && !existing ? (
        <Button type="button" variant="ghost" onClick={goBack}>
          <ChevronLeft className="h-4 w-4" /> Back
        </Button>
      ) : undefined}
    >
      {step === 'category' && (
        <div className="grid gap-2 sm:grid-cols-2">
          {catalog.categories().map((entry) => (
            <button
              key={entry.capability}
              type="button"
              onClick={() => { setCategory(entry.capability); setStep('provider') }}
              className="flex items-center gap-3 rounded-lg border border-mga-border bg-mga-elevated/40 p-4 text-left transition hover:border-mga-accent/40 hover:bg-mga-elevated focus:outline-none focus:ring-2 focus:ring-mga-accent/40"
            >
              <PluginIcon capability={entry.capability} size={22} className="shrink-0 text-mga-accent" />
              <span className="min-w-0">
                <span className="block truncate text-sm font-medium text-mga-text">{entry.label}</span>
                <span className="block text-xs text-mga-muted">
                  {entry.providers.length} provider{entry.providers.length === 1 ? '' : 's'}
                </span>
              </span>
              <ChevronRight className="ml-auto h-4 w-4 shrink-0 text-mga-muted" />
            </button>
          ))}
        </div>
      )}

      {step === 'provider' && (
        <div className="max-h-[50vh] space-y-2 overflow-y-auto pr-1">
          {catalog.providersFor(category ?? '').map((descriptor) => (
            <button
              key={descriptor.pluginId}
              type="button"
              onClick={() => chooseProvider(descriptor)}
              className="flex w-full items-center gap-3 rounded-lg border border-mga-border bg-mga-elevated/40 p-3 text-left transition hover:border-mga-accent/40 hover:bg-mga-elevated focus:outline-none focus:ring-2 focus:ring-mga-accent/40"
            >
              <PluginIcon pluginId={descriptor.pluginId} capability={descriptor.capability} size={20} className="shrink-0 text-mga-accent" />
              <span className="min-w-0 flex-1">
                <span className="block truncate text-sm font-medium text-mga-text">{descriptor.name}</span>
                <span className="block text-xs text-mga-muted">{descriptor.setup.summary}</span>
              </span>
              <ChevronRight className="h-4 w-4 shrink-0 text-mga-muted" />
            </button>
          ))}
        </div>
      )}

      {step === 'configure' && provider && (
        <div className="space-y-4">
          <div className={cn(
            'flex items-start gap-3 rounded-lg border p-3',
            provider.setup.kind === 'sign_in'
              ? 'border-sky-400/25 bg-sky-400/5'
              : 'border-mga-border bg-mga-elevated/40',
          )}>
            <PluginIcon pluginId={provider.pluginId} capability={provider.capability} size={20} className="mt-0.5 shrink-0 text-mga-accent" />
            <div className="min-w-0">
              <p className="text-sm font-medium text-mga-text">{provider.name}</p>
              <p className="mt-1 text-xs leading-5 text-mga-muted">{provider.setup.detail}</p>
            </div>
          </div>

          {provider.pluginId === 'game-source-google-drive-desktop' && (
            <SyncedDriveNotes hasDriveApiConnection={hasDriveApiConnection} />
          )}

          <Input label="Label" value={label} onChange={(event) => setLabel(event.target.value)} autoFocus />

          {failure && (
            <div className="rounded-lg border border-rose-400/25 bg-rose-500/5 p-4" role="alert">
              <p className="text-sm font-semibold text-rose-200">{failure.title}</p>
              <p className="mt-1 text-xs leading-5 text-mga-muted">{failure.detail}</p>
            </div>
          )}

          {missingMessage && !failure && (
            <p className="text-xs leading-5 text-amber-300" role="status">{missingMessage}</p>
          )}

          {schema.length > 0 && (
            <div className="max-h-[40vh] overflow-y-auto pr-1">
              <ConfigFieldsRenderer
                schema={schema}
                values={values}
                onChange={(key, value) => setValues((current) => ({ ...current, [key]: value }))}
                browsePluginId={provider.pluginId}
                browse={(path) => browsePlugin(provider.pluginId, path, {
                  integrationId: existing?.id,
                  config: values,
                })}
              />
            </div>
          )}

          {consent && (
            <div className="rounded-lg border border-amber-400/25 bg-amber-400/5 p-4">
              <p className="text-xs leading-5 text-mga-text">
                {provider.name} needs your approval before this connection can be saved. Approve it in
                the provider tab; this dialog finishes on its own once the provider confirms.
              </p>
              <div className="mt-3 flex flex-wrap items-center gap-2">
                <Button
                  type="button"
                  variant="outline"
                  size="sm"
                  onClick={() => {
                    setConsentError(null)
                    setAwaitingConsent(true)
                    window.open(consent.url, '_blank', 'noopener,noreferrer')
                  }}
                >
                  <ExternalLink className="h-3.5 w-3.5" />
                  {awaitingConsent ? `Reopen ${provider.name} sign-in` : `Open ${provider.name} sign-in`}
                </Button>
                {awaitingConsent && !save.isPending && (
                  <span className="flex items-center gap-2 text-xs text-mga-muted">
                    <LoaderCircle className="h-3.5 w-3.5 animate-spin" /> Waiting for {provider.name}…
                  </span>
                )}
                {/* The confirmation travels over SSE. If that connection is
                    down, the operator can still finish the handshake. */}
                <Button
                  type="button"
                  variant="ghost"
                  size="sm"
                  disabled={save.isPending}
                  onClick={() => retry.current(consent.state)}
                >
                  I have approved it
                </Button>
              </div>
              {consentError && (
                <p className="mt-3 text-xs leading-5 text-rose-300" role="alert">{consentError}</p>
              )}
            </div>
          )}
        </div>
      )}
    </FormDialog>
  )
}

/** Two things worth knowing before pointing MGA at a synced Drive, neither of
 *  which the server can decide for the user.
 *
 *  The overlap warning is deliberately conditional rather than certain. MGA
 *  cannot tell whether the Drive API connection covers the same folders,
 *  because that connection stores Drive-side paths and this one stores a local
 *  mount, and nothing records which Google account either belongs to. Saying
 *  "if they overlap" is honest; claiming they do would not be.
 *
 *  Streaming is not detected at all. Drive for Desktop decides per folder
 *  whether content is on the disk or fetched on read, and there is no reliable
 *  way to ask it from here — so the user is told what to check rather than
 *  shown a guess dressed up as a fact. */
function SyncedDriveNotes({ hasDriveApiConnection }: { hasDriveApiConnection: boolean }) {
  return (
    <div className="space-y-2">
      {hasDriveApiConnection && (
        <div className="rounded-lg border border-amber-400/25 bg-amber-400/5 p-3" role="status">
          <p className="text-xs font-medium text-amber-200">You already have a Google Drive connection</p>
          <p className="mt-1 text-xs leading-5 text-mga-muted">
            If this folder holds the same games, each one will appear twice — once from each connection. Point them at
            different folders, or remove the other connection once this one has scanned.
          </p>
        </div>
      )}
      <div className="rounded-lg border border-mga-border bg-mga-elevated/40 p-3">
        <p className="text-xs font-medium text-mga-text">Check these files are on the disk</p>
        <p className="mt-1 text-xs leading-5 text-mga-muted">
          Google Drive can keep files online-only and fetch them when something opens them. MGA sends the actual bytes,
          so a scan or a download will pull anything that is not local yet, which can be slow and can fill the drive.
          In Drive for Desktop, set this folder to be available offline if you want to avoid that.
        </p>
      </div>
    </div>
  )
}
