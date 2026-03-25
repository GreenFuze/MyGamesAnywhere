import { useState, useCallback } from 'react'
import type { PluginConfigField } from '@/lib/gameUtils'
import { Input } from '@/components/ui/input'
import { Eye, EyeOff } from 'lucide-react'

interface ConfigFieldsRendererProps {
  /** Parsed schema fields from parsePluginConfigSchema(). */
  schema: Array<{ key: string; field: PluginConfigField }>
  /** Current field values (all stored as strings). */
  values: Record<string, string>
  /** Called when a field value changes. */
  onChange: (key: string, value: string) => void
  /** Set of field keys to show as masked (edit mode for existing secrets). */
  secretMask?: Set<string>
  /** Called when user clicks "Change" on a masked secret field. */
  onRevealSecret?: (key: string) => void
}

/**
 * Renders dynamic config form fields from a plugin's flat config schema.
 * Supports string, number, boolean types with secret masking and required indicators.
 */
export function ConfigFieldsRenderer({
  schema,
  values,
  onChange,
  secretMask,
  onRevealSecret,
}: ConfigFieldsRendererProps) {
  if (schema.length === 0) {
    return (
      <p className="text-sm text-mga-muted py-4 text-center">
        No configuration required for this plugin.
      </p>
    )
  }

  return (
    <div className="space-y-4">
      {schema.map(({ key, field }) => (
        <ConfigField
          key={key}
          fieldKey={key}
          field={field}
          value={values[key] ?? ''}
          onChange={onChange}
          isMasked={secretMask?.has(key) ?? false}
          onReveal={onRevealSecret}
        />
      ))}
    </div>
  )
}

// ---------------------------------------------------------------------------
// Individual field renderer
// ---------------------------------------------------------------------------

interface ConfigFieldProps {
  fieldKey: string
  field: PluginConfigField
  value: string
  onChange: (key: string, value: string) => void
  isMasked: boolean
  onReveal?: (key: string) => void
}

function ConfigField({ fieldKey, field, value, onChange, isMasked, onReveal }: ConfigFieldProps) {
  const [showSecret, setShowSecret] = useState(false)
  const isSecret = field['x-secret'] === true
  const isRequired = field.required === true
  const fieldType = field.type ?? 'string'

  // Build label with required indicator.
  const label = (
    <span>
      {fieldKey}
      {isRequired && <span className="text-red-400 ml-0.5">*</span>}
    </span>
  )

  const handleChange = useCallback(
    (val: string) => onChange(fieldKey, val),
    [fieldKey, onChange],
  )

  // Boolean fields render as checkbox.
  if (fieldType === 'boolean') {
    return (
      <label className="flex items-center gap-2 text-sm text-mga-text cursor-pointer">
        <input
          type="checkbox"
          checked={value === 'true' || value === '1'}
          onChange={(e) => handleChange(e.target.checked ? 'true' : 'false')}
          className="rounded border-mga-border"
        />
        <span>
          {fieldKey}
          {isRequired && <span className="text-red-400 ml-0.5">*</span>}
        </span>
        {field.description && (
          <span className="text-xs text-mga-muted">({field.description})</span>
        )}
      </label>
    )
  }

  // Masked secret (edit mode — existing value hidden).
  if (isMasked) {
    return (
      <div className="flex flex-col gap-1">
        <span className="text-sm font-medium text-mga-text">{label}</span>
        <div className="flex items-center gap-2">
          <span className="text-sm text-mga-muted font-mono">{'•'.repeat(12)}</span>
          <button
            type="button"
            onClick={() => onReveal?.(fieldKey)}
            className="text-xs text-mga-accent hover:underline"
          >
            Change
          </button>
        </div>
        {field.description && (
          <span className="text-xs text-mga-muted">{field.description}</span>
        )}
      </div>
    )
  }

  // Secret fields with show/hide toggle.
  if (isSecret) {
    return (
      <div className="flex flex-col gap-1">
        <span className="text-sm font-medium text-mga-text">{label}</span>
        <div className="relative">
          <Input
            type={showSecret ? 'text' : 'password'}
            value={value}
            onChange={(e) => handleChange(e.target.value)}
            placeholder={field.description ?? `Enter ${fieldKey}...`}
            className="pr-10"
          />
          <button
            type="button"
            onClick={() => setShowSecret(!showSecret)}
            className="absolute right-2 top-1/2 -translate-y-1/2 text-mga-muted hover:text-mga-text transition-colors"
          >
            {showSecret ? <EyeOff size={16} /> : <Eye size={16} />}
          </button>
        </div>
        {field.description && (
          <span className="text-xs text-mga-muted">{field.description}</span>
        )}
      </div>
    )
  }

  // Standard text/number input.
  return (
    <Input
      label={fieldKey + (isRequired ? ' *' : '')}
      type={fieldType === 'number' || fieldType === 'integer' ? 'number' : 'text'}
      value={value}
      onChange={(e) => handleChange(e.target.value)}
      placeholder={field.description ?? (field.default != null ? String(field.default) : undefined)}
    />
  )
}
