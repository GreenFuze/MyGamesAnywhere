import type { LibraryPrefs } from '@/api/client'
import { Badge } from '@/components/ui/badge'
import { Button } from '@/components/ui/button'
import { ToggleGroup } from '@/components/ui/toggle-group'

// ---------------------------------------------------------------------------
// Sort options
// ---------------------------------------------------------------------------

const SORT_OPTIONS: { value: LibraryPrefs['sortBy']; label: string }[] = [
  { value: 'title',        label: 'Title' },
  { value: 'release_date', label: 'Release Date' },
  { value: 'platform',     label: 'Platform' },
  { value: 'rating',       label: 'Rating' },
]

// ---------------------------------------------------------------------------
// Component
// ---------------------------------------------------------------------------

interface LibraryToolbarProps {
  title: string
  subtitle?: string
  totalCount: number
  filteredCount: number
  viewMode: LibraryPrefs['viewMode']
  onViewModeChange: (mode: LibraryPrefs['viewMode']) => void
  sortBy: LibraryPrefs['sortBy']
  sortDir: LibraryPrefs['sortDir']
  onSortChange: (by: LibraryPrefs['sortBy'], dir: LibraryPrefs['sortDir']) => void
  addButtonLabel?: string
  showAddButton?: boolean
  onAddButtonClick?: () => void
  filterBarOpen: boolean
  onFilterBarToggle: () => void
  activeFilterCount: number
}

export function LibraryToolbar({
  title,
  subtitle,
  totalCount,
  filteredCount,
  viewMode,
  onViewModeChange,
  sortBy,
  sortDir,
  onSortChange,
  addButtonLabel = 'Add Section',
  showAddButton = true,
  onAddButtonClick,
  filterBarOpen,
  onFilterBarToggle,
  activeFilterCount,
}: LibraryToolbarProps) {
  return (
    <div className="flex flex-wrap items-center gap-3">
      {/* Title + counts */}
      <div>
        <h1 className="text-2xl font-semibold tracking-tight">{title}</h1>
        {subtitle && <p className="text-sm text-mga-muted">{subtitle}</p>}
      </div>
      <span className="text-sm text-mga-muted">
        {filteredCount === totalCount
          ? `${totalCount} games`
          : `${filteredCount} of ${totalCount}`}
      </span>

      {/* Spacer */}
      <div className="flex-1" />

      {/* Sort */}
      <div className="flex items-center gap-1.5">
        <select
          value={sortBy}
          onChange={(e) => onSortChange(e.target.value as LibraryPrefs['sortBy'], sortDir)}
          className="rounded-mga border border-mga-border bg-mga-bg px-2 py-1.5 text-xs text-mga-text"
        >
          {SORT_OPTIONS.map((opt) => (
            <option key={opt.value} value={opt.value}>
              {opt.label}
            </option>
          ))}
        </select>
        <button
          type="button"
          onClick={() => onSortChange(sortBy, sortDir === 'asc' ? 'desc' : 'asc')}
          className="rounded-mga border border-mga-border bg-mga-bg px-2 py-1.5 text-xs text-mga-muted hover:text-mga-text"
          title={sortDir === 'asc' ? 'Ascending' : 'Descending'}
        >
          {sortDir === 'asc' ? '\u25B2' : '\u25BC'}
        </button>
      </div>

      {/* View mode toggle */}
      <ToggleGroup
        value={viewMode}
        onChange={onViewModeChange}
        options={[
          { value: 'shelf' as const, label: 'Shelf' },
          { value: 'grid' as const, label: 'Grid' },
        ]}
      />

      {showAddButton && onAddButtonClick && (
        <Button variant="outline" size="sm" onClick={onAddButtonClick}>
          {addButtonLabel}
        </Button>
      )}

      {/* Filters toggle */}
      <Button
        variant={filterBarOpen ? 'outline' : 'ghost'}
        size="sm"
        onClick={onFilterBarToggle}
      >
        Filters
        {activeFilterCount > 0 && (
          <Badge variant="accent" className="ml-1">
            {activeFilterCount}
          </Badge>
        )}
      </Button>
    </div>
  )
}
