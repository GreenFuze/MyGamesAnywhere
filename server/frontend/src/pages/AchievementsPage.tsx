import { useEffect, useMemo, useState } from 'react'
import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query'
import { Link, useLocation } from 'react-router-dom'
import { AlertTriangle, ChevronDown, ChevronRight, Loader2, RefreshCw, Trophy } from 'lucide-react'
import {
  getAchievementRefreshJob,
  getAchievementsDashboard,
  getAchievementsExplorer,
  startAchievementRefresh,
  type AchievementDTO,
  type AchievementsDashboardResponse,
  type AchievementExplorerGameDTO,
  type AchievementRefreshJobStatus,
  type AchievementGameSummaryDTO,
  type AchievementSetDTO,
  type AchievementSystemSummaryDTO,
} from '@/api/client'
import { AchievementProgressRing } from '@/components/library/AchievementProgressRing'
import { BrandBadge } from '@/components/ui/brand-icon'
import { Badge } from '@/components/ui/badge'
import { CoverImage } from '@/components/ui/cover-image'
import { ProgressBar } from '@/components/ui/progress-bar'
import { platformLabel, selectCoverUrl, sourceLabel } from '@/lib/gameUtils'
import { useProfiles } from '@/hooks/useProfiles'
import { useSSE } from '@/hooks/useSSE'

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

type AchievementDetailLinkState = {
  from: string
  scrollY: number
  originLabel: 'Achievements'
}

function buildAchievementDetailLinkState(pathname: string, search: string): AchievementDetailLinkState {
  return {
    from: `${pathname}${search}`,
    scrollY: Math.max(0, Math.floor(window.scrollY)),
    originLabel: 'Achievements',
  }
}

