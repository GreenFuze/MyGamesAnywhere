import { useState, useCallback } from 'react'
import type { FilesystemIncludePath, PluginConfigField } from '@/lib/gameUtils'
import { Input } from '@/components/ui/input'
import { Button } from '@/components/ui/button'
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
  /** Disables remote browsing until the integration has been verified/authenticated. */
  browseDisabled?: boolean
  browseDisabledReason?: string
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
  browseDisabled = false,
  browseDisabledReason,
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
          browseDisabled={browseDisabled}
          browseDisabledReason={browseDisabledReason}
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
  browseDisabled: boolean
  browseDisabledReason?: string
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

function ConfigField({ fieldKey, field, value, onChange, isMasked, onReveal, browsePluginId, browseDisabled, browseDisabledReason }: ConfigFieldProps) {
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
        browseDisabled={browseDisabled}
        browseDisabledReason={browseDisabledReason}
      />
    )
  }

  if (fieldType === 'array' && fieldKey === 'exclude_paths') {
    return (
      <StringPathsField
        fieldKey={fieldKey}
        field={field}
        value={value}
        onChange={onChange}
        browsePluginId={browsePluginId}
        browseDisabled={browseDisabled}
        browseDisabledReason={browseDisabledReason}
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

  if (fieldType === 'string' && browsePluginId && isBrowsablePathField(fieldKey)) {
    return (
      <BrowsableStringField
        fieldKey={fieldKey}
        field={field}
        value={stringValue}
        onChange={handleChange}
        isRequired={isRequired}
        browsePluginId={browsePluginId}
        browseDisabled={browseDisabled}
        browseDisabledReason={browseDisabledReason}
      />
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

function BrowsableStringField({
  fieldKey,
  field,
  value,
  onChange,
  isRequired,
  browsePluginId,
  browseDisabled,
  browseDisabledReason,
}: {
  fieldKey: string
  field: PluginConfigField
  value: string
  onChange: (value: string) => void
  isRequired: boolean
  browsePluginId: string
  browseDisabled: boolean
  browseDisabledReason?: string
}) {
  const [showBrowser, setShowBrowser] = useState(false)
  return (
    <div className="space-y-2">
      <div className="grid gap-2 sm:grid-cols-[1fr_auto] sm:items-end">
        <Input
          label={displayLabel(fieldKey) + (isRequired ? ' *' : '')}
          value={value}
          onChange={(e) => onChange(e.target.value)}
          placeholder={field.description ?? (field.default != null ? String(field.default) : undefined)}
        />
        <Button
          type="button"
          variant="outline"
          size="sm"
          disabled={browseDisabled}
          onClick={() => setShowBrowser((open) => !open)}
        >
          {showBrowser ? 'Hide Browse' : 'Browse'}
        </Button>
      </div>
      {browseDisabled && browseDisabledReason && (
        <p className="text-xs text-amber-200">{browseDisabledReason}</p>
      )}
      {showBrowser && (
        <div className="rounded-mga border border-mga-border bg-mga-surface/70 p-3">
          <FolderBrowser
            pluginId={browsePluginId}
            initialPath={value}
            onSelect={(path) => {
              onChange(path)
              setShowBrowser(false)
            }}
          />
        </div>
      )}
      <FieldHint field={field} />
    </div>
  )
}

function StringPathsField({
  fieldKey,
  field,
  value,
  onChange,
  browsePluginId,
  browseDisabled = false,
  browseDisabledReason,
}: {
  fieldKey: string
  field: PluginConfigField
  value: unknown
  onChange: (key: string, value: unknown) => void
  browsePluginId?: string | null
  browseDisabled?: boolean
  browseDisabledReason?: string
}) {
  const [browserIndex, setBrowserIndex] = useState<number | null>(null)
  const paths = normalizeStringPathsValue(value)
  const update = (next: string[]) => onChange(fieldKey, next)
  const setPath = (index: number, path: string) => {
    update(paths.map((entry, current) => (current === index ? path : entry)))
  }
  const remove = (index: number) => update(paths.filter((_, current) => current !== index))
  return (
    <div className="space-y-3">
      <div className="flex items-center justify-between">
        <span className="text-sm font-medium text-mga-text">{displayLabel(fieldKey)}</span>
        <button type="button" onClick={() => update([...paths, ''])} className="text-xs text-mga-accent hover:underline">
          Add excluded folder
        </button>
      </div>
      {paths.length === 0 ? (
        <p className="text-xs text-mga-muted">No folders excluded.</p>
      ) : paths.map((entry, index) => (
        <div key={`${index}:${entry}`} className="rounded-mga border border-mga-border p-3 space-y-3">
          <div className="flex gap-2 items-start">
            <div className="flex-1">
              <Input
                label={`Folder ${index + 1}`}
                value={entry}
                onChange={(e) => setPath(index, e.target.value)}
                placeholder="Folder to skip recursively"
              />
            </div>
            <button type="button" onClick={() => remove(index)} className="text-xs text-red-300 hover:text-red-200 mt-7">
              Remove
            </button>
          </div>
          {browsePluginId && (
            <div className="space-y-2">
              <Button
                type="button"
                variant="outline"
                size="sm"
                disabled={browseDisabled}
                onClick={() => setBrowserIndex(browserIndex === index ? null : index)}
              >
                {browserIndex === index ? 'Hide browser' : 'Browse...'}
              </Button>
              {browseDisabled && browseDisabledReason && (
                <p className="text-xs text-amber-200">{browseDisabledReason}</p>
              )}
              {browserIndex === index && (
                <FolderBrowser
                  pluginId={browsePluginId}
                  initialPath={entry}
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

function IncludePathsField({
  fieldKey,
  field,
  value,
  onChange,
  browsePluginId,
  browseDisabled = false,
  browseDisabledReason,
}: {
  fieldKey: string
  field: PluginConfigField
  value: unknown
  onChange: (key: string, value: unknown) => void
  browsePluginId?: string | null
  browseDisabled?: boolean
  browseDisabledReason?: string
}) {
  const [browserIndex, setBrowserIndex] = useState<number | null>(null)
  const [excludeBrowser, setExcludeBrowser] = useState<{ includeIndex: number; excludeIndex: number } | null>(null)
  const [excludeError, setExcludeError] = useState<Record<string, string>>({})
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

  const setExcludePath = (includeIndex: number, excludeIndex: number, path: string) => {
    update(includePaths.map((entry, current) => {
      if (current !== includeIndex) return entry
      const excludes = [...(entry.exclude_paths ?? [])]
      excludes[excludeIndex] = path
      return { ...entry, exclude_paths: excludes }
    }))
  }

  const addExclude = (includeIndex: number) => {
    update(includePaths.map((entry, current) => (
      current === includeIndex
        ? { ...entry, exclude_paths: [...(entry.exclude_paths ?? []), ''] }
        : entry
    )))
  }

  const removeExclude = (includeIndex: number, excludeIndex: number) => {
    update(includePaths.map((entry, current) => {
      if (current !== includeIndex) return entry
      return { ...entry, exclude_paths: (entry.exclude_paths ?? []).filter((_, index) => index !== excludeIndex) }
    }))
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
              <Button
                type="button"
                variant="outline"
                size="sm"
                disabled={browseDisabled}
                onClick={() => setBrowserIndex(browserIndex === index ? null : index)}
              >
                {browserIndex === index ? 'Hide browser' : 'Browse...'}
              </Button>
              {browseDisabled && browseDisabledReason && (
                <p className="text-xs text-amber-200">{browseDisabledReason}</p>
              )}
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

          <div className="rounded-mga border border-mga-border/70 bg-black/10 p-3 space-y-3">
            <div className="flex items-start justify-between gap-3">
              <div>
                <p className="text-sm font-medium text-mga-text">Excluded folders</p>
                <p className="text-xs text-mga-muted">Folders skipped inside this include path.</p>
              </div>
              <Button type="button" variant="outline" size="sm" onClick={() => addExclude(index)}>
                Add excluded folder
              </Button>
            </div>

            {(entry.exclude_paths ?? []).length === 0 ? (
              <p className="text-xs text-mga-muted">No folders excluded for this include path.</p>
            ) : (
              <div className="space-y-3">
                {(entry.exclude_paths ?? []).map((excludePath, excludeIndex) => {
                  const errorKey = `${index}:${excludeIndex}`
                  const browserOpen = excludeBrowser?.includeIndex === index && excludeBrowser.excludeIndex === excludeIndex
                  return (
                    <div key={errorKey} className="space-y-2">
                      <div className="grid gap-2 sm:grid-cols-[1fr_auto_auto] sm:items-end">
                        <Input
                          label={`Exclude ${excludeIndex + 1}`}
                          value={excludePath}
                          onChange={(e) => setExcludePath(index, excludeIndex, e.target.value)}
                          placeholder={entry.path ? `${entry.path}/folder-to-skip` : 'Folder to skip recursively'}
                        />
                        {browsePluginId && (
                          <Button
                            type="button"
                            variant="outline"
                            size="sm"
                            disabled={browseDisabled}
                            onClick={() => setExcludeBrowser(browserOpen ? null : { includeIndex: index, excludeIndex })}
                          >
                            {browserOpen ? 'Hide Browse' : 'Browse'}
                          </Button>
                        )}
                        <Button type="button" variant="ghost" size="sm" onClick={() => removeExclude(index, excludeIndex)}>
                          Remove
                        </Button>
                      </div>
                      {excludeError[errorKey] && <p className="text-xs text-red-300">{excludeError[errorKey]}</p>}
                      {browserOpen && browsePluginId && (
                        <FolderBrowser
                          pluginId={browsePluginId}
                          initialPath={excludePath || entry.path}
                          onSelect={(path) => {
                            if (!pathInsideInclude(path, entry.path)) {
                              setExcludeError((prev) => ({
                                ...prev,
                                [errorKey]: `Excluded folder must be inside ${entry.path || '(root)'}.`,
                              }))
                              return
                            }
                            setExcludeError((prev) => {
                              const next = { ...prev }
                              delete next[errorKey]
                              return next
                            })
                            setExcludePath(index, excludeIndex, path)
                            setExcludeBrowser(null)
                          }}
                        />
                      )}
                    </div>
                  )
                })}
              </div>
            )}
          </div>
        </div>
      ))}

      <FieldHint field={field} />
    </div>
  )
}

function normalizeIncludePathsValue(value: unknown): FilesystemIncludePath[] {
  if (Array.isArray(value)) {
    const includePaths = value
      .map((entry): FilesystemIncludePath | null => {
        if (!entry || typeof entry !== 'object') return null
        const item = entry as Record<string, unknown>
        return {
          path: typeof item.path === 'string' ? item.path : '',
          recursive: typeof item.recursive === 'boolean' ? item.recursive : true,
          exclude_paths: normalizeStringPathsValue(item.exclude_paths),
        }
      })
      .filter((entry): entry is FilesystemIncludePath => entry !== null)
    if (includePaths.length > 0) {
      return includePaths
    }
  }
  return [{ path: '', recursive: true }]
}

function pathInsideInclude(path: string, includePath: string): boolean {
  const normalizedPath = normalizeLogicalPath(path)
  const normalizedInclude = normalizeLogicalPath(includePath)
  return normalizedInclude === '' || normalizedPath === normalizedInclude || normalizedPath.startsWith(`${normalizedInclude}/`)
}

function normalizeLogicalPath(path: string): string {
  return path.trim().replaceAll('\\', '/').replace(/^\/+|\/+$/g, '')
}

function normalizeStringPathsValue(value: unknown): string[] {
  if (!Array.isArray(value)) return []
  return value
    .map((entry) => (typeof entry === 'string' ? entry : ''))
    .filter((entry, index, all) => entry !== '' || index === all.length - 1)
}

function isBrowsablePathField(fieldKey: string): boolean {
  return fieldKey === 'root_path' || fieldKey === 'sync_path' || fieldKey.endsWith('_path')
}

function displayLabel(fieldKey: string): string {
  return fieldKey
    .split('_')
    .filter(Boolean)
    .map((part) => part.charAt(0).toUpperCase() + part.slice(1))
    .join(' ')
}
