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

type ViewModeOption = { value: LibraryPrefs['viewMode']; label: string }

const DEFAULT_VIEW_OPTIONS: ViewModeOption[] = [
  { value: 'shelf', label: 'Shelves' },
  { value: 'grid', label: 'Covers' },
]

const GROUP_OPTIONS: Array<{ value: LibraryPrefs['groupBy']; label: string }> = [
  { value: 'none', label: 'No grouping' },
  { value: 'platform', label: 'Platform' },
  { value: 'integration', label: 'Connection' },
  { value: 'play_method', label: 'Play option' },
  { value: 'achievements', label: 'Achievements' },
  { value: 'year', label: 'Release year' },
]

// ---------------------------------------------------------------------------
// Component
// ---------------------------------------------------------------------------

interface LibraryToolbarProps {
  title: string
  subtitle?: string
  totalCount: number
  filteredCount: number
  isLoading?: boolean
  viewMode: LibraryPrefs['viewMode']
  onViewModeChange: (mode: LibraryPrefs['viewMode']) => void
  groupBy: LibraryPrefs['groupBy']
  onGroupByChange: (groupBy: LibraryPrefs['groupBy']) => void
  sortBy: LibraryPrefs['sortBy']
  sortDir: LibraryPrefs['sortDir']
  onSortChange: (by: LibraryPrefs['sortBy'], dir: LibraryPrefs['sortDir']) => void
  addButtonLabel?: string
  showAddButton?: boolean
  onAddButtonClick?: () => void
  filterBarOpen: boolean
  onFilterBarToggle: () => void
  activeFilterCount: number
  showViewToggle?: boolean
  showGrouping?: boolean
  viewModeOptions?: ViewModeOption[]
}

export function LibraryToolbar({
  title,
  subtitle,
  totalCount,
  filteredCount,
  isLoading = false,
  viewMode,
  onViewModeChange,
  groupBy,
  onGroupByChange,
  sortBy,
  sortDir,
  onSortChange,
  addButtonLabel = 'Add shelf',
  showAddButton = true,
  onAddButtonClick,
  filterBarOpen,
  onFilterBarToggle,
  activeFilterCount,
  showViewToggle = true,
  showGrouping = true,
  viewModeOptions = DEFAULT_VIEW_OPTIONS,
}: LibraryToolbarProps) {
  return (
    <div className="flex flex-wrap items-center gap-3">
      {/* Title + counts */}
      <div>
        <h1 className="text-2xl font-semibold tracking-tight">{title}</h1>
        {subtitle && <p className="text-sm text-mga-muted">{subtitle}</p>}
      </div>
      <span className="text-sm text-mga-muted">
        {isLoading
          ? 'Loading games...'
          : filteredCount === totalCount
          ? `${totalCount} games`
          : `${filteredCount} of ${totalCount}`}
      </span>

      {/* Spacer */}
      <div className="flex-1" />

      {/* Sort */}
      <div className="flex items-center gap-1.5">
        <select
          aria-label="Sort games"
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
          aria-label={sortDir === 'asc' ? 'Sort ascending' : 'Sort descending'}
          onClick={() => onSortChange(sortBy, sortDir === 'asc' ? 'desc' : 'asc')}
          className="rounded-mga border border-mga-border bg-mga-bg px-2 py-1.5 text-xs text-mga-muted hover:text-mga-text"
          title={sortDir === 'asc' ? 'Ascending' : 'Descending'}
        >
          {sortDir === 'asc' ? '\u25B2' : '\u25BC'}
        </button>
      </div>

      {showGrouping ? (
        <select
          value={groupBy}
          onChange={(event) => onGroupByChange(event.target.value as LibraryPrefs['groupBy'])}
          aria-label="Group games"
          className="rounded-mga border border-mga-border bg-mga-bg px-2 py-1.5 text-xs text-mga-text"
        >
          {GROUP_OPTIONS.map((option) => (
            <option key={option.value} value={option.value}>
              {option.label}
            </option>
          ))}
        </select>
      ) : null}

      {/* View mode toggle */}
      {showViewToggle && (
        <ToggleGroup
          value={viewMode}
          onChange={onViewModeChange}
          options={viewModeOptions}
        />
      )}

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
