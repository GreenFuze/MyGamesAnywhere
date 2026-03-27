import { Badge } from '@/components/ui/badge'
import { Button } from '@/components/ui/button'
import { pluginLabel } from '@/lib/gameUtils'
import type { FilterState } from '@/lib/libraryFilter'
import { cn } from '@/lib/utils'

// ---------------------------------------------------------------------------
// Props
// ---------------------------------------------------------------------------

interface FilterBarProps {
  state: FilterState
  onChange: (patch: Partial<FilterState>) => void
  availablePlatforms: string[]
  availableGenres: string[]
  availableSources: string[]
  yearRange: [number, number] | null
  isOpen: boolean
}

// ---------------------------------------------------------------------------
// Helpers
// ---------------------------------------------------------------------------

function toggleInArray(arr: string[], item: string): string[] {
  return arr.includes(item) ? arr.filter((v) => v !== item) : [...arr, item]
}

function countActiveFilters(state: FilterState): number {
  let n = 0
  if (state.platforms.length > 0) n++
  if (state.genres.length > 0) n++
  if (state.yearMin !== null || state.yearMax !== null) n++
  if (state.developer) n++
  if (state.publisher) n++
  if (state.source) n++
  if (state.playableOnly) n++
  if (state.xcloudOnly) n++
  if (state.gamePassOnly) n++
  return n
}

// ---------------------------------------------------------------------------
// Component
// ---------------------------------------------------------------------------

