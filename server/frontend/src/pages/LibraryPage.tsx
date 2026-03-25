import { useMemo, useState } from 'react'
import { useSearch } from '@/hooks/useSearchContext'
import { useLibraryData } from '@/hooks/useLibraryData'
import { useLibraryPrefs } from '@/hooks/useLibraryPrefs'
import { LibraryToolbar } from '@/components/library/LibraryToolbar'
import { FilterBar } from '@/components/library/FilterBar'
import { GameGrid } from '@/components/library/GameGrid'
import { GameList } from '@/components/library/GameList'
import {
  applySectionFilter,
  DEFAULT_FILTER_STATE,
  LibraryFilter,
  type FilterState,
  type LibrarySection,
} from '@/lib/libraryFilter'

// ---------------------------------------------------------------------------
// Section metadata
// ---------------------------------------------------------------------------

const SECTIONS: Record<LibrarySection, { title: string; subtitle: string; emptyMessage: string }> = {
  all: {
    title: 'Library',
    subtitle: 'All games in your collection',
    emptyMessage: 'No games in the library yet. Run a scan from the server.',
  },
  playable: {
    title: 'Playable',
    subtitle: 'Browser-emulatable games',
    emptyMessage: 'No playable games found. Scan for GBA, PS1, Arcade, ScummVM, or DOS games.',
  },
  xcloud: {
    title: 'xCloud',
    subtitle: 'Xbox Game Pass cloud-playable games',
    emptyMessage: 'No xCloud games found. Add an Xbox integration and scan.',
  },
}

// ---------------------------------------------------------------------------
// Component
// ---------------------------------------------------------------------------

interface LibraryPageProps {
  section?: LibrarySection
}

export function LibraryPage({ section = 'all' }: LibraryPageProps) {
  const { searchQuery } = useSearch()
  const { data: allGames = [], isPending, isError, error } = useLibraryData()
  const { prefs, setViewMode, setSortBy, setSortDir } = useLibraryPrefs()

  // Local filter state (not persisted — session only)
  const [filterState, setFilterState] = useState<FilterState>(DEFAULT_FILTER_STATE)
  const [filterBarOpen, setFilterBarOpen] = useState(false)

  // Section pre-filter (e.g. playable-only, xcloud-only)
  const sectionGames = useMemo(
    () => applySectionFilter(allGames, section),
    [allGames, section],
  )

  // Build filter engine from the section-filtered games
  const filter = useMemo(() => new LibraryFilter(sectionGames), [sectionGames])

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

  const sectionMeta = SECTIONS[section]

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
        title={sectionMeta.title}
        subtitle={sectionMeta.subtitle}
        totalCount={sectionGames.length}
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
              : sectionMeta.emptyMessage}
          </p>
        </div>
      )}
    </div>
  )
}
