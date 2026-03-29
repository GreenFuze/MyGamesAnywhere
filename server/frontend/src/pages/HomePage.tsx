import { useQuery } from '@tanstack/react-query'
import { Link } from 'react-router-dom'
import { getScanReports, getStats, type LibraryStats, type ScanReport } from '@/api/client'

function percent(value: number) {
  return `${Math.round(value)}%`
}

function formatStartedAt(value: string) {
  const date = new Date(value)
  if (Number.isNaN(date.getTime())) return value
  return date.toLocaleString([], {
    month: 'short',
    day: 'numeric',
    hour: '2-digit',
    minute: '2-digit',
  })
}

function formatDuration(ms: number) {
  if (ms < 1000) return `${ms}ms`
  const seconds = Math.round(ms / 1000)
  if (seconds < 60) return `${seconds}s`
  const minutes = Math.floor(seconds / 60)
  const remainder = seconds % 60
  return remainder > 0 ? `${minutes}m ${remainder}s` : `${minutes}m`
}

function sortedEntries(map: Record<string, number>) {
  return Object.entries(map).sort(([, a], [, b]) => b - a)
}

function DashboardStat({ label, value, detail }: { label: string; value: string | number; detail: string }) {
  return (
    <article className="rounded-mga border border-mga-border bg-mga-surface p-4 shadow-sm shadow-black/10">
      <p className="text-xs uppercase tracking-[0.18em] text-mga-muted">{label}</p>
      <p className="mt-2 text-2xl font-semibold text-mga-text">{value}</p>
      <p className="mt-1 text-xs text-mga-muted">{detail}</p>
    </article>
  )
}

function BarChartCard({
  title,
  subtitle,
  entries,
}: {
  title: string
  subtitle: string
  entries: Array<[string, number]>
}) {
  const topValue = entries[0]?.[1] ?? 0

  return (
    <section className="rounded-mga border border-mga-border bg-mga-surface p-4 shadow-sm shadow-black/10">
      <div className="mb-4">
        <h2 className="text-lg font-semibold text-mga-text">{title}</h2>
        <p className="mt-1 text-sm text-mga-muted">{subtitle}</p>
      </div>

      {entries.length === 0 ? (
        <p className="text-sm text-mga-muted">No data yet.</p>
      ) : (
        <div className="space-y-3">
          {entries.map(([label, count]) => {
            const width = topValue > 0 ? `${Math.max((count / topValue) * 100, 8)}%` : '0%'
            return (
              <div key={label} className="space-y-1">
                <div className="flex items-center justify-between gap-3 text-sm">
                  <span className="truncate text-mga-text">{label}</span>
                  <span className="shrink-0 font-mono text-mga-muted">{count}</span>
                </div>
                <div className="h-2 overflow-hidden rounded-full bg-mga-bg">
                  <div className="h-full rounded-full bg-mga-accent/80" style={{ width }} />
                </div>
              </div>
            )
          })}
        </div>
      )}
    </section>
  )
}

function RecentScanCard({ reports }: { reports: ScanReport[] }) {
  return (
    <section className="rounded-mga border border-mga-border bg-mga-surface p-4 shadow-sm shadow-black/10">
      <div className="mb-4 flex items-center justify-between gap-3">
        <div>
          <h2 className="text-lg font-semibold text-mga-text">Recent Scan Activity</h2>
          <p className="mt-1 text-sm text-mga-muted">Latest scan and metadata refresh results.</p>
        </div>
        <Link
          to="/settings"
          className="inline-flex rounded-mga border border-mga-accent/30 bg-mga-accent/10 px-3 py-1.5 text-xs font-medium text-mga-accent hover:bg-mga-accent/20"
        >
          Open Settings
        </Link>
      </div>

      {reports.length === 0 ? (
        <p className="text-sm text-mga-muted">No scans have been recorded yet.</p>
      ) : (
        <div className="space-y-3">
          {reports.map((report) => (
            <article
              key={report.id}
              className="rounded-mga border border-mga-border/80 bg-mga-bg p-3"
            >
              <div className="flex items-center justify-between gap-3">
                <div>
                  <p className="text-sm font-medium text-mga-text">
                    {report.metadata_only ? 'Metadata Refresh' : 'Source Scan'}
                  </p>
                  <p className="mt-1 text-xs text-mga-muted">{formatStartedAt(report.started_at)}</p>
                </div>
                <span className="text-xs font-mono text-mga-muted">{formatDuration(report.duration_ms)}</span>
              </div>
              <div className="mt-3 flex flex-wrap gap-2 text-xs">
                <span className="rounded-full bg-emerald-500/10 px-2 py-1 text-emerald-300">
                  +{report.games_added} added
                </span>
                <span className="rounded-full bg-red-500/10 px-2 py-1 text-red-300">
                  -{report.games_removed} removed
                </span>
                <span className="rounded-full bg-mga-elevated px-2 py-1 text-mga-muted">
                  {report.total_games} total games
                </span>
              </div>
            </article>
          ))}
        </div>
      )}
    </section>
  )
}

