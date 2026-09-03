import { useState } from 'react'
import { useMutation } from '@tanstack/react-query'
import { ChevronLeft, ChevronRight, ExternalLink } from 'lucide-react'
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
import { parsePluginConfigSchema, type PluginConfigField } from '@/lib/gameUtils'
import { ProviderCatalog, type ProviderDescriptor } from '@/lib/providerCatalog'
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
  plugins, existing, onClose, onSaved,
}: {
  plugins: PluginInfo[]
  existing?: Integration
  onClose: () => void
  onSaved: () => Promise<void>
}) {
  const catalog = new ProviderCatalog(plugins)
  const editingProvider = existing ? catalog.find(existing.plugin_id) : undefined

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
  const [consentURL, setConsentURL] = useState<string | null>(null)

  const schema: Array<{ key: string; field: PluginConfigField }> = provider
    ? parsePluginConfigSchema(provider.plugin.config as Record<string, unknown> | undefined)
    : []

  const save = useMutation({
    mutationFn: async () => {
      if (!provider) throw new Error('Choose a provider first.')
      if (existing) {
        return updateIntegration(existing.id, { label: label.trim(), config: values })
      }
      return createIntegration({
        plugin_id: provider.pluginId,
        label: label.trim(),
        integration_type: provider.integrationType,
        config: values,
      })
    },
    onSuccess: async (result) => {
      // A provider needing consent returns 202 with a URL instead of a saved
      // connection; it does not exist until the operator finishes sign-in.
      if (isOAuthRequired(result) && result.authorize_url) {
        setConsentURL(result.authorize_url)
        return
      }
      await onSaved()
    },
  })

  const chooseProvider = (descriptor: ProviderDescriptor) => {
    setProvider(descriptor)
    // Suggest the provider's own name so the operator renames rather than types
    // from nothing, and start from a clean configuration for that provider.
    setLabel((current) => current.trim() === '' ? descriptor.name : current)
    setValues({})
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

  return (
    <FormDialog
      open
      onClose={onClose}
      title={title}
      submitLabel={existing ? 'Save changes' : 'Create connection'}
      submitting={save.isPending}
      error={save.error}
      disabled={!isFinalStep || !provider || label.trim() === ''}
      onSubmit={() => save.mutate()}
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

          <Input label="Label" value={label} onChange={(event) => setLabel(event.target.value)} autoFocus />

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

          {consentURL && (
            <div className="rounded-lg border border-amber-400/25 bg-amber-400/5 p-4">
              <p className="text-xs leading-5 text-mga-text">
                {provider.name} needs your approval before this connection can be saved.
              </p>
              <Button
                type="button"
                variant="outline"
                size="sm"
                className="mt-2"
                onClick={() => window.open(consentURL, '_blank', 'noopener,noreferrer')}
              >
                <ExternalLink className="h-3.5 w-3.5" /> Open {provider.name} sign-in
              </Button>
            </div>
          )}
        </div>
      )}
    </FormDialog>
  )
}
