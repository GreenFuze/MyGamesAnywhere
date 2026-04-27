import { useEffect, useMemo, useState } from 'react'
import { Plus, Trash2 } from 'lucide-react'
import { Navigate, useLocation, useNavigate, useParams } from 'react-router-dom'
import { useSearch } from '@/hooks/useSearchContext'
import { useLibraryData } from '@/hooks/useLibraryData'
import { useLibraryPrefs } from '@/hooks/useLibraryPrefs'
import { CollectionShelf } from '@/components/library/CollectionShelf'
import { HorizontalGameShelf } from '@/components/library/HorizontalGameShelf'
import { LibraryToolbar } from '@/components/library/LibraryToolbar'
import { FilterBar } from '@/components/library/FilterBar'
import { GameGrid } from '@/components/library/GameGrid'
import { SectionPickerDialog } from '@/components/library/SectionPickerDialog'
import { Button } from '@/components/ui/button'
import { useRecentPlayed } from '@/hooks/useRecentPlayed'
import {
  createFavoritesSection,
  filterGamesBySection,
  sanitizeSections,
  type RuntimeCollectionSectionConfig,
} from '@/lib/collectionSections'
import {
  applyScopeFilter,
  DEFAULT_FILTER_STATE,
  LibraryFilter,
  type FilterState,
  type CollectionScope,
} from '@/lib/libraryFilter'
import {
  consumeStoredRouteScroll,
  readFocusRouteState,
  rememberRouteScroll,
  shouldRestoreRouteScroll,
} from '@/lib/gameNavigation'
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

function scopeBasePath(scope: CollectionScope): string {
  return scope === 'play' ? '/play' : '/library'
}

function scopeSectionPath(scope: CollectionScope, sectionID: string): string {
  return `${scopeBasePath(scope)}/section/${encodeURIComponent(sectionID)}`
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
          <GameGrid games={yearGames} isLoading={false} cardVariant="library" />
        </section>
      ))}
    </div>
  )
}

function RecentPlayedShelf({
  games,
  onRemove,
}: {
  games: GameDetailResponse[]
  onRemove: (gameID: string) => void
}) {
  if (games.length === 0) return null

  return (
    <section className="space-y-3">
      <div className="flex flex-wrap items-center justify-between gap-3">
        <div className="flex min-w-0 items-center gap-2 text-left">
          <h2 className="truncate text-2xl font-semibold tracking-tight text-mga-text">Recent Played</h2>
          <span className="text-sm text-mga-muted">{games.length}</span>
        </div>
      </div>
      <HorizontalGameShelf
        games={games}
        label="Recent Played"
        cardVariant="play"
        renderHoverAction={(game) => (
          <button
            type="button"
            onClick={(event) => {
              event.preventDefault()
              event.stopPropagation()
              if (window.confirm(`Are you sure you want to remove "${game.title}" from Recent Played?`)) {
                onRemove(game.id)
              }
            }}
            className="flex h-9 w-9 items-center justify-center rounded-full border border-mga-border bg-black/70 text-white backdrop-blur transition-colors hover:border-red-400/70 hover:text-red-300"
            aria-label={`Remove ${game.title} from recent played`}
            title="Remove from recent played"
          >
            <Trash2 size={15} />
          </button>
        )}
      />
    </section>
  )
}

