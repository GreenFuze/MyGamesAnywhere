import { useMemo } from 'react'
import { useQuery } from '@tanstack/react-query'
import { Link, NavLink, useLocation } from 'react-router-dom'
import {
  BarChart3,
  Clock3,
  Database,
  Heart,
  Library,
  ScanLine,
  Trophy,
} from 'lucide-react'
import {
  getGamerStatistics,
  getLibraryStatistics,
  type CountStat,
  type CoverageStat,
  type GamerStatistics,
  type LibraryStatistics,
  type ScanReport,
} from '@/api/client'
import { useRecentPlayed } from '@/hooks/useRecentPlayed'
import { platformLabel, pluginLabel, sourceLabel } from '@/lib/gameUtils'
import { cn } from '@/lib/utils'

const barColors = [
  'bg-sky-400',
  'bg-emerald-400',
  'bg-amber-300',
  'bg-rose-400',
  'bg-violet-400',
  'bg-cyan-300',
]

function formatNumber(value: number | undefined): string {
  return new Intl.NumberFormat().format(value ?? 0)
}

function percentText(value: number | undefined): string {
  return `${Math.round(value ?? 0)}%`
}

function formatStartedAt(value: string): string {
  const date = new Date(value)
  if (Number.isNaN(date.getTime())) return value
  return date.toLocaleString([], {
    month: 'short',
    day: 'numeric',
    hour: '2-digit',
    minute: '2-digit',
  })
}

function formatDuration(ms: number): string {
  if (ms < 1000) return `${ms}ms`
  const seconds = Math.round(ms / 1000)
  if (seconds < 60) return `${seconds}s`
  const minutes = Math.floor(seconds / 60)
  const remainder = seconds % 60
  return remainder > 0 ? `${minutes}m ${remainder}s` : `${minutes}m`
}

function platformStatLabel(item: CountStat): string {
  return platformLabel(item.key)
}

function pluginStatLabel(item: CountStat): string {
  return pluginLabel(item.key) || sourceLabel(item.key)
}

function identityStatLabel(item: CountStat): string {
  return item.label || item.key
}

function libraryFilterPath(key: string, value: string): string {
  const params = new URLSearchParams()
  params.set(key, value)
  return `/library?${params.toString()}`
}

function StatTile({
  label,
  value,
  detail,
  icon: Icon,
  tone = 'sky',
}: {
  label: string
  value: string | number
  detail: string
  icon: typeof BarChart3
  tone?: 'sky' | 'emerald' | 'amber' | 'rose' | 'violet'
}) {
  const toneClasses = {
    sky: 'bg-sky-400/10 text-sky-300',
    emerald: 'bg-emerald-400/10 text-emerald-300',
    amber: 'bg-amber-300/10 text-amber-200',
    rose: 'bg-rose-400/10 text-rose-300',
    violet: 'bg-violet-400/10 text-violet-300',
  }

  return (
    <article className="rounded-mga border border-mga-border bg-mga-surface p-4 shadow-sm shadow-black/10">
      <div className="flex items-start justify-between gap-3">
        <div className="min-w-0">
          <p className="text-xs font-medium uppercase tracking-[0.16em] text-mga-muted">{label}</p>
          <p className="mt-2 text-3xl font-semibold text-mga-text">{value}</p>
        </div>
        <span className={cn('rounded-mga p-2', toneClasses[tone])}>
          <Icon className="h-5 w-5" />
        </span>
      </div>
      <p className="mt-3 text-sm leading-6 text-mga-muted">{detail}</p>
    </article>
  )
}

function EmptyState({ title, detail }: { title: string; detail: string }) {
  return (
    <div className="rounded-mga border border-dashed border-mga-border bg-mga-surface p-8 text-center">
      <p className="text-lg font-semibold text-mga-text">{title}</p>
      <p className="mx-auto mt-2 max-w-2xl text-sm leading-6 text-mga-muted">{detail}</p>
    </div>
  )
}

