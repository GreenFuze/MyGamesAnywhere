import { useEffect, useMemo, useState } from 'react'
import { Plus } from 'lucide-react'
import { useLocation } from 'react-router-dom'
import { useSearch } from '@/hooks/useSearchContext'
import { useLibraryData } from '@/hooks/useLibraryData'
import { useLibraryPrefs } from '@/hooks/useLibraryPrefs'
import { CollectionShelf } from '@/components/library/CollectionShelf'
import { LibraryToolbar } from '@/components/library/LibraryToolbar'
import { FilterBar } from '@/components/library/FilterBar'
import { GameGrid } from '@/components/library/GameGrid'
import { SectionPickerDialog } from '@/components/library/SectionPickerDialog'
import { Button } from '@/components/ui/button'
import { sanitizeSections } from '@/lib/collectionSections'
import {
  applyScopeFilter,
  DEFAULT_FILTER_STATE,
  LibraryFilter,
  type FilterState,
  type CollectionScope,
} from '@/lib/libraryFilter'
import { consumeStoredRouteScroll, shouldRestoreRouteScroll } from '@/lib/gameNavigation'
import type { GameDetailResponse } from '@/api/client'

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
    subtitle: 'Browser-ready and xCloud-ready games',
    emptyMessage: 'No actionable games found. Add sources and run a scan.',
  },
}

// ---------------------------------------------------------------------------
// Component
// ---------------------------------------------------------------------------

interface CollectionPageProps {
  scope: CollectionScope
}

function releaseYear(game: GameDetailResponse): string {
  const value = game.release_date?.substring(0, 4)
  return value && /^\d{4}$/.test(value) ? value : 'Unknown release date'
}

function TimelineView({ games }: { games: GameDetailResponse[] }) {
  const groups = useMemo(() => {
    const map = new Map<string, GameDetailResponse[]>()
    for (const game of games) {
      const year = releaseYear(game)
      map.set(year, [...(map.get(year) ?? []), game])
    }
    return Array.from(map.entries()).sort(([a], [b]) => {
      if (a === 'Unknown release date') return 1
      if (b === 'Unknown release date') return -1
      return Number(b) - Number(a)
    })
  }, [games])

  return (
    <div className="space-y-8">
      {groups.map(([year, yearGames]) => (
        <section key={year} className="space-y-3">
          <div className="flex items-baseline gap-3 border-b border-mga-border pb-2">
            <h2 className="text-2xl font-semibold tracking-tight">{year}</h2>
            <span className="text-sm text-mga-muted">{yearGames.length} games</span>
          </div>
          <GameGrid games={yearGames} isLoading={false} />
        </section>
      ))}
    </div>
  )
}

export function CollectionPage({ scope }: CollectionPageProps) {
  const location = useLocation()
  const { searchQuery } = useSearch()
  const { data: allGames = [], isPending, isError, error } = useLibraryData()
  const {
    prefs,
    setViewMode,
    setSortBy,
    setSortDir,
    setSections,
    setExpandedSectionId,
  } = useLibraryPrefs(scope)

  // Local filter state (not persisted — session only)
  const [filterState, setFilterState] = useState<FilterState>(DEFAULT_FILTER_STATE)
  const [filterBarOpen, setFilterBarOpen] = useState(false)
  const [sectionPickerOpen, setSectionPickerOpen] = useState(false)

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
  const showLibraryShelfAddButton = scope === 'library' && prefs.viewMode === 'shelf'

  // Patch filter state (merge partial updates)
  const patchFilter = (patch: Partial<FilterState>) => {
    setFilterState((prev) => ({ ...prev, ...patch }))
  }

  // Sort change handler
  const handleSortChange = (by: typeof prefs.sortBy, dir: typeof prefs.sortDir) => {
    setSortBy(by)
    setSortDir(dir)
  }

  const handleAddSections = (sectionsToAdd: typeof prefs.sections) => {
    setSections(sanitizeSections([...prefs.sections, ...sectionsToAdd]))
  }

  const handleRemoveSection = (sectionID: string) => {
    const next = sanitizeSections(prefs.sections.filter((section) => section.id !== sectionID))
    setSections(next)
    if (prefs.expandedSectionId === sectionID || !next.some((section) => section.id === prefs.expandedSectionId)) {
      setExpandedSectionId(null)
    }
  }

  useEffect(() => {
    if (isPending || !shouldRestoreRouteScroll(location.state)) return

    const nextScroll = consumeStoredRouteScroll(location.pathname, location.search)
    if (nextScroll === null) return

    const frame = window.requestAnimationFrame(() => {
      window.scrollTo({ top: nextScroll, behavior: 'auto' })
    })

    return () => window.cancelAnimationFrame(frame)
  }, [isPending, location.pathname, location.search, location.state])

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
        addButtonLabel="Add Section"
        showAddButton={scope !== 'library'}
        onAddButtonClick={() => setSectionPickerOpen(true)}
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
      />

      {/* Content */}
      {isError && (
        <p className="text-sm text-red-400">Error: {(error as Error).message}</p>
      )}

      {prefs.viewMode === 'grid' ? (
        <GameGrid games={displayedGames} isLoading={isPending} />
      ) : prefs.viewMode === 'timeline' ? (
        <TimelineView games={displayedGames} />
      ) : (
        <div className="space-y-6">
          <CollectionShelf
            sections={prefs.sections}
            expandedSectionId={prefs.expandedSectionId}
            onExpandedSectionChange={setExpandedSectionId}
            onRemoveSection={handleRemoveSection}
            games={displayedGames}
            isLoading={isPending}
          />
          {showLibraryShelfAddButton && (
            <div className="flex justify-center">
              <Button
                type="button"
                variant="outline"
                size="icon"
                onClick={() => setSectionPickerOpen(true)}
                aria-label="Add shelf"
                title="Add shelf"
                className="h-11 w-11 rounded-full"
              >
                <Plus size={18} />
              </Button>
            </div>
          )}
        </div>
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

      <SectionPickerDialog
        open={sectionPickerOpen}
        onClose={() => setSectionPickerOpen(false)}
        games={scopeGames}
        existingSections={prefs.sections}
        onAddSections={handleAddSections}
      />
    </div>
  )
}

export function LibraryPage() {
  return <CollectionPage scope="library" />
}
