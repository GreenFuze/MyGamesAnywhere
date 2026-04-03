import { useState, useCallback } from 'react'
import type { FilesystemIncludePath, PluginConfigField } from '@/lib/gameUtils'
import { Input } from '@/components/ui/input'
import { Eye, EyeOff, ExternalLink } from 'lucide-react'
import { FolderBrowser } from './FolderBrowser'

interface ConfigFieldsRendererProps {
  /** Parsed schema fields from parsePluginConfigSchema(). */
  schema: Array<{ key: string; field: PluginConfigField }>
  /** Current field values. */
  values: Record<string, unknown>
  /** Called when a field value changes. */
  onChange: (key: string, value: unknown) => void
  /** Set of field keys to show as masked (edit mode for existing secrets). */
  secretMask?: Set<string>
  /** Called when user clicks "Change" on a masked secret field. */
  onRevealSecret?: (key: string) => void
  /** Plugin id used for folder browsing where supported. */
  browsePluginId?: string | null
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
  browsePluginId,
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
          value={values[key]}
          onChange={onChange}
          isMasked={secretMask?.has(key) ?? false}
          onReveal={onRevealSecret}
          browsePluginId={browsePluginId}
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
  value: unknown
  onChange: (key: string, value: unknown) => void
  isMasked: boolean
  onReveal?: (key: string) => void
  browsePluginId?: string | null
}

/** Renders field description text with an optional help link. */
function FieldHint({ field }: { field: PluginConfigField }) {
  const helpUrl = field['x-help-url']

  if (!field.description && !helpUrl) return null

  return (
    <span className="text-xs text-mga-muted flex items-center gap-1.5">
      {field.description && <span>{field.description}</span>}
      {helpUrl && (
        <a
          href={helpUrl}
          target="_blank"
          rel="noopener noreferrer"
          className="inline-flex items-center gap-0.5 text-mga-accent hover:underline"
        >
          {field.description ? 'Get one' : 'Help'}
          <ExternalLink size={12} />
        </a>
      )}
    </span>
  )
}

function ConfigField({ fieldKey, field, value, onChange, isMasked, onReveal, browsePluginId }: ConfigFieldProps) {
  const [showSecret, setShowSecret] = useState(false)
  const isSecret = field['x-secret'] === true
  const isRequired = field.required === true
  const fieldType = field.type ?? 'string'

  // Build label with required indicator.
  const label = (
    <span>
      {displayLabel(fieldKey)}
      {isRequired && <span className="text-red-400 ml-0.5">*</span>}
    </span>
  )

  const handleChange = useCallback(
    (val: string) => onChange(fieldKey, val),
    [fieldKey, onChange],
  )

  if (fieldType === 'array' && fieldKey === 'include_paths') {
    return (
      <IncludePathsField
        fieldKey={fieldKey}
        field={field}
        value={value}
        onChange={onChange}
        browsePluginId={browsePluginId}
      />
    )
  }

  const stringValue = typeof value === 'string' ? value : value == null ? '' : String(value)

  // Boolean fields render as checkbox.
  if (fieldType === 'boolean') {
    return (
      <label className="flex items-center gap-2 text-sm text-mga-text cursor-pointer">
        <input
          type="checkbox"
          checked={value === true || stringValue === 'true' || stringValue === '1'}
          onChange={(e) => handleChange(e.target.checked ? 'true' : 'false')}
          className="rounded border-mga-border"
        />
        <span>
          {displayLabel(fieldKey)}
          {isRequired && <span className="text-red-400 ml-0.5">*</span>}
        </span>
        <FieldHint field={field} />
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
        <FieldHint field={field} />
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
            value={stringValue}
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
        <FieldHint field={field} />
      </div>
    )
  }

  // Standard text/number input.
  return (
    <Input
      label={displayLabel(fieldKey) + (isRequired ? ' *' : '')}
      type={fieldType === 'number' || fieldType === 'integer' ? 'number' : 'text'}
      value={stringValue}
      onChange={(e) => handleChange(e.target.value)}
      placeholder={field.description ?? (field.default != null ? String(field.default) : undefined)}
    />
  )
}