function RankedBars({
  title,
  subtitle,
  items,
  labelFor = identityStatLabel,
  hrefFor,
  limit = 8,
}: {
  title: string
  subtitle: string
  items: CountStat[]
  labelFor?: (item: CountStat) => string
  hrefFor?: (item: CountStat) => string | undefined
  limit?: number
}) {
  const visible = items.slice(0, limit)
  const max = visible[0]?.count ?? 0

  return (
    <section className="rounded-mga border border-mga-border bg-mga-surface p-4 shadow-sm shadow-black/10">
      <div className="mb-4">
        <h2 className="text-lg font-semibold text-mga-text">{title}</h2>
        <p className="mt-1 text-sm text-mga-muted">{subtitle}</p>
      </div>
      {visible.length === 0 ? (
        <p className="text-sm text-mga-muted">No data yet.</p>
      ) : (
        <div className="space-y-3">
          {visible.map((item, index) => {
            const width = max > 0 ? `${Math.max((item.count / max) * 100, 7)}%` : '0%'
            const href = hrefFor?.(item)
            const content = (
              <>
                <div className="flex items-center justify-between gap-3 text-sm">
                  <span className="truncate text-mga-text">{labelFor(item)}</span>
                  <span className="shrink-0 font-mono text-mga-muted">{formatNumber(item.count)}</span>
                </div>
                <div className="h-2 overflow-hidden rounded-full bg-mga-bg">
                  <div className={cn('h-full rounded-full', barColors[index % barColors.length])} style={{ width }} />
                </div>
              </>
            )
            return (
              href ? (
                <Link
                  key={item.key}
                  to={href}
                  className="block space-y-1 rounded-mga outline-none transition-colors hover:bg-mga-elevated/50 focus-visible:ring-2 focus-visible:ring-mga-accent"
                  aria-label={`Show ${labelFor(item)} in library`}
                >
                  {content}
                </Link>
              ) : (
                <div key={item.key} className="space-y-1">
                  {content}
                </div>
              )
            )
          })}
        </div>
      )}
    </section>
  )
}

function CoveragePanel({ items }: { items: CoverageStat[] }) {
  return (
    <section className="grid gap-3 md:grid-cols-2 xl:grid-cols-4">
      {items.map((item, index) => (
        <article key={item.key} className="rounded-mga border border-mga-border bg-mga-surface p-4 shadow-sm shadow-black/10">
          <div className="flex items-center justify-between gap-3">
            <p className="text-sm font-semibold text-mga-text">{item.label}</p>
            <span className="font-mono text-sm text-mga-muted">{percentText(item.percent)}</span>
          </div>
          <div className="mt-3 h-2 overflow-hidden rounded-full bg-mga-bg">
            <div
              className={cn('h-full rounded-full', barColors[index % barColors.length])}
              style={{ width: `${Math.min(Math.max(item.percent, 0), 100)}%` }}
            />
          </div>
          <p className="mt-3 text-sm text-mga-muted">{formatNumber(item.count)} games</p>
        </article>
      ))}
    </section>
  )
}

function RecentScansPanel({ reports }: { reports: ScanReport[] }) {
  return (
    <section className="rounded-mga border border-mga-border bg-mga-surface p-4 shadow-sm shadow-black/10">
      <div className="mb-4 flex items-start justify-between gap-3">
        <div>
          <h2 className="text-lg font-semibold text-mga-text">Recent Scan Activity</h2>
          <p className="mt-1 text-sm text-mga-muted">Newest source scans and metadata refreshes.</p>
        </div>
        <ScanLine className="h-5 w-5 text-mga-accent" />
      </div>
      {reports.length === 0 ? (
        <p className="text-sm text-mga-muted">No scans have been recorded yet.</p>
      ) : (
        <div className="space-y-3">
          {reports.map((report) => (
            <article key={report.id} className="border-t border-mga-border/80 pt-3 first:border-t-0 first:pt-0">
              <div className="flex items-start justify-between gap-3">
                <div>
                  <p className="text-sm font-medium text-mga-text">
                    {report.metadata_only ? 'Metadata Refresh' : 'Source Scan'}
                  </p>
                  <p className="mt-1 text-xs text-mga-muted">{formatStartedAt(report.started_at)}</p>
                </div>
                <span className="shrink-0 font-mono text-xs text-mga-muted">{formatDuration(report.duration_ms)}</span>
              </div>
              <div className="mt-2 flex flex-wrap gap-2 text-xs">
                <span className="rounded-full bg-emerald-500/10 px-2 py-1 text-emerald-300">+{report.games_added} added</span>
                <span className="rounded-full bg-rose-500/10 px-2 py-1 text-rose-300">-{report.games_removed} removed</span>
                <span className="rounded-full bg-mga-elevated px-2 py-1 text-mga-muted">{report.total_games} total</span>
              </div>
            </article>
          ))}
        </div>
      )}
    </section>
  )
}

