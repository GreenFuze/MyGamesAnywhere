import { useQuery } from '@tanstack/react-query'
import { Link } from 'react-router-dom'
import { Trophy } from 'lucide-react'
import {
  getAchievementsDashboard,
  type AchievementGameSummaryDTO,
  type AchievementSystemSummaryDTO,
} from '@/api/client'
import { AchievementProgressRing } from '@/components/library/AchievementProgressRing'
import { BrandBadge } from '@/components/ui/brand-icon'
import { Badge } from '@/components/ui/badge'
import { CoverImage } from '@/components/ui/cover-image'
import { ProgressBar } from '@/components/ui/progress-bar'
import { selectCoverUrl, sourceLabel } from '@/lib/gameUtils'

function percent(unlocked: number, total: number): number {
  if (total <= 0) return 0
  return (unlocked / total) * 100
}

function pointsText(item: Pick<AchievementSystemSummaryDTO, 'earned_points' | 'total_points'>): string | null {
  if (!item.total_points || item.total_points <= 0) return null
  return `${item.earned_points ?? 0}/${item.total_points} pts`
}

function SystemSummaryCard({ system }: { system: AchievementSystemSummaryDTO }) {
  const progress = percent(system.unlocked_count, system.total_count)

  return (
    <div className="rounded-mga border border-mga-border bg-mga-surface p-4 shadow-sm shadow-black/10">
      <div className="flex items-start justify-between gap-3">
        <div className="min-w-0 space-y-2">
          <BrandBadge brand={system.source} label={sourceLabel(system.source)} />
          <p className="text-sm text-mga-muted">{system.game_count} games</p>
        </div>
        <AchievementProgressRing
          summary={{
            source_count: 1,
            total_count: system.total_count,
            unlocked_count: system.unlocked_count,
            total_points: system.total_points,
            earned_points: system.earned_points,
          }}
          size={48}
          strokeWidth={5}
        />
      </div>
      <div className="mt-4 space-y-2">
        <div className="flex flex-wrap items-center gap-2 text-sm text-mga-muted">
          <span>{system.unlocked_count}/{system.total_count} unlocked</span>
          {pointsText(system) && <Badge variant="muted">{pointsText(system)}</Badge>}
        </div>
        <ProgressBar value={progress} />
      </div>
    </div>
  )
}

function GameAchievementRow({ item }: { item: AchievementGameSummaryDTO }) {
  const summary = item.game.achievement_summary
  const coverUrl = selectCoverUrl(item.game.media, item.game.cover_override)

  return (
    <Link
      to={`/game/${encodeURIComponent(item.game.id)}`}
      className="grid gap-3 rounded-mga border border-mga-border bg-mga-surface p-3 transition-colors hover:border-mga-accent/70 hover:bg-mga-elevated/40 md:grid-cols-[4rem,minmax(0,1fr),minmax(16rem,0.8fr)]"
    >
      <div className="h-20 w-14 overflow-hidden rounded-mga border border-mga-border bg-mga-bg">
        <CoverImage
          src={coverUrl}
          alt={item.game.title}
          fit="contain"
          variant="compact"
          className="h-full w-full"
        />
      </div>
      <div className="min-w-0 space-y-2">
        <div>
          <h2 className="line-clamp-2 text-base font-semibold text-mga-text">{item.game.title}</h2>
          <p className="text-sm text-mga-muted">{item.game.platform}</p>
        </div>
        <div className="flex flex-wrap gap-2">
          {item.systems.map((system) => (
            <BrandBadge key={system.source} brand={system.source} label={sourceLabel(system.source)} />
          ))}
        </div>
      </div>
      <div className="space-y-3">
        {summary && (
          <div className="flex items-center gap-3">
            <AchievementProgressRing summary={summary} size={48} strokeWidth={5} />
            <div className="min-w-0 text-sm text-mga-muted">
              <p className="font-medium text-mga-text">{summary.unlocked_count}/{summary.total_count} unlocked</p>
              {pointsText({
                earned_points: summary.earned_points,
                total_points: summary.total_points,
              }) && (
                <p>{pointsText({ earned_points: summary.earned_points, total_points: summary.total_points })}</p>
              )}
            </div>
          </div>
        )}
        {item.systems.map((system) => (
          <ProgressBar
            key={system.source}
            value={percent(system.unlocked_count, system.total_count)}
            label={`${sourceLabel(system.source)} ${system.unlocked_count}/${system.total_count}`}
          />
        ))}
      </div>
    </Link>
  )
}

