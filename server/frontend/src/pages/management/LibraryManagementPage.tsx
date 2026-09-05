import { useMemo, useState } from 'react'
import { keepPreviousData, useQuery } from '@tanstack/react-query'
import { Gamepad2, SearchX } from 'lucide-react'
import { getStats, listGames, type GameDetailResponse, type LibraryStats } from '@/api/client'
import { GameMediaCollection } from '@/lib/gameMedia'
import { humanizeIdentifier, platformLabel, sourceLabel } from '@/lib/displayText'
import { MetricCard, PageIntro, QueryFeedback, SectionCard, StatusPill, formatCount } from '@/components/management/ManagementPrimitives'

/** How many games arrive at once. "Show more" raises it rather than paging,
 *  because someone looking for one game scrolls; they do not want to remember
 *  which page it was on. */
const PAGE_STEP = 48

export function LibraryManagementPage() {
  const [shown, setShown] = useState(PAGE_STEP)

  // The counts come from the library-wide stats endpoint, never from the rows
  // that happen to be loaded. A number that changes as you scroll is worse than
  // no number at all.
  const stats = useQuery({ queryKey: ['management', 'library', 'stats'], queryFn: () => getStats() })
  const library = useQuery({
    queryKey: ['management', 'library', { shown }],
    queryFn: () => listGames({ page: 0, page_size: shown, sort_by: 'title', sort_dir: 'asc' }),
    placeholderData: keepPreviousData,
  })

  const games = library.data?.games ?? []
  const total = library.data?.total ?? stats.data?.canonical_game_count
  const missing = missingFromSources(stats.data)
  const hasMore = total !== undefined && games.length < total

  return (
    <div className="mga-page-enter space-y-7">
      <PageIntro eyebrow="Library" title="Your games" description="Every game MGA has found, and where each one comes from." />

      <div className="grid gap-4 sm:grid-cols-2">
        <MetricCard label="Games" value={formatCount(total)} detail="Across all your connected sources" icon={<Gamepad2 className="h-4 w-4" />} />
        {missing !== null && (
          <MetricCard
            label="Missing right now"
            value={formatCount(missing.count)}
            detail={missing.count > 0 ? 'Found by an earlier scan, gone at the last one. A source may be offline.' : 'Everything found earlier is still there'}
            tone={missing.count > 0 ? 'attention' : 'good'}
            icon={<SearchX className="h-4 w-4" />}
          />
        )}
      </div>

      <SectionCard
        title="All games"
        description={total !== undefined && games.length > 0 ? `Showing ${formatCount(games.length)} of ${formatCount(total)}, sorted by name.` : undefined}
      >
        <QueryFeedback
          pending={library.isPending}
          error={library.error}
          empty={!library.isPending && games.length === 0}
          emptyTitle="No games yet"
          emptyDescription="Connect a source and scan it, and your games will show up here."
        />

        {games.length > 0 && (
          <>
            <ul className="grid grid-cols-2 gap-4 sm:grid-cols-3 lg:grid-cols-4 xl:grid-cols-6">
              {games.map((game) => <GameCard key={game.id} game={game} />)}
            </ul>
            {hasMore && (
              <div className="mt-6 flex justify-center">
                <button
                  type="button"
                  onClick={() => setShown((current) => current + PAGE_STEP)}
                  disabled={library.isFetching}
                  className="rounded-md border border-mga-border bg-mga-elevated/60 px-4 py-2 text-sm text-mga-text transition hover:bg-mga-elevated disabled:opacity-60"
                >
                  {library.isFetching ? 'Loading…' : `Show more (${formatCount((total ?? 0) - games.length)} left)`}
                </button>
              </div>
            )}
          </>
        )}
      </SectionCard>
    </div>
  )
}

function GameCard({ game }: { game: GameDetailResponse }) {
  const media = useMemo(() => new GameMediaCollection(game.media), [game.media])
  const cover = media.coverUrl()
  const sources = useMemo(() => sourceNames(game), [game])

  return (
    <li className="group overflow-hidden rounded-lg border border-mga-border bg-mga-elevated/35">
      <div className="relative aspect-[3/4] w-full overflow-hidden bg-mga-elevated">
        {cover
          ? <img src={cover} alt="" loading="lazy" className="h-full w-full object-cover transition duration-300 group-hover:scale-[1.03]" />
          : <CoverPlaceholder title={game.title} />}
      </div>
      <div className="space-y-1.5 p-3">
        <p className="truncate text-sm font-medium text-mga-text" title={game.title}>{game.title}</p>
        <p className="truncate text-xs text-mga-muted">{platformLabel(game.platform || 'unknown')}</p>
        <div className="flex flex-wrap items-center gap-1.5 pt-0.5">
          {sources.map((source) => <StatusPill key={source} label={source} />)}
          {!cover && <StatusPill label="No artwork" tone="attention" />}
          {game.kind && game.kind !== 'base_game' && <StatusPill label={humanizeIdentifier(game.kind)} />}
        </div>
      </div>
    </li>
  )
}

/** A title-derived tile, so a game with no artwork still reads as a game rather
 *  than as a hole in the grid. */
function CoverPlaceholder({ title }: { title: string }) {
  const initials = title.trim().split(/\s+/).slice(0, 2).map((word) => word.charAt(0).toUpperCase()).join('')
  return (
    <div className="flex h-full w-full items-center justify-center bg-gradient-to-br from-mga-elevated to-mga-surface">
      <span className="text-2xl font-semibold text-mga-muted">{initials || '?'}</span>
    </div>
  )
}

/** Which providers this game came from, named the way the user knows them, with
 *  duplicates collapsed: two Xbox entries for one game is our bookkeeping. */
function sourceNames(game: GameDetailResponse): string[] {
  const names = new Set<string>()
  for (const source of game.source_games ?? []) {
    const label = source.integration_label?.trim() || sourceLabel(source.plugin_id)
    if (label) names.add(label)
  }
  return [...names].sort()
}

/** Entries a scan found before and could not find last time. Reported only when
 *  the server gives both totals, so the card never shows a number derived from
 *  whichever rows this page happened to load. */
function missingFromSources(stats: LibraryStats | undefined) {
  if (!stats || stats.source_game_total_count === undefined || stats.source_game_found_count === undefined) return null
  const count = Math.max(stats.source_game_total_count - stats.source_game_found_count, 0)
  return { count }
}