function EmptyDashboard() {
  return (
    <section className="rounded-mga border border-dashed border-mga-border bg-mga-surface p-6 text-center shadow-sm shadow-black/10">
      <h2 className="text-lg font-semibold text-mga-text">Your library is ready for its first scan</h2>
      <p className="mx-auto mt-2 max-w-2xl text-sm leading-6 text-mga-muted">
        Add a source integration, run a scan, and this dashboard will start filling in platform
        coverage, genre distribution, media coverage, and recent scan history.
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

function Dashboard({ stats, reports }: { stats: LibraryStats; reports: ScanReport[] }) {
  const platformEntries = sortedEntries(stats.by_platform).slice(0, 8)
  const decadeEntries = sortedEntries(stats.by_decade)
  const genreEntries = sortedEntries(stats.top_genres).slice(0, 8)

  return (
    <div className="space-y-6">
      <div className="grid gap-3 md:grid-cols-2 xl:grid-cols-5">
        <DashboardStat
          label="Library"
          value={stats.canonical_game_count}
          detail={`${stats.source_game_found_count} source entries currently found`}
        />
        <DashboardStat
          label="Sources"
          value={stats.source_game_total_count}
          detail="Tracked records across configured source integrations"
        />
        <DashboardStat
          label="Description"
          value={percent(stats.percent_with_description)}
          detail={`${stats.games_with_description} games have unified descriptions`}
        />
        <DashboardStat
          label="Media"
          value={percent(stats.percent_with_media)}
          detail={`${stats.games_with_media} games have artwork or media`}
        />
        <DashboardStat
          label="Achievements"
          value={percent(stats.percent_with_achievements)}
          detail={`${stats.games_with_achievements} games have cached achievements`}
        />
      </div>

      <div className="grid gap-4 xl:grid-cols-2">
        <BarChartCard
          title="Games By Platform"
          subtitle="Visible canonical games grouped by primary platform."
          entries={platformEntries}
        />
        <BarChartCard
          title="Games By Decade"
          subtitle="Release-date coverage using unified canonical metadata."
          entries={decadeEntries}
        />
      </div>

      <div className="grid gap-4 xl:grid-cols-[1.2fr_1fr]">
        <BarChartCard
          title="Top Genres"
          subtitle="Most common canonical genres from resolver metadata."
          entries={genreEntries}
        />
        <RecentScanCard reports={reports} />
      </div>
    </div>
  )
}

export function HomePage() {
  const statsQuery = useQuery({
    queryKey: ['stats'],
    queryFn: getStats,
  })
  const reportsQuery = useQuery({
    queryKey: ['scan-reports', 5],
    queryFn: () => getScanReports(5),
  })

  const stats = statsQuery.data
  const reports = reportsQuery.data ?? []

  return (
    <div className="space-y-8">
      <section className="overflow-hidden rounded-mga border border-mga-border bg-mga-surface shadow-sm shadow-black/20">
        <div className="grid gap-6 p-5 md:grid-cols-[1.2fr_0.8fr] md:p-6">
          <div className="space-y-4">
            <img
              src="/title.png"
              alt="MyGamesAnywhere"
              width={1200}
              height={400}
              className="block h-auto w-full max-w-2xl object-contain object-left"
            />
            <p className="max-w-2xl text-sm leading-7 text-mga-muted">
              The shelf that follows your library. Track coverage, scan health, and library shape
              from one place instead of bouncing between raw lists and settings panels.
            </p>
            <div className="flex flex-wrap gap-3">
              <Link
                to="/library"
                className="inline-flex rounded-mga border border-mga-accent/40 bg-mga-accent/10 px-4 py-2 text-sm font-medium text-mga-accent hover:bg-mga-accent/20"
              >
                Open library
              </Link>
              <Link
                to="/settings"
                className="inline-flex rounded-mga border border-mga-border px-4 py-2 text-sm font-medium text-mga-text hover:bg-mga-hover"
              >
                Manage integrations
              </Link>
            </div>
          </div>

          <div className="rounded-mga border border-mga-border/80 bg-mga-bg p-4">
            <p className="text-xs uppercase tracking-[0.18em] text-mga-muted">Dashboard Feed</p>
            <div className="mt-4 space-y-3 text-sm">
              <div className="rounded-mga border border-mga-border/80 bg-mga-surface p-3">
                <p className="font-medium text-mga-text">One stats query</p>
                <p className="mt-1 text-mga-muted">Platform, decade, metadata, media, and achievement coverage.</p>
              </div>
              <div className="rounded-mga border border-mga-border/80 bg-mga-surface p-3">
                <p className="font-medium text-mga-text">One recent-scan query</p>
                <p className="mt-1 text-mga-muted">Latest reports without loading the full library table.</p>
              </div>
            </div>
          </div>
        </div>
      </section>

      {statsQuery.isPending || reportsQuery.isPending ? (
        <section className="rounded-mga border border-mga-border bg-mga-surface p-6 text-sm text-mga-muted shadow-sm shadow-black/10">
          Loading dashboard…
        </section>
      ) : null}

      {statsQuery.isError ? (
        <section className="rounded-mga border border-red-500/30 bg-red-500/10 p-6 text-sm text-red-300 shadow-sm shadow-black/10">
          Failed to load library stats: {statsQuery.error.message}
        </section>
      ) : null}

      {!statsQuery.isPending && !statsQuery.isError && stats ? (
        stats.canonical_game_count === 0 ? <EmptyDashboard /> : <Dashboard stats={stats} reports={reports} />
      ) : null}
    </div>
  )
}