export function FilterBar({
  state,
  onChange,
  availablePlatforms,
  availableGenres,
  availableSources,
  yearRange,
  isOpen,
}: FilterBarProps) {
  const activeCount = countActiveFilters(state)

  const clearAll = () => {
    onChange({
      platforms: [],
      genres: [],
      yearMin: null,
      yearMax: null,
      developer: '',
      publisher: '',
      source: '',
      playableOnly: false,
      xcloudOnly: false,
      gamePassOnly: false,
    })
  }

  if (!isOpen) return null

  return (
    <div className="space-y-4 rounded-mga border border-mga-border bg-mga-surface p-4">
      {activeCount > 0 && (
        <div className="flex items-center justify-end gap-2">
          <Badge variant="accent">{activeCount} active</Badge>
          <Button variant="ghost" size="sm" onClick={clearAll}>
            Clear all
          </Button>
        </div>
      )}

          {/* Platforms */}
          {availablePlatforms.length > 0 && (
            <FilterSection label="Platform">
              <div className="flex flex-wrap gap-1.5">
                {availablePlatforms.map((p) => (
                  <button
                    key={p}
                    type="button"
                    onClick={() => onChange({ platforms: toggleInArray(state.platforms, p) })}
                    className={cn(
                      'rounded-mga border px-2 py-1 text-xs font-medium transition-colors',
                      state.platforms.includes(p)
                        ? 'border-mga-accent bg-mga-accent/20 text-mga-accent'
                        : 'border-mga-border bg-mga-bg text-mga-muted hover:text-mga-text',
                    )}
                  >
                    {p}
                  </button>
                ))}
              </div>
            </FilterSection>
          )}

          {/* Genres */}
          {availableGenres.length > 0 && (
            <FilterSection label="Genre">
              <div className="flex flex-wrap gap-1.5">
                {availableGenres.map((g) => (
                  <button
                    key={g}
                    type="button"
                    onClick={() => onChange({ genres: toggleInArray(state.genres, g) })}
                    className={cn(
                      'rounded-mga border px-2 py-1 text-xs font-medium transition-colors',
                      state.genres.includes(g)
                        ? 'border-mga-accent bg-mga-accent/20 text-mga-accent'
                        : 'border-mga-border bg-mga-bg text-mga-muted hover:text-mga-text',
                    )}
                  >
                    {g}
                  </button>
                ))}
              </div>
            </FilterSection>
          )}

          {/* Year range */}
          {yearRange && (
            <FilterSection label="Year">
              <div className="flex items-center gap-2">
                <input
                  type="number"
                  placeholder={String(yearRange[0])}
                  value={state.yearMin ?? ''}
                  onChange={(e) =>
                    onChange({ yearMin: e.target.value ? Number(e.target.value) : null })
                  }
                  className="w-20 rounded-mga border border-mga-border bg-mga-bg px-2 py-1 text-sm text-mga-text placeholder:text-mga-muted"
                />
                <span className="text-mga-muted">\u2013</span>
                <input
                  type="number"
                  placeholder={String(yearRange[1])}
                  value={state.yearMax ?? ''}
                  onChange={(e) =>
                    onChange({ yearMax: e.target.value ? Number(e.target.value) : null })
                  }
                  className="w-20 rounded-mga border border-mga-border bg-mga-bg px-2 py-1 text-sm text-mga-text placeholder:text-mga-muted"
                />
              </div>
            </FilterSection>
          )}

          {/* Developer / Publisher text inputs */}
          <div className="flex flex-wrap gap-4">
            <FilterSection label="Developer">
              <input
                type="text"
                placeholder="Filter by developer..."
                value={state.developer}
                onChange={(e) => onChange({ developer: e.target.value })}
                className="w-48 rounded-mga border border-mga-border bg-mga-bg px-2 py-1 text-sm text-mga-text placeholder:text-mga-muted"
              />
            </FilterSection>
            <FilterSection label="Publisher">
              <input
                type="text"
                placeholder="Filter by publisher..."
                value={state.publisher}
                onChange={(e) => onChange({ publisher: e.target.value })}
                className="w-48 rounded-mga border border-mga-border bg-mga-bg px-2 py-1 text-sm text-mga-text placeholder:text-mga-muted"
              />
            </FilterSection>
          </div>

          {/* Source */}
          {availableSources.length > 0 && (
            <FilterSection label="Source">
              <div className="flex flex-wrap gap-1.5">
                <button
                  type="button"
                  onClick={() => onChange({ source: '' })}
                  className={cn(
                    'rounded-mga border px-2 py-1 text-xs font-medium transition-colors',
                    !state.source
                      ? 'border-mga-accent bg-mga-accent/20 text-mga-accent'
                      : 'border-mga-border bg-mga-bg text-mga-muted hover:text-mga-text',
                  )}
                >
                  All
                </button>
                {availableSources.map((s) => (
                  <button
                    key={s}
                    type="button"
                    onClick={() => onChange({ source: state.source === s ? '' : s })}
                    className={cn(
                      'rounded-mga border px-2 py-1 text-xs font-medium transition-colors',
                      state.source === s
                        ? 'border-mga-accent bg-mga-accent/20 text-mga-accent'
                        : 'border-mga-border bg-mga-bg text-mga-muted hover:text-mga-text',
                    )}
                  >
                    {pluginLabel(s)}
                  </button>
                ))}
              </div>
            </FilterSection>
          )}

          {/* Flag toggles */}
          <FilterSection label="Show only">
            <div className="flex flex-wrap gap-3">
              <ToggleChip
                label="Playable"
                active={state.playableOnly}
                onClick={() => onChange({ playableOnly: !state.playableOnly })}
              />
              <ToggleChip
                label="xCloud"
                active={state.xcloudOnly}
                onClick={() => onChange({ xcloudOnly: !state.xcloudOnly })}
              />
              <ToggleChip
                label="Game Pass"
                active={state.gamePassOnly}
                onClick={() => onChange({ gamePassOnly: !state.gamePassOnly })}
              />
            </div>
          </FilterSection>
    </div>
  )
}

// ---------------------------------------------------------------------------
// Sub-components
// ---------------------------------------------------------------------------

function FilterSection({
  label,
  children,
}: {
  label: string
  children: React.ReactNode
}) {
  return (
    <div>
      <p className="mb-1.5 text-xs font-medium uppercase tracking-wider text-mga-muted">
        {label}
      </p>
      {children}
    </div>
  )
}

function ToggleChip({
  label,
  active,
  onClick,
}: {
  label: string
  active: boolean
  onClick: () => void
}) {
  return (
    <button
      type="button"
      onClick={onClick}
      className={cn(
        'rounded-mga border px-2.5 py-1 text-xs font-medium transition-colors',
        active
          ? 'border-mga-accent bg-mga-accent/20 text-mga-accent'
          : 'border-mga-border bg-mga-bg text-mga-muted hover:text-mga-text',
      )}
    >
      {label}
    </button>
  )
}
