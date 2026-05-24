import { useCallback, useEffect, useMemo, useRef, useState } from 'react'
import { ArrowRightLeft, Plus, Trash2 } from 'lucide-react'
import { Navigate, useLocation, useNavigate, useParams } from 'react-router-dom'
import { useQueryClient } from '@tanstack/react-query'
import { useSearch } from '@/hooks/useSearchContext'
import { useLibraryData } from '@/hooks/useLibraryData'
import { useLibraryPrefs } from '@/hooks/useLibraryPrefs'
import { CollectionShelf } from '@/components/library/CollectionShelf'
import { HorizontalGameShelf } from '@/components/library/HorizontalGameShelf'
import { LibraryToolbar } from '@/components/library/LibraryToolbar'
import { FilterBar } from '@/components/library/FilterBar'
import { GameGrid } from '@/components/library/GameGrid'
import { GameListView } from '@/components/library/GameListView'
import { SectionPickerDialog } from '@/components/library/SectionPickerDialog'
import { Button } from '@/components/ui/button'
import { Dialog } from '@/components/ui/dialog'
import { BatchSourceHardDeleteDialog, type BatchSourceDeleteTarget } from '@/components/library/BatchSourceHardDeleteDialog'
import { useRecentPlayed } from '@/hooks/useRecentPlayed'
import { buildBulkReclassifySearchParams, writeBulkReclassifyQueue, type BulkReclassifyQueueItem } from '@/lib/bulkReclassifyQueue'
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
import type { GameDetailResponse, LibraryPrefs } from '@/api/client'

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

const LIBRARY_VIEW_OPTIONS: Array<{ value: LibraryPrefs['viewMode']; label: string }> = [
  { value: 'shelf', label: 'Shelf' },
  { value: 'grid', label: 'Grid' },
  { value: 'list', label: 'List' },
  { value: 'timeline', label: 'Timeline' },
]

const PLAY_VIEW_OPTIONS: Array<{ value: LibraryPrefs['viewMode']; label: string }> = [
  { value: 'shelf', label: 'Shelf' },
  { value: 'grid', label: 'Grid' },
  { value: 'timeline', label: 'Timeline' },
]

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

function listParam(params: URLSearchParams, key: string): string[] {
  return params
    .getAll(key)
    .flatMap((value) => value.split(','))
    .map((value) => value.trim())
    .filter(Boolean)
}

function filterStateFromSearch(search: string): FilterState {
  const params = new URLSearchParams(search)
  const decade = params.get('decade')?.trim() ?? ''
  const decadeStart = /^\d{4}s?$/.test(decade) ? Number(decade.substring(0, 4)) : null

  return {
    ...DEFAULT_FILTER_STATE,
    platforms: listParam(params, 'platform'),
    genres: listParam(params, 'genre'),
    source: params.get('source')?.trim() ?? '',
    integration: params.get('integration')?.trim() ?? '',
    yearMin: decadeStart,
    yearMax: decadeStart === null ? null : decadeStart + 9,
  }
}

function releaseYear(game: GameDetailResponse): string {
  const value = game.release_date?.substring(0, 4)
  return value && /^\d{4}$/.test(value) ? value : 'Unknown release date'
}

function sourceRecordLabel(game: GameDetailResponse): string {
  const source = game.source_games[0]
  if (!source) return game.title
  return `${source.integration_label || source.integration_id} · ${source.raw_title || source.external_id}`
}

type BulkActionKind = 'reclassify' | 'hard_delete'