export function AchievementsPage() {
  const dashboard = useQuery({
    queryKey: ['achievements-dashboard'],
    queryFn: getAchievementsDashboard,
  })

  if (dashboard.isPending) {
    return <p className="text-sm text-mga-muted">Loading cached achievements...</p>
  }

  if (dashboard.isError) {
    return (
      <div className="rounded-mga border border-red-500/30 bg-red-500/10 p-4">
        <p className="text-sm text-red-300">{dashboard.error.message}</p>
      </div>
    )
  }

  const data = dashboard.data
  const hasCachedAchievements = data.games.length > 0

  return (
    <div className="space-y-6">
      <div className="flex flex-wrap items-start justify-between gap-3">
        <div>
          <h1 className="text-2xl font-semibold tracking-tight text-mga-text">Achievements</h1>
          <p className="text-sm text-mga-muted">Cached progress across achievement systems</p>
        </div>
        <div className="flex items-center gap-2 rounded-mga border border-mga-border bg-mga-surface px-3 py-2 text-sm text-mga-muted">
          <Trophy size={16} className="text-mga-accent" />
          <span>{data.games.length} games</span>
        </div>
      </div>

      {!hasCachedAchievements ? (
        <div className="rounded-mga border border-dashed border-mga-border bg-mga-surface p-8 text-center">
          <Trophy size={28} className="mx-auto text-mga-muted" />
          <h2 className="mt-3 text-lg font-semibold text-mga-text">No cached achievements yet</h2>
          <p className="mx-auto mt-2 max-w-xl text-sm leading-6 text-mga-muted">
            Open a game detail page and load its achievements to cache progress here. This page only reads stored achievement data.
          </p>
        </div>
      ) : (
        <>
          <section className="grid gap-3 md:grid-cols-4">
            <div className="rounded-mga border border-mga-border bg-mga-surface p-4">
              <p className="text-xs font-medium uppercase tracking-wide text-mga-muted">Systems</p>
              <p className="mt-1 text-2xl font-semibold">{data.systems.length}</p>
            </div>
            <div className="rounded-mga border border-mga-border bg-mga-surface p-4">
              <p className="text-xs font-medium uppercase tracking-wide text-mga-muted">Games</p>
              <p className="mt-1 text-2xl font-semibold">{data.games.length}</p>
            </div>
            <div className="rounded-mga border border-mga-border bg-mga-surface p-4">
              <p className="text-xs font-medium uppercase tracking-wide text-mga-muted">Unlocked</p>
              <p className="mt-1 text-2xl font-semibold">{data.totals.unlocked_count}/{data.totals.total_count}</p>
            </div>
            <div className="rounded-mga border border-mga-border bg-mga-surface p-4">
              <p className="text-xs font-medium uppercase tracking-wide text-mga-muted">Points</p>
              <p className="mt-1 text-2xl font-semibold">{pointsText({
                earned_points: data.totals.earned_points,
                total_points: data.totals.total_points,
              }) ?? 'Unknown'}</p>
            </div>
          </section>

          <section className="grid gap-3 md:grid-cols-2 xl:grid-cols-3">
            {data.systems.map((system) => (
              <SystemSummaryCard key={system.source} system={system} />
            ))}
          </section>

          <section className="space-y-3">
            <div>
              <h2 className="text-lg font-semibold text-mga-text">Games</h2>
              <p className="text-sm text-mga-muted">Cached achievement summaries by game</p>
            </div>
            <div className="space-y-3">
              {data.games.map((item) => (
                <GameAchievementRow key={item.game.id} item={item} />
              ))}
            </div>
          </section>
        </>
      )}
    </div>
  )
}
