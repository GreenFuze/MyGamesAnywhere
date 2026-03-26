import { useMemo, useState } from 'react'
import { useSearch } from '@/hooks/useSearchContext'
import { useLibraryData } from '@/hooks/useLibraryData'
import { useLibraryPrefs } from '@/hooks/useLibraryPrefs'
import { LibraryToolbar } from '@/components/library/LibraryToolbar'
import { FilterBar } from '@/components/library/FilterBar'
import { GameGrid } from '@/components/library/GameGrid'
import { GameList } from '@/components/library/GameList'
import {
  applyScopeFilter,
  DEFAULT_FILTER_STATE,
  LibraryFilter,
  type FilterState,
  type CollectionScope,
} from '@/lib/libraryFilter'

// ---------------------------------------------------------------------------
// Section metadata
// ---------------------------------------------------------------------------

const SCOPES: Record<CollectionScope, { title: string; subtitle: string; emptyMessage: string }> = {
  library: {
    title: 'Library',
    subtitle: 'All games in your collection',
    emptyMessage: 'No games in the library yet. Run a scan from the server.',
  },
  play: {
    title: 'Play',
    subtitle: 'Browser-emulatable and xCloud-ready games',
    emptyMessage: 'No actionable games found. Add sources and run a scan.',
  },
}

// ---------------------------------------------------------------------------
// Component
// ---------------------------------------------------------------------------

interface CollectionPageProps {
  scope: CollectionScope
}

export function CollectionPage({ scope }: CollectionPageProps) {
  const { searchQuery } = useSearch()
  const { data: allGames = [], isPending, isError, error } = useLibraryData()
  const { prefs, setViewMode, setSortBy, setSortDir } = useLibraryPrefs(scope)

  // Local filter state (not persisted — session only)
  const [filterState, setFilterState] = useState<FilterState>(DEFAULT_FILTER_STATE)
  const [filterBarOpen, setFilterBarOpen] = useState(false)

  const scopeGames = useMemo(
    () => applyScopeFilter(allGames, scope),
    [allGames, scope],
  )

  const filter = useMemo(() => new LibraryFilter(scopeGames), [scopeGames])

  // Apply user filters + search + sort
  const displayedGames = useMemo(
    () =>
      filter.apply({
        ...filterState,
        search: searchQuery,
        sortBy: prefs.sortBy,
        sortDir: prefs.sortDir,
      }),
    [filter, filterState, searchQuery, prefs.sortBy, prefs.sortDir],
  )

  // Facets for filter bar (derived from full section list, not filtered subset)
  const availablePlatforms = useMemo(() => filter.allPlatforms(), [filter])
  const availableGenres = useMemo(() => filter.allGenres(), [filter])
  const availableSources = useMemo(() => filter.allSources(), [filter])
  const yearRange = useMemo(() => filter.yearRange(), [filter])

  // Active filter count
  const activeFilterCount = useMemo(() => {
    let n = 0
    if (filterState.platforms.length > 0) n++
    if (filterState.genres.length > 0) n++
    if (filterState.yearMin !== null || filterState.yearMax !== null) n++
    if (filterState.developer) n++
    if (filterState.publisher) n++
    if (filterState.source) n++
    if (filterState.playableOnly) n++
    if (filterState.xcloudOnly) n++
    if (filterState.gamePassOnly) n++
    return n
  }, [filterState])

  const scopeMeta = SCOPES[scope]

  // Patch filter state (merge partial updates)
  const patchFilter = (patch: Partial<FilterState>) => {
    setFilterState((prev) => ({ ...prev, ...patch }))
  }

  // Sort change handler
  const handleSortChange = (by: typeof prefs.sortBy, dir: typeof prefs.sortDir) => {
    setSortBy(by)
    setSortDir(dir)
  }

  return (
    <div className="space-y-4">
      {/* Toolbar: title, counts, sort, view toggle, filters button */}
      <LibraryToolbar
        title={scopeMeta.title}
        subtitle={scopeMeta.subtitle}
        totalCount={scopeGames.length}
        filteredCount={displayedGames.length}
        viewMode={prefs.viewMode}
        onViewModeChange={setViewMode}
        sortBy={prefs.sortBy}
        sortDir={prefs.sortDir}
        onSortChange={handleSortChange}
        filterBarOpen={filterBarOpen}
        onFilterBarToggle={() => setFilterBarOpen((v) => !v)}
        activeFilterCount={activeFilterCount}
      />

      {/* Filter bar (collapsible) */}
      <FilterBar
        state={filterState}
        onChange={patchFilter}
        availablePlatforms={availablePlatforms}
        availableGenres={availableGenres}
        availableSources={availableSources}
        yearRange={yearRange}
        isOpen={filterBarOpen}
        onToggle={() => setFilterBarOpen((v) => !v)}
      />

      {/* Content */}
      {isError && (
        <p className="text-sm text-red-400">Error: {(error as Error).message}</p>
      )}

      {prefs.viewMode === 'grid' ? (
        <GameGrid games={displayedGames} isLoading={isPending} />
      ) : (
        <GameList
          games={displayedGames}
          isLoading={isPending}
          sortBy={prefs.sortBy}
          sortDir={prefs.sortDir}
          onSortChange={handleSortChange}
        />
      )}

      {/* Empty state */}
      {!isPending && !isError && displayedGames.length === 0 && (
        <div className="py-12 text-center">
          <p className="text-mga-muted">
            {searchQuery || activeFilterCount > 0
              ? 'No games match your filters.'
              : scopeMeta.emptyMessage}
          </p>
        </div>
      )}
    </div>
  )
}

export function LibraryPage() {
  return <CollectionPage scope="library" />
}
