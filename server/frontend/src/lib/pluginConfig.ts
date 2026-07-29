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