function LibraryStatsView({ data }: { data: LibraryStatistics }) {
  if (data.summary.canonical_game_count === 0) {
    return (
      <EmptyState
        title="No library statistics yet"
        detail="Add a source integration and run a scan to populate platform, source, metadata, media, and scan activity statistics."
      />
    )
  }

  return (
    <div className="space-y-5">
      <section className="grid gap-3 md:grid-cols-2 xl:grid-cols-4">
        <StatTile
          label="Library"
          value={formatNumber(data.summary.canonical_game_count)}
          detail={`${formatNumber(data.summary.source_game_found_count)} visible source records`}
          icon={Library}
          tone="sky"
        />
        <StatTile
          label="Sources"
          value={formatNumber(data.summary.source_game_total_count)}
          detail="Tracked source records across integrations"
          icon={Database}
          tone="emerald"
        />
        <StatTile
          label="Metadata"
          value={percentText(data.summary.percent_with_resolver_title)}
          detail={`${formatNumber(data.summary.canonical_with_resolver_title)} games have matched titles`}
          icon={BarChart3}
          tone="violet"
        />
        <StatTile
          label="Media"
          value={percentText(data.summary.percent_with_media)}
          detail={`${formatNumber(data.summary.games_with_media)} games have artwork or media`}
          icon={Clock3}
          tone="amber"
        />
      </section>

      <CoveragePanel items={data.coverage} />

      <section className="grid gap-4 xl:grid-cols-2">
        <RankedBars
          title="Platforms"
          subtitle="Visible source records by detected platform."
          items={data.platforms}
          labelFor={platformStatLabel}
          hrefFor={(item) => libraryFilterPath('platform', item.key)}
        />
        <RankedBars
          title="Source Integrations"
          subtitle="Configured sources contributing library records."
          items={data.source_integrations}
          hrefFor={(item) => libraryFilterPath('integration', item.key)}
        />
      </section>

      <section className="grid gap-4 xl:grid-cols-2">
        <RankedBars
          title="Genres"
          subtitle="Canonical games grouped by metadata genre."
          items={data.genres}
          hrefFor={(item) => libraryFilterPath('genre', item.key)}
        />
        <RankedBars
          title="Decades"
          subtitle="Release-date coverage by decade."
          items={data.decades}
          hrefFor={(item) => libraryFilterPath('decade', item.key)}
        />
      </section>

      <section className="grid gap-4 xl:grid-cols-[1fr_1fr_1.1fr]">
        <RankedBars
          title="Source Plugins"
          subtitle="Source record distribution by plugin."
          items={data.source_plugins}
          labelFor={pluginStatLabel}
          hrefFor={(item) => libraryFilterPath('source', item.key)}
          limit={6}
        />
        <RankedBars title="Metadata Providers" subtitle="Winning resolver matches by provider." items={data.metadata_providers} labelFor={pluginStatLabel} limit={6} />
        <RecentScansPanel reports={data.recent_scans} />
      </section>
    </div>
  )
}

function AchievementBucketPanel({ data }: { data: GamerStatistics }) {
  const total = Math.max(data.total_games, 1)

  return (
    <section className="rounded-mga border border-mga-border bg-mga-surface p-4 shadow-sm shadow-black/10">
      <div className="mb-4">
        <h2 className="text-lg font-semibold text-mga-text">Achievement Completion</h2>
        <p className="mt-1 text-sm text-mga-muted">Completion buckets from stored achievement sets.</p>
      </div>
      <div className="grid gap-3 md:grid-cols-5">
        {(data.achievement_completion_buckets ?? []).map((bucket, index) => (
          <article key={bucket.key} className="rounded-mga border border-mga-border/80 bg-mga-bg p-3">
            <p className="text-sm font-semibold text-mga-text">{bucket.label}</p>
            <p className="mt-2 text-2xl font-semibold">{formatNumber(bucket.game_count)}</p>
            <div className="mt-3 h-2 overflow-hidden rounded-full bg-mga-surface">
              <div
                className={cn('h-full rounded-full', barColors[index % barColors.length])}
                style={{ width: `${Math.max((bucket.game_count / total) * 100, bucket.game_count > 0 ? 7 : 0)}%` }}
              />
            </div>
          </article>
        ))}
      </div>
    </section>
  )
}