export function CollectionPage({ scope }: CollectionPageProps) {
  const { sectionId } = useParams()
  const location = useLocation()
  const navigate = useNavigate()
  const { searchQuery } = useSearch()
  const { data: allGames = [], isPending, isError, error } = useLibraryData()
  const { recentPlayed, removeRecentPlayed } = useRecentPlayed()
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

  const scopeGames = useMemo(() => applyScopeFilter(allGames, scope), [allGames, scope])
  const basePath = scopeBasePath(scope)
  const sanitizedSections = useMemo(() => sanitizeSections(prefs.sections), [prefs.sections])
  const computedSections = useMemo<RuntimeCollectionSectionConfig[]>(() => {
    if (!scopeGames.some((game) => game.favorite)) return []
    return [createFavoritesSection()]
  }, [scopeGames])
  const runtimeSections = useMemo<RuntimeCollectionSectionConfig[]>(
    () => [...computedSections, ...sanitizedSections],
    [computedSections, sanitizedSections],
  )
  const focusedSection = useMemo<RuntimeCollectionSectionConfig | null>(() => {
    if (!sectionId) return null
    return runtimeSections.find((section) => section.id === sectionId) ?? null
  }, [runtimeSections, sectionId])
  const focusState = useMemo(() => readFocusRouteState(location.state), [location.state])
  const favoritesAvailableInScope = computedSections.length > 0

  const filteredScopeGames = useMemo(() => {
    if (!focusedSection) return scopeGames
    return filterGamesBySection(scopeGames, focusedSection)
  }, [focusedSection, scopeGames])

  const filter = useMemo(() => new LibraryFilter(filteredScopeGames), [filteredScopeGames])

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
  const showLibraryShelfAddButton = scope === 'library' && prefs.viewMode === 'shelf' && !focusedSection
  const emptyMessage = focusedSection
    ? 'No games in this section match the current filters.'
    : scopeMeta.emptyMessage

  const recentPlayedGames = useMemo(() => {
    if (scope !== 'play') return []
    return recentPlayed
      .map((entry) => {
        const game = allGames.find((candidate) => candidate.id === entry.gameId)
        if (!game) return null
        return {
          launchedAt: entry.launchedAt,
          game,
        }
      })
      .filter((entry): entry is { launchedAt: string; game: GameDetailResponse } => entry !== null)
      .sort((a, b) => b.launchedAt.localeCompare(a.launchedAt))
      .map((entry) => entry.game)
  }, [allGames, recentPlayed, scope])

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

  const openSection = (sectionID: string) => {
    const from = rememberRouteScroll(location.pathname, location.search)
    navigate(scopeSectionPath(scope, sectionID), { state: { from } })
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

  if (sectionId && !focusedSection) {
    return <Navigate to={basePath} replace />
  }

  return (
    <div className="space-y-4">
      {/* Toolbar: title, counts, sort, view toggle, filters button */}
      {focusedSection && (
        <Button
          type="button"
          variant="ghost"
          size="sm"
          onClick={() => navigate(focusState?.from ?? basePath, { state: { restoreScroll: true } })}
          className="w-fit"
        >
          Back to {scopeMeta.title}
        </Button>
      )}
      <LibraryToolbar
        title={focusedSection?.label ?? scopeMeta.title}
        subtitle={
          focusedSection
            ? `Filtered section view in ${scopeMeta.title}`
            : scopeMeta.subtitle
        }
        totalCount={filteredScopeGames.length}
        filteredCount={displayedGames.length}
        viewMode={focusedSection ? 'grid' : prefs.viewMode}
        onViewModeChange={setViewMode}
        sortBy={prefs.sortBy}
        sortDir={prefs.sortDir}
        onSortChange={handleSortChange}
        addButtonLabel="Add Section"
        showAddButton={scope !== 'library' && !focusedSection}
        onAddButtonClick={() => setSectionPickerOpen(true)}
        filterBarOpen={filterBarOpen}
        onFilterBarToggle={() => setFilterBarOpen((v) => !v)}
        activeFilterCount={activeFilterCount}
        showViewToggle={!focusedSection}
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

      {focusedSection ? (
        <div className="space-y-6">
          <GameGrid
            games={displayedGames}
            isLoading={isPending}
            progressive
            cardVariant={scope === 'play' ? 'play' : 'library'}
          />
        </div>
      ) : prefs.viewMode === 'grid' ? (
        <div className="space-y-8">
          <RecentPlayedShelf games={recentPlayedGames} onRemove={removeRecentPlayed} />
          <GameGrid
            games={displayedGames}
            isLoading={isPending}
            cardVariant={scope === 'play' ? 'play' : 'library'}
          />
        </div>
      ) : prefs.viewMode === 'timeline' ? (
        <div className="space-y-8">
          <RecentPlayedShelf games={recentPlayedGames} onRemove={removeRecentPlayed} />
          <TimelineView games={displayedGames} />
        </div>
      ) : (
        <div className="space-y-6">
          {scope === 'play' && favoritesAvailableInScope ? (
            <>
              <CollectionShelf
                sections={runtimeSections}
                onOpenSection={openSection}
                onRemoveSection={handleRemoveSection}
                games={displayedGames}
                isLoading={isPending}
                scope={scope}
              />
              <RecentPlayedShelf games={recentPlayedGames} onRemove={removeRecentPlayed} />
            </>
          ) : (
            <>
              <RecentPlayedShelf games={recentPlayedGames} onRemove={removeRecentPlayed} />
              <CollectionShelf
                sections={runtimeSections}
                onOpenSection={openSection}
                onRemoveSection={handleRemoveSection}
                games={displayedGames}
                isLoading={isPending}
                scope={scope}
              />
            </>
          )}
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
              : emptyMessage}
          </p>
        </div>
      )}

      <SectionPickerDialog
        open={sectionPickerOpen}
        onClose={() => setSectionPickerOpen(false)}
        games={filteredScopeGames}
        existingSections={prefs.sections}
        onAddSections={handleAddSections}
      />
    </div>
  )
}

export function LibraryPage() {
  return <CollectionPage scope="library" />
}
