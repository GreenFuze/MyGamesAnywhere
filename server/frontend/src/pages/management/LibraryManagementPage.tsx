import { useMemo, useState } from 'react'
import { Link } from 'react-router'
import { keepPreviousData, useQuery } from '@tanstack/react-query'
import { Gamepad2, LayoutGrid, Rows3, SearchX } from 'lucide-react'
import {
  getStats, listCatalogOffers, listGames,
  type CatalogOffer, type GameDetailResponse, type LibraryStats,
} from '@/api/client'
import { GameMediaCollection } from '@/lib/gameMedia'
import { platformLabel, sourceLabel } from '@/lib/displayText'
import { gameBadges, gameSourceNames, type GameBadge } from '@/lib/gameBadges'
import { readLibraryView, storeLibraryView, type LibraryView } from '@/lib/libraryView'
import { MetricCard, PageIntro, QueryFeedback, SectionCard, StatusPill, formatCount } from '@/components/management/ManagementPrimitives'

/** How many games arrive at once. "Show more" raises it rather than paging,
 *  because someone looking for one game scrolls; they do not want to remember
 *  which page it was on. */
const PAGE_STEP = 48

export function LibraryManagementPage() {
  const [shown, setShown] = useState(PAGE_STEP)
  // Detailed is the default. A wall of covers is pleasant and says almost
  // nothing: not the platform, not the source, not whether a game is in a
  // subscription or has gone missing. The covers are the other view.
  const [view, setView] = useState<LibraryView>(readLibraryView)

  const chooseView = (next: LibraryView) => {
    setView(next)
    storeLibraryView(next)
  }

  // The counts come from the library-wide stats endpoint, never from the rows
  // that happen to be loaded. A number that changes as you scroll is worse than
  // no number at all.
  const stats = useQuery({ queryKey: ['management', 'library', 'stats'], queryFn: () => getStats() })
  const library = useQuery({
    queryKey: ['management', 'library', { shown }],
    queryFn: () => listGames({ page: 0, page_size: shown, sort_by: 'title', sort_dir: 'asc' }),
    placeholderData: keepPreviousData,
  })
  // Availability is only known once a store or subscription source has been
  // scanned. No offers means no badge, never a guessed one.
  const offers = useQuery({ queryKey: ['management', 'catalog-offers'], queryFn: listCatalogOffers })

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
        <div className="mb-4 flex justify-end">
          <ViewToggle view={view} onChange={chooseView} />
        </div>

        <QueryFeedback
          pending={library.isPending}
          error={library.error}
          empty={!library.isPending && games.length === 0}
          emptyTitle="No games yet"
          emptyDescription="Connect a source and scan it, and your games will show up here."
        />

        {games.length > 0 && (
          <>
            {view === 'detailed' ? (
              <ul className="divide-y divide-mga-border/70 overflow-hidden rounded-lg border border-mga-border">
                {games.map((game) => <GameRow key={game.id} game={game} offers={offers.data} />)}
              </ul>
            ) : (
              <ul className="grid grid-cols-2 gap-4 sm:grid-cols-3 lg:grid-cols-4 xl:grid-cols-6">
                {games.map((game) => <GameCard key={game.id} game={game} offers={offers.data} />)}
              </ul>
            )}

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

function ViewToggle({ view, onChange }: { view: LibraryView; onChange: (next: LibraryView) => void }) {
  const options: { id: LibraryView; label: string; icon: React.ReactNode }[] = [
    { id: 'detailed', label: 'Detailed', icon: <Rows3 className="h-3.5 w-3.5" /> },
    { id: 'grid', label: 'Covers', icon: <LayoutGrid className="h-3.5 w-3.5" /> },
  ]
  return (
    <div className="inline-flex rounded-md border border-mga-border p-0.5" role="group" aria-label="How to show your games">
      {options.map((option) => (
        <button
          key={option.id}
          type="button"
          onClick={() => onChange(option.id)}
          aria-pressed={view === option.id}
          className={`inline-flex items-center gap-1.5 rounded px-2.5 py-1 text-xs transition ${
            view === option.id ? 'bg-mga-elevated text-mga-text' : 'text-mga-muted hover:text-mga-text'
          }`}
        >
          {option.icon}
          {option.label}
        </button>
      ))}
    </div>
  )
}

/** The detailed row: everything the covers cannot say. */
function GameRow({ game, offers }: { game: GameDetailResponse; offers?: CatalogOffer[] }) {
  const media = useMemo(() => new GameMediaCollection(game.media), [game.media])
  const cover = media.coverUrl()
  const badges = useMemo(() => gameBadges(game, offers), [game, offers])
  const sources = useMemo(() => gameSourceNames(game, sourceLabel), [game])
  const year = game.release_date ? new Date(game.release_date).getFullYear() : undefined

  return (
    <li>
      <Link
        to={`/library/game/${encodeURIComponent(game.id)}`}
        className="flex items-center gap-4 bg-mga-elevated/35 px-4 py-3 transition hover:bg-mga-elevated/70 focus:outline-none focus:ring-2 focus:ring-mga-accent/50"
      >
        <div className="h-14 w-11 shrink-0 overflow-hidden rounded border border-mga-border bg-mga-elevated">
          {cover ? <img src={cover} alt="" loading="lazy" className="h-full w-full object-cover" /> : <CoverPlaceholder title={game.title} compact />}
        </div>

        <div className="min-w-0 flex-1">
          <p className="truncate text-sm font-medium text-mga-text">{game.title}</p>
          <p className="mt-0.5 truncate text-xs text-mga-muted">
            {[platformLabel(game.platform || 'unknown'), year, game.developer].filter(Boolean).join(' · ')}
          </p>
          <div className="mt-1.5 flex flex-wrap items-center gap-1.5">
            {sources.map((source) => <StatusPill key={source} label={source} />)}
            <BadgeRow badges={badges} />
          </div>
        </div>
      </Link>
    </li>
  )
}

function GameCard({ game, offers }: { game: GameDetailResponse; offers?: CatalogOffer[] }) {
  const media = useMemo(() => new GameMediaCollection(game.media), [game.media])
  const cover = media.coverUrl()
  const badges = useMemo(() => gameBadges(game, offers), [game, offers])

  return (
    <li className="group overflow-hidden rounded-lg border border-mga-border bg-mga-elevated/35 transition hover:border-mga-accent/40">
      <Link to={`/library/game/${encodeURIComponent(game.id)}`} className="block focus:outline-none focus:ring-2 focus:ring-mga-accent/50">
        <div className="relative aspect-[3/4] w-full overflow-hidden bg-mga-elevated">
          {cover
            ? <img src={cover} alt="" loading="lazy" className="h-full w-full object-cover transition duration-300 group-hover:scale-[1.03]" />
            : <CoverPlaceholder title={game.title} />}
        </div>
        <div className="space-y-1.5 p-3">
          <p className="truncate text-sm font-medium text-mga-text" title={game.title}>{game.title}</p>
          <p className="truncate text-xs text-mga-muted">{platformLabel(game.platform || 'unknown')}</p>
          <div className="flex flex-wrap items-center gap-1.5 pt-0.5">
            {/* Only what changes what you would do. The row view has room for the rest. */}
            <BadgeRow badges={badges.filter((badge) => badge.tone !== 'neutral').slice(0, 3)} />
          </div>
        </div>
      </Link>
    </li>
  )
}

function BadgeRow({ badges }: { badges: GameBadge[] }) {
  return (
    <>
      {badges.map((badge) => (
        <span key={badge.id} title={badge.title}>
          <StatusPill label={badge.label} tone={badge.tone} />
        </span>
      ))}
    </>
  )
}

/** A title-derived tile, so a game with no artwork still reads as a game rather
 *  than as a hole in the list. */
function CoverPlaceholder({ title, compact = false }: { title: string; compact?: boolean }) {
  const initials = title.trim().split(/\s+/).slice(0, 2).map((word) => word.charAt(0).toUpperCase()).join('')
  return (
    <div className="flex h-full w-full items-center justify-center bg-gradient-to-br from-mga-elevated to-mga-surface">
      <span className={`font-semibold text-mga-muted ${compact ? 'text-xs' : 'text-2xl'}`}>{initials || '?'}</span>
    </div>
  )
}

/** Entries a scan found before and could not find last time. Reported only when
 *  the server gives both totals, so the card never shows a number derived from
 *  whichever rows this page happened to load. */
function missingFromSources(stats: LibraryStats | undefined) {
  if (!stats || stats.source_game_total_count === undefined || stats.source_game_found_count === undefined) return null
  const count = Math.max(stats.source_game_total_count - stats.source_game_found_count, 0)
  return { count }
}
