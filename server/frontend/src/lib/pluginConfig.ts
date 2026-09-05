/** A single field definition from a plugin's flat config schema. */
export type PluginConfigField = {
  type?: string
  required?: boolean
  default?: unknown
  description?: string
  items?: PluginConfigField
  properties?: Record<string, PluginConfigField>
  'x-secret'?: boolean
  'x-help-url'?: string
  'x-auth-method'?: string
}

/**
 * Parse the plugin's flat config schema into fields the player may edit.
 * Provider-managed credentials belong to their dedicated sign-in panel and
 * must never be rendered as text boxes.
 */
export function parsePluginConfigSchema(
  config: Record<string, unknown> | undefined,
): Array<{ key: string; field: PluginConfigField }> {
  if (!config) return []
  return Object.entries(config)
    .map(([key, def]) => ({
      key,
      field: (def ?? {}) as PluginConfigField,
    }))
    .filter(({ field }) => !field['x-auth-method'])
}

/**
 * The credential a provider issues through its own app, if it has one.
 *
 * parsePluginConfigSchema hides these from the form on purpose — a QR-issued
 * token is not something anyone types — which also made them invisible to the
 * console entirely. This is how a screen finds the one it must offer a sign-in
 * for instead.
 */
export function pluginQRSignInField(
  config: Record<string, unknown> | undefined,
): { key: string; field: PluginConfigField } | null {
  if (!config) return null
  for (const [key, def] of Object.entries(config)) {
    const field = (def ?? {}) as PluginConfigField
    if (field['x-auth-method'] === 'qr') return { key, field }
  }
  return null
}