function IncludePathsField({
  fieldKey,
  field,
  value,
  onChange,
  browsePluginId,
}: {
  fieldKey: string
  field: PluginConfigField
  value: unknown
  onChange: (key: string, value: unknown) => void
  browsePluginId?: string | null
}) {
  const [browserIndex, setBrowserIndex] = useState<number | null>(null)
  const includePaths = normalizeIncludePathsValue(value)

  const update = (next: FilesystemIncludePath[]) => {
    onChange(fieldKey, next)
  }

  const setPath = (index: number, path: string) => {
    update(includePaths.map((entry, current) => (
      current === index ? { ...entry, path } : entry
    )))
  }

  const setRecursive = (index: number, recursive: boolean) => {
    update(includePaths.map((entry, current) => (
      current === index ? { ...entry, recursive } : entry
    )))
  }

  const remove = (index: number) => {
    const next = includePaths.filter((_, current) => current !== index)
    update(next.length > 0 ? next : [{ path: '', recursive: true }])
  }

  return (
    <div className="space-y-3">
      <div className="flex items-center justify-between">
        <span className="text-sm font-medium text-mga-text">
          {displayLabel(fieldKey)}
          {field.required && <span className="text-red-400 ml-0.5">*</span>}
        </span>
        <button
          type="button"
          onClick={() => update([...includePaths, { path: '', recursive: true }])}
          className="text-xs text-mga-accent hover:underline"
        >
          Add include path
        </button>
      </div>

      {includePaths.map((entry, index) => (
        <div key={`${index}:${entry.path}`} className="rounded-mga border border-mga-border p-3 space-y-3">
          <div className="flex gap-2 items-start">
            <div className="flex-1">
              <Input
                label={`Path ${index + 1}`}
                value={entry.path}
                onChange={(e) => setPath(index, e.target.value)}
                placeholder="Leave empty to scan the root"
              />
            </div>
            <button
              type="button"
              onClick={() => remove(index)}
              className="text-xs text-red-300 hover:text-red-200 mt-7"
            >
              Remove
            </button>
          </div>

          <label className="flex items-center gap-2 text-sm text-mga-text cursor-pointer">
            <input
              type="checkbox"
              checked={entry.recursive}
              onChange={(e) => setRecursive(index, e.target.checked)}
              className="rounded border-mga-border"
            />
            <span>Scan recursively</span>
          </label>

          {browsePluginId && (
            <div className="space-y-2">
              <button
                type="button"
                onClick={() => setBrowserIndex(browserIndex === index ? null : index)}
                className="text-xs text-mga-accent hover:underline"
              >
                {browserIndex === index ? 'Hide browser' : 'Browse...'}
              </button>
              {browserIndex === index && (
                <FolderBrowser
                  pluginId={browsePluginId}
                  initialPath={entry.path}
                  onSelect={(path) => {
                    setPath(index, path)
                    setBrowserIndex(null)
                  }}
                />
              )}
            </div>
          )}
        </div>
      ))}

      <FieldHint field={field} />
    </div>
  )
}

function normalizeIncludePathsValue(value: unknown): FilesystemIncludePath[] {
  if (Array.isArray(value)) {
    const includePaths = value
      .map((entry) => {
        if (!entry || typeof entry !== 'object') return null
        const item = entry as Record<string, unknown>
        return {
          path: typeof item.path === 'string' ? item.path : '',
          recursive: typeof item.recursive === 'boolean' ? item.recursive : true,
        }
      })
      .filter((entry): entry is FilesystemIncludePath => entry !== null)
    if (includePaths.length > 0) {
      return includePaths
    }
  }
  return [{ path: '', recursive: true }]
}

function displayLabel(fieldKey: string): string {
  return fieldKey
    .split('_')
    .filter(Boolean)
    .map((part) => part.charAt(0).toUpperCase() + part.slice(1))
    .join(' ')
}
