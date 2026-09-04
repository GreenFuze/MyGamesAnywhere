import type { PluginConfigField } from '@/lib/pluginConfig'

/**
 * Starting values for a newly chosen provider.
 *
 * The renderer shows a default include-path row whether or not one is stored,
 * so seeding the same row keeps what the operator sees and what gets validated
 * in agreement. Without it the form displays a filled-in field and then refuses
 * to submit because that field is "missing".
 */
export function initialConfigValues(
  schema: Array<{ key: string; field: PluginConfigField }>,
): Record<string, unknown> {
  const seeded: Record<string, unknown> = {}
  for (const { key, field } of schema) {
    if (field.default !== undefined) seeded[key] = field.default
    else if (key === 'include_paths') seeded[key] = [{ path: '', recursive: true }]
  }
  return seeded
}

export interface RequiredField {
  key: string
  label: string
}

/** "base_path" reads as "Base Path" in the form, so name it the same way here. */
export function fieldLabel(key: string): string {
  return key
    .split('_')
    .filter(Boolean)
    .map((part) => part.charAt(0).toUpperCase() + part.slice(1))
    .join(' ')
}

function isBlank(value: unknown): boolean {
  if (value === undefined || value === null) return true
  if (typeof value === 'string') return value.trim() === ''
  if (Array.isArray(value)) return value.length === 0
  return false
}

/**
 * Lists the required configuration an operator has not filled in yet.
 *
 * The server rejects an incomplete connection anyway, but only after a round
 * trip, and it answers with the raw field name. Naming the gap before the
 * button is pressed is the difference between "Create connection" doing
 * nothing visible and the operator knowing they still have to pick a folder.
 *
 * A secret already stored is left alone while editing: the form masks it and
 * the server preserves it, so an untouched password is not a missing one.
 */
export function missingRequiredFields(
  schema: Array<{ key: string; field: PluginConfigField }>,
  values: Record<string, unknown>,
  options: { editing?: boolean } = {},
): RequiredField[] {
  return schema
    .filter(({ key, field }) => {
      if (field.required !== true) return false
      if (options.editing && field['x-secret'] === true) return false
      return isBlank(values[key])
    })
    .map(({ key }) => ({ key, label: fieldLabel(key) }))
}

/** One sentence naming what is still needed, or null when nothing is. */
export function describeMissingFields(missing: RequiredField[]): string | null {
  if (missing.length === 0) return null
  const labels = missing.map((entry) => entry.label)
  if (labels.length === 1) return `Choose a ${labels[0].toLowerCase()} before creating this connection.`
  const last = labels.pop()
  return `Fill in ${labels.join(', ')} and ${last} before creating this connection.`
}
