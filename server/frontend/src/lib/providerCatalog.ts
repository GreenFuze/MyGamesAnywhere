import type { PluginInfo } from '@/api/client'
import { CAPABILITY_META, CAPABILITY_ORDER, pluginLabel } from './displayText.ts'
import { parsePluginConfigSchema } from './pluginConfig.ts'

/** How a provider proves who you are and where its content lives. Providers are
 * not interchangeable, so the console states this before anything is filled in. */
export type ProviderSetupKind =
  | 'sign_in'      // redirects to the provider for consent
  | 'credentials'  // needs a secret typed into MGA
  | 'location'     // needs a folder, share, or path
  | 'none'         // works as soon as it is named

export interface ProviderSetup {
  kind: ProviderSetupKind
  /** Short phrase for the provider list. */
  summary: string
  /** Sentence shown once the provider is chosen. */
  detail: string
}

export interface ProviderDescriptor {
  plugin: PluginInfo
  pluginId: string
  /** Human name, never the raw plugin id. */
  name: string
  /** Server-side integration type, derived from the plugin's own capability. */
  integrationType: string
  capability: string
  setup: ProviderSetup
  /** Whether the provider has editable configuration fields. */
  hasConfig: boolean
}

export interface ProviderCategory {
  capability: string
  label: string
  providers: ProviderDescriptor[]
}

const SETUP_COPY: Record<ProviderSetupKind, Omit<ProviderSetup, 'kind'>> = {
  sign_in: {
    summary: 'Sign-in required',
    detail: 'You will be sent to the provider to approve access. MGA never sees your provider password.',
  },
  credentials: {
    summary: 'Credentials required',
    detail: 'This provider needs a secret stored by MGA, such as an API key or account password.',
  },
  location: {
    summary: 'Location required',
    detail: 'Choose which folder or share MGA should read. Nothing outside it is scanned.',
  },
  none: {
    summary: 'No setup needed',
    detail: 'This provider works as soon as you name the connection.',
  },
}

/** Classifies one plugin so the console can explain it before it is chosen. */
export function describeProvider(plugin: PluginInfo): ProviderDescriptor {
  const provides = plugin.provides ?? []
  const schema = parsePluginConfigSchema(plugin.config as Record<string, unknown> | undefined)

  // A provider that redirects for consent is a sign-in provider even when it
  // also exposes configuration fields.
  const usesOAuth = provides.some((item) => item.startsWith('auth.oauth'))
  const hasSecretField = schema.some(({ field }) => field['x-secret'] === true)
  const needsLocation = schema.some(({ key }) =>
    key === 'include_paths' || key === 'root_path' || key === 'path' || key === 'share')

  let kind: ProviderSetupKind = 'none'
  if (usesOAuth) kind = 'sign_in'
  else if (hasSecretField) kind = 'credentials'
  else if (needsLocation) kind = 'location'

  return {
    plugin,
    pluginId: plugin.plugin_id,
    name: pluginLabel(plugin.plugin_id),
    // The plugin's own first capability is the authoritative integration type;
    // assuming "source" would mis-file metadata and save-sync connections.
    integrationType: plugin.capabilities?.[0] ?? 'source',
    capability: plugin.capabilities?.[0] ?? 'other',
    setup: { kind, ...SETUP_COPY[kind] },
    hasConfig: schema.length > 0,
  }
}

/**
 * Groups the installed plugins into the categories the product already uses,
 * so an operator picks what they are connecting before which vendor supplies
 * it. A flat provider list hides the fact that a storefront, a metadata
 * service, and a network share are different kinds of connection.
 */
export class ProviderCatalog {
  readonly #byCapability: Map<string, ProviderDescriptor[]>

  constructor(plugins: readonly PluginInfo[]) {
    this.#byCapability = new Map()
    for (const plugin of plugins) {
      const descriptor = describeProvider(plugin)
      const existing = this.#byCapability.get(descriptor.capability)
      if (existing) existing.push(descriptor)
      else this.#byCapability.set(descriptor.capability, [descriptor])
    }
    for (const group of this.#byCapability.values()) {
      group.sort((left, right) => left.name.localeCompare(right.name))
    }
  }

  /** Categories in the product's established order, unknown ones last. */
  categories(): ProviderCategory[] {
    const known = CAPABILITY_ORDER.filter((capability) => this.#byCapability.has(capability))
    const extra = [...this.#byCapability.keys()]
      .filter((capability) => !CAPABILITY_ORDER.includes(capability))
      .sort()
    return [...known, ...extra].map((capability) => ({
      capability,
      label: CAPABILITY_META[capability]?.label ?? capability,
      providers: this.#byCapability.get(capability) ?? [],
    }))
  }

  providersFor(capability: string): ProviderDescriptor[] {
    return this.#byCapability.get(capability) ?? []
  }

  find(pluginId: string): ProviderDescriptor | undefined {
    for (const group of this.#byCapability.values()) {
      const match = group.find((descriptor) => descriptor.pluginId === pluginId)
      if (match) return match
    }
    return undefined
  }
}