function GameAchievementRow({
  item,
  detailLinkState,
}: {
  item: AchievementGameSummaryDTO
  detailLinkState: AchievementDetailLinkState
}) {
  const summary = item.game.achievement_summary
  const coverUrl = selectCoverUrl(item.game.media, item.game.cover_override)

  return (
    <Link
      to={`/game/${encodeURIComponent(item.game.id)}`}
      state={detailLinkState}
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
          <p className="text-sm text-mga-muted">{platformLabel(item.game.platform)}</p>
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

function achievementMatches(
  achievement: AchievementDTO,
  achievementQuery: string,
  statusFilter: 'all' | 'unlocked' | 'locked',
) {
  if (statusFilter === 'unlocked' && !achievement.unlocked) return false
  if (statusFilter === 'locked' && achievement.unlocked) return false
  if (!achievementQuery) return true

  const haystack = [
    achievement.title,
    achievement.description,
    achievement.external_id,
  ]
    .filter(Boolean)
    .join(' ')
    .toLowerCase()
  return haystack.includes(achievementQuery)
}

function formatUnlockedAt(value?: string): string | null {
  if (!value) return null
  const date = new Date(value)
  if (Number.isNaN(date.getTime())) return null
  return date.toLocaleString()
}

function formatDateTime(value?: string): string | null {
  if (!value) return null
  const date = new Date(value)
  if (Number.isNaN(date.getTime())) return null
  return date.toLocaleString()
}

function isRefreshTerminal(job?: Pick<AchievementRefreshJobStatus, 'status'> | null) {
  return job?.status === 'completed' || job?.status === 'failed'
}

const emptyDashboard: AchievementsDashboardResponse = {
  totals: { source_count: 0, total_count: 0, unlocked_count: 0 },
  systems: [],
  games: [],
  refresh: { total: 0, success_count: 0, failed_count: 0, skipped_count: 0 },
  refresh_states: [],
}

function AchievementExplorerRow({ achievement }: { achievement: AchievementDTO }) {
  return (
    <div className="rounded-mga border border-mga-border bg-mga-bg/60 p-3">
      <div className="flex flex-wrap items-start justify-between gap-3">
        <div className="min-w-0 space-y-1">
          <div className="flex flex-wrap items-center gap-2">
            <p className="font-medium text-mga-text">{achievement.title}</p>
            {achievement.unlocked ? <Badge variant="accent">Unlocked</Badge> : <Badge variant="muted">Locked</Badge>}
            {achievement.points ? <Badge variant="muted">{achievement.points} pts</Badge> : null}
            {achievement.rarity ? <Badge variant="muted">{achievement.rarity.toFixed(1)}%</Badge> : null}
          </div>
          {achievement.description ? (
            <p className="text-sm leading-6 text-mga-muted">{achievement.description}</p>
          ) : null}
        </div>
        {formatUnlockedAt(achievement.unlocked_at) ? (
          <span className="text-xs text-mga-muted">{formatUnlockedAt(achievement.unlocked_at)}</span>
        ) : null}
      </div>
    </div>
  )
}

function AchievementSystemGroup({
  gameId,
  system,
  visibleAchievements,
  forcedOpen,
}: {
  gameId: string
  system: AchievementSetDTO
  visibleAchievements: AchievementDTO[]
  forcedOpen: boolean
}) {
  const [open, setOpen] = useState(false)
  const expanded = forcedOpen || open

  return (
    <div className="rounded-mga border border-mga-border bg-mga-bg/50 p-4">
      <button
        type="button"
        onClick={() => setOpen((value) => !value)}
        className="flex w-full items-start justify-between gap-3 text-left"
      >
        <div className="min-w-0 space-y-2">
          <div className="flex flex-wrap items-center gap-2">
            {expanded ? <ChevronDown size={16} className="text-mga-muted" /> : <ChevronRight size={16} className="text-mga-muted" />}
            <BrandBadge brand={system.source} label={sourceLabel(system.source)} />
            <Badge variant="muted">{system.unlocked_count}/{system.total_count}</Badge>
            {pointsText(system) ? <Badge variant="muted">{pointsText(system)}</Badge> : null}
            {visibleAchievements.length !== system.achievements.length ? (
              <Badge variant="accent">Showing {visibleAchievements.length}/{system.achievements.length}</Badge>
            ) : null}
          </div>
          <p className="text-sm text-mga-muted">External game id: {system.external_game_id}</p>
        </div>
        <div className="w-40 shrink-0">
          <ProgressBar
            value={percent(system.unlocked_count, system.total_count)}
            label={`${sourceLabel(system.source)} ${system.unlocked_count}/${system.total_count}`}
          />
        </div>
      </button>

      {expanded && (
        <div className="mt-4 space-y-3">
          {visibleAchievements.map((achievement) => (
            <AchievementExplorerRow key={`${gameId}:${system.source}:${achievement.external_id}`} achievement={achievement} />
          ))}
        </div>
      )}
    </div>
  )
}

function AchievementExplorerGameGroup({
  item,
  systems,
  forcedOpen,
  detailLinkState,
}: {
  item: AchievementExplorerGameDTO
  systems: Array<{ system: AchievementSetDTO; achievements: AchievementDTO[] }>
  forcedOpen: boolean
  detailLinkState: AchievementDetailLinkState
}) {
  const [open, setOpen] = useState(false)
  const expanded = forcedOpen || open
  const coverUrl = selectCoverUrl(item.game.media, item.game.cover_override)
  const visibleAchievementCount = systems.reduce((sum, entry) => sum + entry.achievements.length, 0)

  return (
    <section className="rounded-[1.5rem] border border-mga-border bg-mga-surface p-4 shadow-sm shadow-black/10">
      <div className="flex flex-wrap items-start justify-between gap-4">
        <button type="button" onClick={() => setOpen((value) => !value)} className="flex min-w-0 flex-1 items-start gap-4 text-left">
          <div className="h-24 w-16 shrink-0 overflow-hidden rounded-mga border border-mga-border bg-mga-bg">
            <CoverImage src={coverUrl} alt={item.game.title} fit="contain" variant="compact" className="h-full w-full" />
          </div>
          <div className="min-w-0 space-y-2">
            <div className="flex flex-wrap items-center gap-2">
              {expanded ? <ChevronDown size={18} className="text-mga-muted" /> : <ChevronRight size={18} className="text-mga-muted" />}
              <h2 className="text-lg font-semibold text-mga-text">{item.game.title}</h2>
              <Badge variant="accent">{visibleAchievementCount} achievements</Badge>
            </div>
            <p className="text-sm text-mga-muted">{platformLabel(item.game.platform)}</p>
            <div className="flex flex-wrap gap-2">
              {systems.map(({ system }) => (
                <BrandBadge key={`${item.game.id}:${system.source}:${system.external_game_id}`} brand={system.source} label={sourceLabel(system.source)} />
              ))}
            </div>
          </div>
        </button>

        <div className="min-w-[11rem]">
          {item.game.achievement_summary ? (
            <>
              <div className="flex items-center justify-end gap-3">
                <AchievementProgressRing summary={item.game.achievement_summary} size={54} strokeWidth={5} />
                <div className="text-right text-sm text-mga-muted">
                  <p className="font-medium text-mga-text">
                    {item.game.achievement_summary.unlocked_count}/{item.game.achievement_summary.total_count}
                  </p>
                  {pointsText(item.game.achievement_summary) ? <p>{pointsText(item.game.achievement_summary)}</p> : null}
                </div>
              </div>
              <ProgressBar
                value={percent(item.game.achievement_summary.unlocked_count, item.game.achievement_summary.total_count)}
                label={`${item.game.achievement_summary.unlocked_count}/${item.game.achievement_summary.total_count}`}
              />
            </>
          ) : null}
        </div>
      </div>

      {expanded && (
        <div className="mt-5 space-y-4">
          {systems.map(({ system, achievements }) => (
            <AchievementSystemGroup
              key={`${item.game.id}:${system.source}:${system.external_game_id}`}
              gameId={item.game.id}
              system={system}
              visibleAchievements={achievements}
              forcedOpen={forcedOpen}
            />
          ))}
          <div className="flex justify-end">
            <Link
              to={`/game/${encodeURIComponent(item.game.id)}`}
              state={detailLinkState}
              className="text-sm font-medium text-mga-accent transition-colors hover:opacity-80"
            >
              Open game details
            </Link>
          </div>
        </div>
      )}
    </section>
  )
}

export function AchievementsPage() {
  const location = useLocation()
  const queryClient = useQueryClient()
  const { currentProfile } = useProfiles()
  const { subscribe } = useSSE()
  const [activeRefreshJob, setActiveRefreshJob] = useState<AchievementRefreshJobStatus | null>(null)
  const [refreshError, setRefreshError] = useState('')
  const dashboard = useQuery({
    queryKey: ['achievements-dashboard'],
    queryFn: getAchievementsDashboard,
  })
  const explorer = useQuery({
    queryKey: ['achievements-explorer'],
    queryFn: getAchievementsExplorer,
  })
  const activeRefreshJobQuery = useQuery({
    queryKey: ['achievement-refresh-job', activeRefreshJob?.job_id],
    queryFn: () => getAchievementRefreshJob(activeRefreshJob?.job_id ?? ''),
    enabled: Boolean(activeRefreshJob?.job_id) && !isRefreshTerminal(activeRefreshJob),
    refetchInterval: 2000,
  })
  const refreshMutation = useMutation({
    mutationFn: startAchievementRefresh,
    onSuccess: (result) => {
      setRefreshError('')
      setActiveRefreshJob(result.job)
    },
    onError: (error) => {
      setRefreshError(error instanceof Error ? error.message : 'Achievement refresh failed.')
    },
  })
  const [gameQuery, setGameQuery] = useState('')
  const [achievementQuery, setAchievementQuery] = useState('')
  const [sourceFilter, setSourceFilter] = useState('all')
  const [statusFilter, setStatusFilter] = useState<'all' | 'unlocked' | 'locked'>('all')
  const detailLinkState = buildAchievementDetailLinkState(location.pathname, location.search)
  const data = dashboard.data ?? emptyDashboard
  const explorerData = explorer.data ?? { games: [] }
  const hasStoredAchievements = data.games.length > 0
  const refresh = data.refresh ?? { total: 0, success_count: 0, failed_count: 0, skipped_count: 0 }
  const failedStates = (data.refresh_states ?? []).filter((state) => state.status === 'failed')
  const refreshRunning = Boolean(activeRefreshJob && !isRefreshTerminal(activeRefreshJob))
  const refreshProgress =
    activeRefreshJob?.items_total && activeRefreshJob.items_total > 0
      ? (activeRefreshJob.items_completed / activeRefreshJob.items_total) * 100
      : 0
  const canRefresh = currentProfile?.role === 'admin_player'
  const normalizedGameQuery = gameQuery.trim().toLowerCase()
  const normalizedAchievementQuery = achievementQuery.trim().toLowerCase()
  const filteredExplorerGames = useMemo(() => {
    return explorerData.games
      .map((item) => {
        if (normalizedGameQuery && !item.game.title.toLowerCase().includes(normalizedGameQuery)) {
          return null
        }

        const systems = item.systems
          .filter((system) => sourceFilter === 'all' || system.source === sourceFilter)
          .map((system) => ({
            system,
            achievements: system.achievements.filter((achievement) =>
              achievementMatches(achievement, normalizedAchievementQuery, statusFilter),
            ),
          }))
          .filter(({ achievements }) =>
            normalizedAchievementQuery || statusFilter !== 'all'
              ? achievements.length > 0
              : true,
          )

        if (systems.length === 0) {
          return null
        }

        return { item, systems }
      })
      .filter((item): item is { item: AchievementExplorerGameDTO; systems: Array<{ system: AchievementSetDTO; achievements: AchievementDTO[] }> } => item !== null)
  }, [explorerData.games, normalizedAchievementQuery, normalizedGameQuery, sourceFilter, statusFilter])
  const hasActiveFilters =
    normalizedGameQuery.length > 0 ||
    normalizedAchievementQuery.length > 0 ||
    sourceFilter !== 'all' ||
    statusFilter !== 'all'

  useEffect(() => {
    if (activeRefreshJobQuery.data) {
      setActiveRefreshJob(activeRefreshJobQuery.data)
      if (isRefreshTerminal(activeRefreshJobQuery.data)) {
        queryClient.invalidateQueries({ queryKey: ['achievements-dashboard'] })
        queryClient.invalidateQueries({ queryKey: ['achievements-explorer'] })
      }
    }
  }, [activeRefreshJobQuery.data, queryClient])

  useEffect(() => {
    const updateJob = (raw: unknown) => {
      const job = raw as AchievementRefreshJobStatus
      if (job?.job_id) {
        setActiveRefreshJob(job)
      }
    }
    const invalidateStoredAchievements = () => {
      queryClient.invalidateQueries({ queryKey: ['achievements-dashboard'] })
      queryClient.invalidateQueries({ queryKey: ['achievements-explorer'] })
    }
    const unsubs = [
      subscribe('achievement_refresh_started', updateJob),
      subscribe('achievement_refresh_progress', (raw: unknown) => {
        const data = raw as Partial<AchievementRefreshJobStatus>
        setActiveRefreshJob((current) =>
          current && data.job_id === current.job_id
            ? {
                ...current,
                items_completed: data.items_completed ?? current.items_completed,
                items_total: data.items_total ?? current.items_total,
                current_item: data.current_item ?? current.current_item,
              }
            : current,
        )
      }),
      subscribe('achievement_refresh_completed', (raw: unknown) => {
        updateJob(raw)
        invalidateStoredAchievements()
      }),
      subscribe('achievement_refresh_failed', (raw: unknown) => {
        updateJob(raw)
        invalidateStoredAchievements()
      }),
      subscribe('achievement_refresh_warning', invalidateStoredAchievements),
    ]
    return () => {
      for (const unsub of unsubs) unsub()
    }
  }, [queryClient, subscribe])

  if (dashboard.isPending || explorer.isPending) {
    return <p className="text-sm text-mga-muted">Loading stored achievements...</p>
  }

  if (dashboard.isError || explorer.isError) {
    return (
      <div className="rounded-mga border border-red-500/30 bg-red-500/10 p-4">
        <p className="text-sm text-red-300">{dashboard.error?.message || explorer.error?.message}</p>
      </div>
    )
  }

  return (
    <div className="space-y-6">
      <div className="flex flex-wrap items-start justify-between gap-3">
        <div>
          <h1 className="text-2xl font-semibold tracking-tight text-mga-text">Achievements</h1>
          <p className="text-sm text-mga-muted">Stored progress across achievement systems</p>
        </div>
        <div className="flex flex-wrap items-center gap-2">
          {canRefresh ? (
            <button
              type="button"
              onClick={() => refreshMutation.mutate()}
              disabled={refreshRunning || refreshMutation.isPending}
              className="inline-flex items-center gap-2 rounded-mga bg-mga-accent px-3 py-2 text-sm font-medium text-white transition-opacity hover:opacity-90 disabled:cursor-not-allowed disabled:opacity-60"
            >
              {refreshRunning || refreshMutation.isPending ? <Loader2 size={16} className="animate-spin" /> : <RefreshCw size={16} />}
              Refresh achievements
            </button>
          ) : null}
          <div className="flex items-center gap-2 rounded-mga border border-mga-border bg-mga-surface px-3 py-2 text-sm text-mga-muted">
            <Trophy size={16} className="text-mga-accent" />
            <span>{data.games.length} games</span>
          </div>
        </div>
      </div>

      {(refreshRunning || refresh.last_attempted_at || refreshError || failedStates.length > 0) && (
        <section className="space-y-3">
          {refreshRunning ? (
            <div className="rounded-mga border border-mga-accent/30 bg-mga-accent/10 p-4">
              <div className="flex flex-wrap items-start justify-between gap-3">
                <div className="space-y-1">
                  <div className="flex items-center gap-2 text-sm font-semibold text-mga-text">
                    <Loader2 size={16} className="animate-spin text-mga-accent" />
                    <span>Refreshing stored achievements</span>
                  </div>
                  <p className="text-sm text-mga-muted">
                    {activeRefreshJob?.items_total
                      ? `${activeRefreshJob.items_completed}/${activeRefreshJob.items_total} games processed`
                      : 'Preparing eligible games'}
                  </p>
                  {activeRefreshJob?.current_item ? (
                    <p className="text-xs text-mga-muted">Current: {activeRefreshJob.current_item}</p>
                  ) : null}
                </div>
                <Badge variant="accent">{activeRefreshJob?.status ?? 'running'}</Badge>
              </div>
              {activeRefreshJob?.items_total ? <div className="mt-3"><ProgressBar value={refreshProgress} /></div> : null}
            </div>
          ) : null}

          {refreshError ? (
            <div className="rounded-mga border border-red-500/30 bg-red-500/10 p-4">
              <div className="flex items-start gap-2 text-sm text-red-200">
                <AlertTriangle size={16} className="mt-0.5 shrink-0" />
                <p>{refreshError}</p>
              </div>
            </div>
          ) : null}

          {refresh.last_attempted_at ? (
            <div className="rounded-mga border border-mga-border bg-mga-surface p-4">
              <div className="flex flex-wrap items-center justify-between gap-3">
                <div>
                  <h2 className="text-sm font-semibold text-mga-text">Last refresh</h2>
                  <p className="text-sm text-mga-muted">
                    {formatDateTime(refresh.last_attempted_at)}
                    {refresh.last_successful_at ? ` · last success ${formatDateTime(refresh.last_successful_at)}` : ''}
                  </p>
                </div>
                <div className="flex flex-wrap gap-2">
                  <Badge variant="accent">{refresh.success_count} refreshed</Badge>
                  {refresh.failed_count > 0 ? <Badge variant="muted">{refresh.failed_count} failed</Badge> : null}
                  {refresh.skipped_count > 0 ? <Badge variant="muted">{refresh.skipped_count} skipped</Badge> : null}
                </div>
              </div>
              {refresh.latest_failure_text ? (
                <p className="mt-2 text-sm text-mga-muted">Latest failure: {refresh.latest_failure_text}</p>
              ) : null}
            </div>
          ) : null}

          {failedStates.length > 0 ? (
            <div className="rounded-mga border border-amber-500/30 bg-amber-500/10 p-4">
              <div className="flex items-start gap-2">
                <AlertTriangle size={16} className="mt-0.5 shrink-0 text-amber-300" />
                <div className="min-w-0 space-y-3">
                  <div>
                    <h2 className="text-sm font-semibold text-amber-100">Some achievement providers need attention</h2>
                    <p className="text-sm text-amber-100/80">
                      {failedStates.length} game/provider refresh {failedStates.length === 1 ? 'attempt has' : 'attempts have'} failed.
                    </p>
                  </div>
                  <div className="space-y-2">
                    {failedStates.slice(0, 5).map((state) => (
                      <div key={`${state.source_game_id}:${state.plugin_id}`} className="rounded-mga border border-amber-500/20 bg-mga-bg/50 p-3 text-sm">
                        <p className="font-medium text-amber-100">{sourceLabel(state.plugin_id)} · {state.external_game_id}</p>
                        <p className="mt-1 break-words text-amber-100/75">{state.last_error || 'Refresh failed without a provider message.'}</p>
                      </div>
                    ))}
                  </div>
                </div>
              </div>
            </div>
          ) : null}
        </section>
      )}

      {!hasStoredAchievements ? (
        <div className="rounded-mga border border-dashed border-mga-border bg-mga-surface p-8 text-center">
          <Trophy size={28} className="mx-auto text-mga-muted" />
          <h2 className="mt-3 text-lg font-semibold text-mga-text">No stored achievements yet</h2>
          <p className="mx-auto mt-2 max-w-xl text-sm leading-6 text-mga-muted">
            Run a refresh to fetch achievement data for eligible games. This page reads server-owned stored achievement data.
          </p>
          {canRefresh ? (
            <button
              type="button"
              onClick={() => refreshMutation.mutate()}
              disabled={refreshRunning || refreshMutation.isPending}
              className="mt-4 inline-flex items-center gap-2 rounded-mga bg-mga-accent px-3 py-2 text-sm font-medium text-white transition-opacity hover:opacity-90 disabled:cursor-not-allowed disabled:opacity-60"
            >
              {refreshRunning || refreshMutation.isPending ? <Loader2 size={16} className="animate-spin" /> : <RefreshCw size={16} />}
              Refresh achievements
            </button>
          ) : null}
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
              <p className="text-sm text-mga-muted">Stored achievement summaries by game</p>
            </div>
            <div className="space-y-3">
              {data.games.map((item) => (
                <GameAchievementRow key={item.game.id} item={item} detailLinkState={detailLinkState} />
              ))}
            </div>
          </section>

          <section className="space-y-4">
            <div className="flex flex-wrap items-end justify-between gap-4">
              <div>
                <h2 className="text-lg font-semibold text-mga-text">Achievement Explorer</h2>
                <p className="text-sm text-mga-muted">Browse stored achievements by game and source without live provider fetches.</p>
              </div>
              <Badge variant="accent">{filteredExplorerGames.length} games shown</Badge>
            </div>

            <div className="grid gap-3 rounded-[1.5rem] border border-mga-border bg-mga-surface p-4 md:grid-cols-2 xl:grid-cols-4">
              <label className="space-y-1">
                <span className="text-xs font-medium uppercase tracking-wide text-mga-muted">Game</span>
                <input
                  type="search"
                  value={gameQuery}
                  onChange={(event) => setGameQuery(event.target.value)}
                  placeholder="Filter by title"
                  className="w-full rounded-mga border border-mga-border bg-mga-bg px-3 py-2 text-sm text-mga-text placeholder:text-mga-muted focus:outline-none focus:ring-2 focus:ring-mga-accent"
                />
              </label>
              <label className="space-y-1">
                <span className="text-xs font-medium uppercase tracking-wide text-mga-muted">Achievement</span>
                <input
                  type="search"
                  value={achievementQuery}
                  onChange={(event) => setAchievementQuery(event.target.value)}
                  placeholder="Search title or description"
                  className="w-full rounded-mga border border-mga-border bg-mga-bg px-3 py-2 text-sm text-mga-text placeholder:text-mga-muted focus:outline-none focus:ring-2 focus:ring-mga-accent"
                />
              </label>
              <label className="space-y-1">
                <span className="text-xs font-medium uppercase tracking-wide text-mga-muted">Source</span>
                <select
                  value={sourceFilter}
                  onChange={(event) => setSourceFilter(event.target.value)}
                  className="w-full rounded-mga border border-mga-border bg-mga-bg px-3 py-2 text-sm text-mga-text focus:outline-none focus:ring-2 focus:ring-mga-accent"
                >
                  <option value="all">All sources</option>
                  {data.systems.map((system) => (
                    <option key={system.source} value={system.source}>
                      {sourceLabel(system.source)}
                    </option>
                  ))}
                </select>
              </label>
              <label className="space-y-1">
                <span className="text-xs font-medium uppercase tracking-wide text-mga-muted">Status</span>
                <select
                  value={statusFilter}
                  onChange={(event) => setStatusFilter(event.target.value as 'all' | 'unlocked' | 'locked')}
                  className="w-full rounded-mga border border-mga-border bg-mga-bg px-3 py-2 text-sm text-mga-text focus:outline-none focus:ring-2 focus:ring-mga-accent"
                >
                  <option value="all">All</option>
                  <option value="unlocked">Unlocked only</option>
                  <option value="locked">Locked only</option>
                </select>
              </label>
            </div>

            {filteredExplorerGames.length === 0 ? (
              <div className="rounded-mga border border-dashed border-mga-border bg-mga-surface p-8 text-center">
                <p className="text-sm text-mga-muted">
                  {hasActiveFilters
                    ? 'No stored achievements match the current explorer filters.'
                    : 'No stored achievement explorer entries are available yet.'}
                </p>
              </div>
            ) : (
              <div className="space-y-4">
                {filteredExplorerGames.map(({ item, systems }) => (
                  <AchievementExplorerGameGroup
                    key={item.game.id}
                    item={item}
                    systems={systems}
                    forcedOpen={hasActiveFilters}
                    detailLinkState={detailLinkState}
                  />
                ))}
              </div>
            )}
          </section>
        </>
      )}
    </div>
  )
}