function GamerStatsView({ data, recentPlayedCount }: { data: GamerStatistics; recentPlayedCount: number }) {
  const systemStats = useMemo<CountStat[]>(
    () =>
      (data.achievement_systems ?? []).map((system) => ({
        key: system.source,
        label: sourceLabel(system.source),
        count: system.unlocked_count,
      })),
    [data.achievement_systems],
  )

  if (data.total_games === 0) {
    return (
      <EmptyState
        title="No gamer statistics yet"
        detail="Once your library has games, this page will summarize favorites, recent launches, and stored achievement progress."
      />
    )
  }

  return (
    <div className="space-y-5">
      <section className="grid gap-3 md:grid-cols-2 xl:grid-cols-5">
        <StatTile
          label="Games"
          value={formatNumber(data.total_games)}
          detail="Visible games in this profile"
          icon={Library}
          tone="sky"
        />
        <StatTile
          label="Favorites"
          value={formatNumber(data.favorite_games)}
          detail="Games marked as favorites"
          icon={Heart}
          tone="rose"
        />
        <StatTile
          label="Recent"
          value={formatNumber(recentPlayedCount)}
          detail="Recent launch shortcuts stored for this browser/profile"
          icon={Clock3}
          tone="amber"
        />
        <StatTile
          label="Achievements"
          value={percentText(data.achievement_unlock_percent)}
          detail={`${formatNumber(data.unlocked_achievements)} of ${formatNumber(data.total_achievements)} unlocked`}
          icon={Trophy}
          tone="emerald"
        />
        <StatTile
          label="Points"
          value={data.total_achievement_points ? percentText(data.achievement_point_percent) : 'Unknown'}
          detail={data.total_achievement_points ? `${formatNumber(data.earned_achievement_points)} of ${formatNumber(data.total_achievement_points)} points` : 'No stored point totals yet'}
          icon={BarChart3}
          tone="violet"
        />
      </section>

      {data.total_achievements === 0 ? (
        <EmptyState
          title="No stored achievements yet"
          detail="Run an achievement refresh to fetch stored progress for Gamer Statistics."
        />
      ) : (
        <>
          <AchievementBucketPanel data={data} />
          <section className="grid gap-4 xl:grid-cols-[1fr_1fr]">
            <RankedBars title="Unlocked By System" subtitle="Stored achievement unlocks grouped by provider." items={systemStats} />
            <section className="rounded-mga border border-mga-border bg-mga-surface p-4 shadow-sm shadow-black/10">
              <h2 className="text-lg font-semibold text-mga-text">Achievement Explorer</h2>
              <p className="mt-1 text-sm leading-6 text-mga-muted">
                Detailed per-game achievements remain in the dedicated explorer.
              </p>
              <Link
                to="/achievements"
                className="mt-4 inline-flex rounded-mga border border-mga-accent/40 bg-mga-accent/10 px-4 py-2 text-sm font-medium text-mga-accent hover:bg-mga-accent/20"
              >
                Open Achievements
              </Link>
            </section>
          </section>
        </>
      )}
    </div>
  )
}

export function StatsPage() {
  const location = useLocation()
  const mode = location.pathname.endsWith('/gamer') ? 'gamer' : 'library'
  const { recentPlayed } = useRecentPlayed()
  const libraryQuery = useQuery({
    queryKey: ['stats', 'library'],
    queryFn: getLibraryStatistics,
    enabled: mode === 'library',
  })
  const gamerQuery = useQuery({
    queryKey: ['stats', 'gamer'],
    queryFn: getGamerStatistics,
    enabled: mode === 'gamer',
  })
  const activeQuery = mode === 'library' ? libraryQuery : gamerQuery

  return (
    <div className="space-y-6">
      <div className="flex flex-wrap items-start justify-between gap-4">
        <div>
          <h1 className="text-2xl font-semibold tracking-tight text-mga-text">Stats</h1>
          <p className="mt-1 text-sm text-mga-muted">Library health and player progress from existing MGA data.</p>
        </div>
        <nav className="flex rounded-mga border border-mga-border bg-mga-surface p-1">
          {[
            { to: '/stats/library', label: 'Library Statistics' },
            { to: '/stats/gamer', label: 'Gamer Statistics' },
          ].map((item) => (
            <NavLink
              key={item.to}
              to={item.to}
              className={({ isActive }) =>
                cn(
                  'rounded-mga px-3 py-2 text-sm font-medium transition-colors',
                  isActive
                    ? 'bg-mga-elevated text-mga-accent'
                    : 'text-mga-muted hover:bg-mga-elevated hover:text-mga-text',
                )
              }
            >
              {item.label}
            </NavLink>
          ))}
        </nav>
      </div>

      {activeQuery.isPending ? (
        <section className="rounded-mga border border-mga-border bg-mga-surface p-6 text-sm text-mga-muted shadow-sm shadow-black/10">
          Loading statistics...
        </section>
      ) : null}

      {activeQuery.isError ? (
        <section className="rounded-mga border border-red-500/30 bg-red-500/10 p-6 text-sm text-red-300 shadow-sm shadow-black/10">
          Failed to load statistics: {activeQuery.error.message}
        </section>
      ) : null}

      {mode === 'library' && libraryQuery.data ? <LibraryStatsView data={libraryQuery.data} /> : null}
      {mode === 'gamer' && gamerQuery.data ? (
        <GamerStatsView data={gamerQuery.data} recentPlayedCount={recentPlayed.length} />
      ) : null}
    </div>
  )
}