type BulkActionReview = {
  kind: BulkActionKind
  title: string
  actionLabel: string
  eligibleCount: number
  skipped: string[]
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
  const queryClient = useQueryClient()
  const { searchQuery } = useSearch()
  const loadMoreRef = useRef<HTMLDivElement | null>(null)
  const loadMoreRequestedRef = useRef(false)
  const {
    data: allGames = [],
    totalCount,
    loadedCount,
    hasNextPage,
    fetchNextPage,
    isFetchingNextPage,
    isPending,
    isError,
    error,
  } = useLibraryData()
  const { recentPlayed, removeRecentPlayed } = useRecentPlayed()
  const {
    prefs,
    setViewMode,
    setSortBy,
    setSortDir,
    setSections,
    setExpandedSectionId,
  } = useLibraryPrefs(scope)
  const effectiveViewMode = scope === 'play' && prefs.viewMode === 'list' ? 'shelf' : prefs.viewMode

  // Local filter state (not persisted — session only)
  const [filterState, setFilterState] = useState<FilterState>(() => filterStateFromSearch(location.search))
  const [filterBarOpen, setFilterBarOpen] = useState(false)
  const [sectionPickerOpen, setSectionPickerOpen] = useState(false)
  const [selectedGameIds, setSelectedGameIds] = useState<Set<string>>(() => new Set())
  const [bulkDeleteOpen, setBulkDeleteOpen] = useState(false)
  const [bulkActionReview, setBulkActionReview] = useState<BulkActionReview | null>(null)
  const [bulkNotice, setBulkNotice] = useState('')
  const [bulkWarnings, setBulkWarnings] = useState<string[]>([])

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
  const displayedGameIds = useMemo(() => new Set(displayedGames.map((game) => game.id)), [displayedGames])
  const selectedGames = useMemo(
    () => displayedGames.filter((game) => selectedGameIds.has(game.id)),
    [displayedGames, selectedGameIds],
  )
  const bulkReclassifyGames = useMemo(
    () => selectedGames.filter((game) => game.source_games.length === 1),
    [selectedGames],
  )
  const bulkReclassifySkipped = useMemo(
    () =>
      selectedGames
        .filter((game) => game.source_games.length !== 1)
        .map((game) => `${game.title}: ${game.source_games.length === 0 ? 'no source record' : `${game.source_games.length} source records`}`),
    [selectedGames],
  )
  const bulkReclassifyEnabled = bulkReclassifyGames.length > 0
  const bulkHardDeleteGames = useMemo(
    () => selectedGames.filter((game) => game.source_games.length === 1 && game.source_games[0].hard_delete?.eligible === true),
    [selectedGames],
  )
  const bulkHardDeleteSkipped = useMemo(
    () =>
      selectedGames
        .filter((game) => !(game.source_games.length === 1 && game.source_games[0].hard_delete?.eligible === true))
        .map((game) => {
          if (game.source_games.length !== 1) {
            return `${game.title}: ${game.source_games.length === 0 ? 'no source record' : `${game.source_games.length} source records`}`
          }
          return `${game.title}: ${game.source_games[0].hard_delete?.reason || 'source cannot be hard deleted'}`
        }),
    [selectedGames],
  )
  const bulkHardDeleteEnabled = bulkHardDeleteGames.length > 0
  const bulkDeleteTargets = useMemo<BatchSourceDeleteTarget[]>(
    () =>
      bulkHardDeleteGames.map((game) => ({
        key: game.source_games[0].id,
        canonicalGameId: game.id,
        sourceGameId: game.source_games[0].id,
        sourceLabel: sourceRecordLabel(game),
        gameTitle: game.title,
      })),
    [bulkHardDeleteGames],
  )

  // Facets for filter bar (derived from full section list, not filtered subset)
  const availablePlatforms = useMemo(() => filter.allPlatforms(), [filter])
  const availableGenres = useMemo(() => filter.allGenres(), [filter])
  const availableSources = useMemo(() => filter.allSources(), [filter])
  const availableIntegrations = useMemo(() => {
    const labels = new Map<string, string>()
    for (const game of filteredScopeGames) {
      for (const source of game.source_games ?? []) {
        if (!labels.has(source.integration_id)) {
          labels.set(source.integration_id, source.integration_label || source.integration_id)
        }
      }
    }
    return Array.from(labels.entries())
      .map(([id, label]) => ({ id, label }))
      .sort((a, b) => a.label.localeCompare(b.label))
  }, [filteredScopeGames])
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
    if (filterState.integration) n++
    if (filterState.playableOnly) n++
    if (filterState.xcloudOnly) n++
    if (filterState.gamePassOnly) n++
    return n
  }, [filterState])

  const scopeMeta = SCOPES[scope]
  const showLibraryShelfAddButton = scope === 'library' && effectiveViewMode === 'shelf' && !focusedSection
  const emptyMessage = focusedSection
    ? 'No games in this section match the current filters.'
    : scopeMeta.emptyMessage
  const canUseRemoteTotal = !focusedSection && scope === 'library' && activeFilterCount === 0 && searchQuery.trim() === ''
  const toolbarTotalCount = canUseRemoteTotal ? Math.max(totalCount, filteredScopeGames.length) : filteredScopeGames.length
  const filterRequiresAllPages = activeFilterCount > 0
  const searchRequiresAllPages = searchQuery.trim().length > 0
  const isCompletingSearch = searchRequiresAllPages && hasNextPage
  const isCompletingFilter = !searchRequiresAllPages && filterRequiresAllPages && hasNextPage
  const isClosedShelfOverview = effectiveViewMode === 'shelf' && !focusedSection
  const showLoadMoreSentinel = hasNextPage && (!isClosedShelfOverview || searchRequiresAllPages || filterRequiresAllPages)

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

  const toggleSelectedGame = (game: GameDetailResponse) => {
    setBulkNotice('')
    setBulkWarnings([])
    setSelectedGameIds((prev) => {
      const next = new Set(prev)
      if (next.has(game.id)) next.delete(game.id)
      else next.add(game.id)
      return next
    })
  }

  const selectLoadedVisibleGames = () => {
    setBulkNotice('')
    setBulkWarnings([])
    setSelectedGameIds(new Set(displayedGames.map((game) => game.id)))
  }

  const clearSelection = () => {
    setSelectedGameIds(new Set())
    setBulkWarnings([])
  }

  const startBulkReclassify = () => {
    if (!bulkReclassifyEnabled) return
    const queue: BulkReclassifyQueueItem[] = bulkReclassifyGames.map((game) => ({
      gameId: game.id,
      sourceGameId: game.source_games[0].id,
      title: game.title,
      platform: game.platform,
      pluginId: game.source_games[0].plugin_id,
    }))
    writeBulkReclassifyQueue(queue)
    const first = queue[0]
    navigate({ pathname: '/settings', search: buildBulkReclassifySearchParams(first).toString() })
  }

  const requestBulkReclassify = () => {
    if (!bulkReclassifyEnabled) return
    if (bulkReclassifySkipped.length > 0) {
      setBulkActionReview({
        kind: 'reclassify',
        title: 'Continue Bulk Reclassify?',
        actionLabel: 'Continue',
        eligibleCount: bulkReclassifyGames.length,
        skipped: bulkReclassifySkipped,
      })
      return
    }
    startBulkReclassify()
  }

  const requestBulkHardDelete = () => {
    if (!bulkHardDeleteEnabled) return
    if (bulkHardDeleteSkipped.length > 0) {
      setBulkActionReview({
        kind: 'hard_delete',
        title: 'Continue Bulk Hard Delete?',
        actionLabel: 'Continue',
        eligibleCount: bulkHardDeleteGames.length,
        skipped: bulkHardDeleteSkipped,
      })
      return
    }
    setBulkDeleteOpen(true)
  }

  const continueReviewedBulkAction = () => {
    const review = bulkActionReview
    setBulkActionReview(null)
    if (!review) return
    if (review.kind === 'reclassify') {
      startBulkReclassify()
    } else {
      setBulkDeleteOpen(true)
    }
  }

  const handleBulkDeleteCompleted = async (deleted: BatchSourceDeleteTarget[], warnings: string[]) => {
    const deletedIds = new Set(deleted.map((target) => target.sourceGameId))
    const affectedCanonicalIds = Array.from(new Set(deleted.map((target) => target.canonicalGameId)))
    setSelectedGameIds((prev) => {
      const next = new Set(prev)
      for (const game of selectedGames) {
        if (game.source_games.some((source) => deletedIds.has(source.id))) next.delete(game.id)
      }
      return next
    })
    const warningSuffix = warnings.length
      ? ` ${warnings.length} directory cleanup warning${warnings.length === 1 ? '' : 's'}.`
      : ''
    setBulkNotice(`Deleted ${deleted.length} source record${deleted.length === 1 ? '' : 's'}.${warningSuffix}`)
    setBulkWarnings(warnings)
    await Promise.all([
      queryClient.invalidateQueries({ queryKey: ['games'] }),
      queryClient.invalidateQueries({ queryKey: ['duplicate-games'] }),
      queryClient.invalidateQueries({ queryKey: ['stats'] }),
      queryClient.invalidateQueries({ queryKey: ['library-statistics'] }),
      queryClient.invalidateQueries({ queryKey: ['gamer-statistics'] }),
      queryClient.invalidateQueries({ queryKey: ['achievements'] }),
      queryClient.invalidateQueries({ queryKey: ['cache-entries'] }),
      queryClient.invalidateQueries({ queryKey: ['cache-jobs'] }),
      ...affectedCanonicalIds.flatMap((canonicalId) => [
        queryClient.invalidateQueries({ queryKey: ['game', canonicalId] }),
        queryClient.invalidateQueries({ queryKey: ['game', canonicalId, 'achievements'] }),
      ]),
    ])
  }

  const requestNextPage = useCallback(() => {
    if (!hasNextPage || isFetchingNextPage || loadMoreRequestedRef.current) return
    loadMoreRequestedRef.current = true
    void fetchNextPage().finally(() => {
      loadMoreRequestedRef.current = false
    })
  }, [fetchNextPage, hasNextPage, isFetchingNextPage])

  useEffect(() => {
    if (!isFetchingNextPage) {
      loadMoreRequestedRef.current = false
    }
  }, [isFetchingNextPage])

  useEffect(() => {
    if ((!searchRequiresAllPages && !filterRequiresAllPages) || !hasNextPage) return
    requestNextPage()
  }, [filterRequiresAllPages, hasNextPage, loadedCount, requestNextPage, searchRequiresAllPages])

  useEffect(() => {
    setSelectedGameIds((prev) => {
      if (effectiveViewMode !== 'list' || focusedSection || scope !== 'library') {
        return prev.size === 0 ? prev : new Set()
      }
      const next = new Set<string>()
      for (const id of prev) {
        if (displayedGameIds.has(id)) next.add(id)
      }
      return next.size === prev.size ? prev : next
    })
  }, [displayedGameIds, effectiveViewMode, focusedSection, scope])

  useEffect(() => {
    setFilterState(filterStateFromSearch(location.search))
    setFilterBarOpen(location.search.length > 1)
  }, [location.search])

  useEffect(() => {
    if (!showLoadMoreSentinel || isPending) return
    const sentinel = loadMoreRef.current
    if (!sentinel) return

    const observer = new IntersectionObserver(
      (entries) => {
        if (entries.some((entry) => entry.isIntersecting)) {
          requestNextPage()
        }
      },
      {
        root: null,
        rootMargin: '720px 0px',
        threshold: 0,
      },
    )

    observer.observe(sentinel)
    return () => observer.disconnect()
  }, [displayedGames.length, isPending, requestNextPage, showLoadMoreSentinel])

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
        totalCount={toolbarTotalCount}
        filteredCount={displayedGames.length}
        isLoading={isPending || isCompletingSearch}
        viewMode={focusedSection ? 'grid' : effectiveViewMode}
        onViewModeChange={setViewMode}
        viewModeOptions={scope === 'library' ? LIBRARY_VIEW_OPTIONS : PLAY_VIEW_OPTIONS}
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
        availableIntegrations={availableIntegrations}
        yearRange={yearRange}
        isOpen={filterBarOpen}
      />

      {bulkNotice ? (
        <div className="rounded-mga border border-emerald-500/30 bg-emerald-500/10 px-4 py-3 text-sm text-emerald-100">
          {bulkNotice}
        </div>
      ) : null}

      {bulkWarnings.length ? (
        <div className="space-y-1 rounded-mga border border-amber-400/30 bg-amber-500/10 px-4 py-3 text-sm text-amber-100">
          {bulkWarnings.map((warning) => (
            <p key={warning}>{warning}</p>
          ))}
        </div>
      ) : null}

      {scope === 'library' && effectiveViewMode === 'list' && !focusedSection && selectedGames.length > 0 ? (
        <div className="sticky top-[calc(var(--mga-app-header-height,7rem)+0.75rem)] z-30 rounded-mga border border-mga-accent/30 bg-mga-surface/95 p-3 shadow-xl shadow-black/20 backdrop-blur">
          <div className="flex flex-wrap items-center justify-between gap-3">
            <div>
              <p className="text-sm font-medium text-mga-text">
                {selectedGames.length} loaded game{selectedGames.length === 1 ? '' : 's'} selected
              </p>
              <p className="mt-1 text-xs text-mga-muted">
                Source-level bulk actions will use eligible selected games and warn before skipping the rest.
              </p>
            </div>
            <div className="flex flex-wrap gap-2">
              <Button type="button" variant="outline" size="sm" onClick={selectLoadedVisibleGames}>
                Select loaded visible rows
              </Button>
              <Button type="button" variant="outline" size="sm" onClick={clearSelection}>
                Clear
              </Button>
              <Button
                type="button"
                variant="outline"
                size="sm"
                onClick={requestBulkReclassify}
                disabled={!bulkReclassifyEnabled}
                title={
                  bulkReclassifyEnabled
                    ? `${bulkReclassifyGames.length} selected game${bulkReclassifyGames.length === 1 ? '' : 's'} can be reclassified.`
                    : 'No selected games can be reclassified.'
                }
              >
                <ArrowRightLeft size={14} />
                Reclassify
              </Button>
              <Button
                type="button"
                variant="outline"
                size="sm"
                onClick={requestBulkHardDelete}
                disabled={!bulkHardDeleteEnabled}
                title={
                  bulkHardDeleteEnabled
                    ? `${bulkHardDeleteGames.length} selected source${bulkHardDeleteGames.length === 1 ? '' : 's'} can be hard deleted.`
                    : 'No selected sources can be hard deleted.'
                }
                className="border-red-500/30 text-red-200 hover:bg-red-500/10"
              >
                <Trash2 size={14} />
                Hard Delete
              </Button>
            </div>
          </div>
          {bulkReclassifySkipped.length > 0 || bulkHardDeleteSkipped.length > 0 ? (
            <p className="mt-2 text-xs text-amber-300">
              Reclassify: {bulkReclassifyGames.length} eligible, {bulkReclassifySkipped.length} skipped. Hard delete: {bulkHardDeleteGames.length} eligible, {bulkHardDeleteSkipped.length} skipped.
            </p>
          ) : null}
        </div>
      ) : null}

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
      ) : effectiveViewMode === 'grid' ? (
        <div className="space-y-8">
          <RecentPlayedShelf games={recentPlayedGames} onRemove={removeRecentPlayed} />
          <GameGrid
            games={displayedGames}
            isLoading={isPending}
            cardVariant={scope === 'play' ? 'play' : 'library'}
          />
        </div>
      ) : effectiveViewMode === 'list' && scope === 'library' ? (
        <GameListView
          games={displayedGames}
          selectedIds={selectedGameIds}
          onToggleSelected={toggleSelectedGame}
        />
      ) : effectiveViewMode === 'timeline' ? (
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
            {isCompletingSearch
              ? 'Searching all loaded and unloaded games...'
              : isCompletingFilter
              ? 'Filtering all loaded and unloaded games...'
              : searchQuery || activeFilterCount > 0
              ? 'No games match your filters.'
              : emptyMessage}
          </p>
        </div>
      )}

      {!isPending && !isError && showLoadMoreSentinel ? (
        <div ref={loadMoreRef} className="flex flex-col items-center gap-2 py-6" aria-live="polite">
          <div className="h-1 w-1" aria-hidden="true" />
          <p className="text-xs text-mga-muted">
            {searchRequiresAllPages
              ? `Searching all games: ${loadedCount} of ${totalCount} loaded.`
              : filterRequiresAllPages
              ? `Filtering all games: ${loadedCount} of ${totalCount} loaded.`
              : isFetchingNextPage
              ? 'Loading more games...'
              : `Scroll to load more games. ${loadedCount} of ${totalCount} loaded.`}
          </p>
        </div>
      ) : null}

      <SectionPickerDialog
        open={sectionPickerOpen}
        onClose={() => setSectionPickerOpen(false)}
        games={filteredScopeGames}
        existingSections={prefs.sections}
        onAddSections={handleAddSections}
      />

      <Dialog
        open={bulkActionReview !== null}
        onClose={() => setBulkActionReview(null)}
        title={bulkActionReview?.title ?? 'Continue Bulk Action?'}
      >
        {bulkActionReview ? (
          <div className="space-y-4">
            <p className="text-sm text-mga-muted">
              {bulkActionReview.eligibleCount} selected game{bulkActionReview.eligibleCount === 1 ? '' : 's'} support this operation. The following selected game{bulkActionReview.skipped.length === 1 ? '' : 's'} will be skipped:
            </p>
            <div className="max-h-72 space-y-2 overflow-auto rounded-mga border border-amber-400/25 bg-amber-500/10 p-3 text-sm text-amber-100">
              {bulkActionReview.skipped.map((item) => (
                <p key={item} className="break-words">{item}</p>
              ))}
            </div>
            <div className="flex justify-end gap-3">
              <Button type="button" variant="outline" onClick={() => setBulkActionReview(null)}>
                Cancel
              </Button>
              <Button type="button" onClick={continueReviewedBulkAction}>
                {bulkActionReview.actionLabel}
              </Button>
            </div>
          </div>
        ) : null}
      </Dialog>

      <BatchSourceHardDeleteDialog
        open={bulkDeleteOpen}
        title="Apply Library Hard Delete"
        description="Review every backing file below before applying deletion. This removes the selected one-source library records using the same hard-delete flow as game details and duplicate cleanup."
        confirmCopy="I reviewed the listed files and want to hard delete the selected library source records."
        targets={bulkDeleteTargets}
        onClose={() => setBulkDeleteOpen(false)}
        onDeleted={handleBulkDeleteCompleted}
      />
    </div>
  )
}

export function LibraryPage() {
  return <CollectionPage scope="library" />
}
