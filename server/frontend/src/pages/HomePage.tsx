import { useQuery } from '@tanstack/react-query'
import { Link } from 'react-router-dom'
import { BarChart3, Heart, Library, Settings, Trophy } from 'lucide-react'
import { getGamerStatistics, getLibraryStatistics } from '@/api/client'

function formatNumber(value: number | undefined): string {
  return new Intl.NumberFormat().format(value ?? 0)
}

function percentText(value: number | undefined): string {
  return `${Math.round(value ?? 0)}%`
}

function HomeCard({
  title,
  value,
  detail,
  to,
  action,
  icon: Icon,
}: {
  title: string
  value: string
  detail: string
  to: string
  action: string
  icon: typeof Library
}) {
  return (
    <Link
      to={to}
      className="group rounded-mga border border-mga-border bg-mga-surface p-4 shadow-sm shadow-black/10 transition-colors hover:border-mga-accent/70 hover:bg-mga-elevated/40"
    >
      <div className="flex items-start justify-between gap-3">
        <div className="min-w-0">
          <p className="text-xs font-medium uppercase tracking-[0.16em] text-mga-muted">{title}</p>
          <p className="mt-2 text-3xl font-semibold text-mga-text">{value}</p>
        </div>
        <span className="rounded-mga bg-mga-accent/10 p-2 text-mga-accent">
          <Icon className="h-5 w-5" />
        </span>
      </div>
      <p className="mt-3 text-sm leading-6 text-mga-muted">{detail}</p>
      <p className="mt-4 text-sm font-medium text-mga-accent transition-opacity group-hover:opacity-80">{action}</p>
    </Link>
  )
}

function EmptyHome() {
  return (
    <section className="rounded-mga border border-dashed border-mga-border bg-mga-surface p-8 text-center shadow-sm shadow-black/10">
      <h2 className="text-lg font-semibold text-mga-text">Your library is ready for its first scan</h2>
      <p className="mx-auto mt-2 max-w-2xl text-sm leading-6 text-mga-muted">
        Add a source integration, run a scan, and Home will summarize your library and player progress.
      </p>
      <div className="mt-4">
        <Link
          to="/settings"
          className="inline-flex rounded-mga border border-mga-accent/40 bg-mga-accent/10 px-4 py-2 text-sm font-medium text-mga-accent hover:bg-mga-accent/20"
        >
          Configure integrations
        </Link>
      </div>
    </section>
  )
}

export function HomePage() {
  const libraryQuery = useQuery({
    queryKey: ['stats', 'library'],
    queryFn: getLibraryStatistics,
  })
  const gamerQuery = useQuery({
    queryKey: ['stats', 'gamer'],
    queryFn: getGamerStatistics,
  })
  const library = libraryQuery.data
  const gamer = gamerQuery.data
  const totalGames = library?.summary.canonical_game_count ?? 0

  return (
    <div className="space-y-6">
      <section className="overflow-hidden rounded-mga border border-mga-border bg-mga-surface shadow-sm shadow-black/20">
        <div className="p-5 md:p-6">
          <img
            src="/title.png"
            alt="MyGamesAnywhere"
            width={1200}
            height={400}
            className="block h-auto w-full max-w-2xl object-contain object-left"
          />
          <p className="mt-4 max-w-2xl text-sm leading-7 text-mga-muted">
            A concise view of your library health, progress, and next useful actions.
          </p>
          <div className="mt-5 flex flex-wrap gap-3">
            <Link
              to="/library"
              className="inline-flex rounded-mga border border-mga-accent/40 bg-mga-accent/10 px-4 py-2 text-sm font-medium text-mga-accent hover:bg-mga-accent/20"
            >
              Open library
            </Link>
            <Link
              to="/stats/library"
              className="inline-flex rounded-mga border border-mga-border px-4 py-2 text-sm font-medium text-mga-text hover:bg-mga-hover"
            >
              View stats
            </Link>
          </div>
        </div>
      </section>

      {libraryQuery.isPending || gamerQuery.isPending ? (
        <section className="rounded-mga border border-mga-border bg-mga-surface p-6 text-sm text-mga-muted shadow-sm shadow-black/10">
          Loading dashboard...
        </section>
      ) : null}

      {libraryQuery.isError || gamerQuery.isError ? (
        <section className="rounded-mga border border-red-500/30 bg-red-500/10 p-6 text-sm text-red-300 shadow-sm shadow-black/10">
          Failed to load dashboard: {libraryQuery.error?.message || gamerQuery.error?.message}
        </section>
      ) : null}

      {!libraryQuery.isPending && !libraryQuery.isError && totalGames === 0 ? <EmptyHome /> : null}

      {library && gamer && totalGames > 0 ? (
        <section className="grid gap-3 md:grid-cols-2 xl:grid-cols-5">
          <HomeCard
            title="Library"
            value={formatNumber(totalGames)}
            detail={`${formatNumber(library.summary.source_game_found_count)} visible source records`}
            to="/library"
            action="Browse games"
            icon={Library}
          />
          <HomeCard
            title="Library Stats"
            value={percentText(library.summary.percent_with_media)}
            detail="Games with artwork or media"
            to="/stats/library"
            action="Open library statistics"
            icon={BarChart3}
          />
          <HomeCard
            title="Gamer Stats"
            value={formatNumber(gamer.favorite_games)}
            detail="Favorite games in this profile"
            to="/stats/gamer"
            action="Open gamer statistics"
            icon={Heart}
          />
          <HomeCard
            title="Achievements"
            value={percentText(gamer.achievement_unlock_percent)}
            detail={`${formatNumber(gamer.unlocked_achievements)} of ${formatNumber(gamer.total_achievements)} unlocked`}
            to="/achievements"
            action="Open achievements"
            icon={Trophy}
          />
          <HomeCard
            title="Setup"
            value={formatNumber(library.source_integrations.length)}
            detail="Source integrations contributing games"
            to="/settings"
            action="Manage integrations"
            icon={Settings}
          />
        </section>
      ) : null}
    </div>
  )
}
