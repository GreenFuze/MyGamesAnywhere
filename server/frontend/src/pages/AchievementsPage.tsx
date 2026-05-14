import { useEffect, useMemo, useState } from 'react'
import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query'
import { Link, useLocation, useSearchParams } from 'react-router-dom'
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
  type AchievementSetDTO,
  type AchievementSystemSummaryDTO,
} from '@/api/client'
import { AchievementProgressRing } from '@/components/library/AchievementProgressRing'
import { BrandBadge, BrandIcon } from '@/components/ui/brand-icon'
import { Badge } from '@/components/ui/badge'
import { CoverImage } from '@/components/ui/cover-image'
import { Tabs, type Tab } from '@/components/ui/tabs'
import { ProgressBar } from '@/components/ui/progress-bar'
import { platformLabel, selectCoverUrl, sourceLabel } from '@/lib/gameUtils'
import { useProfiles } from '@/hooks/useProfiles'
import { useSSE } from '@/hooks/useSSE'

function percent(unlocked: number, total: number): number {
  if (total <= 0) return 0
  return (unlocked / total) * 100
}

type PointsLike = {
  earned_points?: number
  total_points?: number
}

function pointsText(item: PointsLike): string | null {
  if (!item.total_points || item.total_points <= 0) return null
  return `${item.earned_points ?? 0}/${item.total_points} pts`
}

function formatPercentValue(value: number): string {
  if (!Number.isFinite(value)) return '0%'
  return `${Math.round(value)}%`
}

function SystemSummaryCard({
  system,
  onOpen,
}: {
  system: AchievementSystemSummaryDTO
  onOpen?: () => void
}) {
  const progress = percent(system.unlocked_count, system.total_count)
  const content = (
    <>
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
          <Badge variant="muted">{formatPercentValue(progress)}</Badge>
          {pointsText(system) && <Badge variant="muted">{pointsText(system)}</Badge>}
        </div>
        <ProgressBar value={progress} />
      </div>
    </>
  )

  if (onOpen) {
    return (
      <button
        type="button"
        onClick={onOpen}
        className="rounded-mga border border-mga-border bg-mga-surface p-4 text-left shadow-sm shadow-black/10 transition-colors hover:border-mga-accent/70 hover:bg-mga-elevated/40"
      >
        {content}
      </button>
    )
  }

  return <div className="rounded-mga border border-mga-border bg-mga-surface p-4 shadow-sm shadow-black/10">{content}</div>
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

type ExplorerGameEntry = {
  item: AchievementExplorerGameDTO
  systems: Array<{ system: AchievementSetDTO; achievements: AchievementDTO[] }>
  totalCount: number
  unlockedCount: number
  completion: number
}

type FailedProviderSummary = {
  pluginID: string
  count: number
  latestError: string
}

function stripProviderHtml(value: string): string {
  return value
    .replace(/<script[\s\S]*?<\/script>/gi, ' ')
    .replace(/<style[\s\S]*?<\/style>/gi, ' ')
    .replace(/<[^>]+>/g, ' ')
    .replace(/\s+/g, ' ')
    .trim()
}

function providerErrorMessage(pluginID: string, error?: string): string {
  const raw = (error ?? '').trim()
  if (!raw) return 'Refresh failed without a provider message.'
  const text = stripProviderHtml(raw)
  if (/\b429\b/i.test(raw) || /too many requests|rate[- ]limited?/i.test(raw)) {
    return `${sourceLabel(pluginID)} rate-limited the last background achievement refresh. Wait a while, then run Refresh achievements again.`
  }
  if (/AUTH_FAILED|not authenticated|re-auth/i.test(raw)) {
    return `${sourceLabel(pluginID)} needs re-authentication before achievement refresh can succeed.`
  }
  return text || 'Refresh failed without a provider message.'
}

function storedRefreshErrorMessage(error?: string): string {
  const raw = (error ?? '').trim()
  if (!raw) return ''
  const text = stripProviderHtml(raw)
  if (/\b429\b/i.test(raw) || /too many requests|rate[- ]limited?/i.test(raw)) {
    return 'An achievement provider rate-limited the last background refresh. Wait a while, then run Refresh achievements again.'
  }
  if (/AUTH_FAILED|not authenticated|re-auth/i.test(raw)) {
    return 'An achievement provider needs re-authentication before achievement refresh can succeed.'
  }
  return text
}

function summarizeFailedProviders(states: NonNullable<AchievementsDashboardResponse['refresh_states']>): FailedProviderSummary[] {
  const byProvider = new Map<string, FailedProviderSummary>()
  for (const state of states) {
    if (state.status !== 'failed') continue
    const message = providerErrorMessage(state.plugin_id, state.last_error)
    const existing = byProvider.get(state.plugin_id)
    if (!existing) {
      byProvider.set(state.plugin_id, {
        pluginID: state.plugin_id,
        count: 1,
        latestError: message,
      })
      continue
    }
    existing.count += 1
    if (!existing.latestError && message) {
      existing.latestError = message
    }
  }
  return [...byProvider.values()].sort((a, b) => {
    if (a.count !== b.count) return b.count - a.count
    return sourceLabel(a.pluginID).localeCompare(sourceLabel(b.pluginID))
  })
}

function StatTile({ label, value, detail }: { label: string; value: string | number; detail?: string }) {
  return (
    <div className="rounded-mga border border-mga-border bg-mga-surface p-4">
      <p className="text-xs font-medium uppercase tracking-wide text-mga-muted">{label}</p>
      <p className="mt-1 text-2xl font-semibold text-mga-text">{value}</p>
      {detail ? <p className="mt-1 text-sm text-mga-muted">{detail}</p> : null}
    </div>
  )
}

function refreshHealthLabel(refresh: AchievementsDashboardResponse['refresh']): { value: string; detail: string } {
  if (refresh.failed_count > 0) {
    return {
      value: `${refresh.failed_count} failed`,
      detail: `${refresh.success_count} refreshed${refresh.skipped_count > 0 ? `, ${refresh.skipped_count} skipped` : ''}`,
    }
  }
  if (refresh.success_count > 0) {
    return {
      value: 'Healthy',
      detail: `${refresh.success_count} refreshed${refresh.skipped_count > 0 ? `, ${refresh.skipped_count} skipped` : ''}`,
    }
  }
  return { value: 'Not run', detail: 'No refresh results yet' }
}

function sortExplorerEntries(a: ExplorerGameEntry, b: ExplorerGameEntry) {
  if (a.completion !== b.completion) return b.completion - a.completion
  if (a.totalCount !== b.totalCount) return b.totalCount - a.totalCount
  return a.item.game.title.localeCompare(b.item.game.title)
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
          {visibleAchievements.length > 0 ? (
            visibleAchievements.map((achievement) => (
              <AchievementExplorerRow key={`${gameId}:${system.source}:${achievement.external_id}`} achievement={achievement} />
            ))
          ) : (
            <p className="rounded-mga border border-dashed border-mga-border bg-mga-bg/50 p-3 text-sm text-mga-muted">
              This stored set does not include achievement rows.
            </p>
          )}
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
  const totalAchievementCount = systems.reduce((sum, entry) => sum + entry.system.total_count, 0)
  const achievementCountLabel =
    visibleAchievementCount !== totalAchievementCount
      ? `${visibleAchievementCount}/${totalAchievementCount} achievements`
      : `${totalAchievementCount} achievements`

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
              <Badge variant={totalAchievementCount > 0 ? 'accent' : 'muted'}>{achievementCountLabel}</Badge>
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

function AchievementExplorerSection({
  entries,
  activeProvider,
  gameQuery,
  achievementQuery,
  statusFilter,
  showEmptyGames,
  hasActiveFilters,
  detailLinkState,
  onGameQueryChange,
  onAchievementQueryChange,
  onStatusFilterChange,
  onShowEmptyGamesChange,
}: {
  entries: ExplorerGameEntry[]
  activeProvider: string
  gameQuery: string
  achievementQuery: string
  statusFilter: 'all' | 'unlocked' | 'locked'
  showEmptyGames: boolean
  hasActiveFilters: boolean
  detailLinkState: AchievementDetailLinkState
  onGameQueryChange: (value: string) => void
  onAchievementQueryChange: (value: string) => void
  onStatusFilterChange: (value: 'all' | 'unlocked' | 'locked') => void
  onShowEmptyGamesChange: (value: boolean) => void
}) {
  const providerLabel = activeProvider === 'all' ? 'all providers' : sourceLabel(activeProvider)

  return (
    <section className="space-y-4">
      <div className="flex flex-wrap items-end justify-between gap-4">
        <div>
          <h2 className="text-lg font-semibold text-mga-text">Achievement Explorer</h2>
          <p className="text-sm text-mga-muted">
            Browse stored achievements for {providerLabel}, sorted by completion percentage.
          </p>
        </div>
        <Badge variant="accent">{entries.length} games shown</Badge>
      </div>

      <div className="grid gap-3 rounded-[1.5rem] border border-mga-border bg-mga-surface p-4 md:grid-cols-2 xl:grid-cols-4">
        <label className="space-y-1">
          <span className="text-xs font-medium uppercase tracking-wide text-mga-muted">Game</span>
          <input
            type="search"
            value={gameQuery}
            onChange={(event) => onGameQueryChange(event.target.value)}
            placeholder="Filter by title"
            className="w-full rounded-mga border border-mga-border bg-mga-bg px-3 py-2 text-sm text-mga-text placeholder:text-mga-muted focus:outline-none focus:ring-2 focus:ring-mga-accent"
          />
        </label>
        <label className="space-y-1">
          <span className="text-xs font-medium uppercase tracking-wide text-mga-muted">Achievement</span>
          <input
            type="search"
            value={achievementQuery}
            onChange={(event) => onAchievementQueryChange(event.target.value)}
            placeholder="Search title or description"
            className="w-full rounded-mga border border-mga-border bg-mga-bg px-3 py-2 text-sm text-mga-text placeholder:text-mga-muted focus:outline-none focus:ring-2 focus:ring-mga-accent"
          />
        </label>
        <label className="space-y-1">
          <span className="text-xs font-medium uppercase tracking-wide text-mga-muted">Status</span>
          <select
            value={statusFilter}
            onChange={(event) => onStatusFilterChange(event.target.value as 'all' | 'unlocked' | 'locked')}
            className="w-full rounded-mga border border-mga-border bg-mga-bg px-3 py-2 text-sm text-mga-text focus:outline-none focus:ring-2 focus:ring-mga-accent"
          >
            <option value="all">All</option>
            <option value="unlocked">Unlocked only</option>
            <option value="locked">Locked only</option>
          </select>
        </label>
        <label className="flex min-h-[4.25rem] items-center gap-3 rounded-mga border border-mga-border bg-mga-bg px-3 py-2">
          <input
            type="checkbox"
            checked={showEmptyGames}
            onChange={(event) => onShowEmptyGamesChange(event.target.checked)}
            className="h-4 w-4 rounded border-mga-border bg-mga-surface text-mga-accent focus:ring-mga-accent"
          />
          <span className="text-sm text-mga-text">Show games with no unlocked achievements</span>
        </label>
      </div>

      {entries.length === 0 ? (
        <div className="rounded-mga border border-dashed border-mga-border bg-mga-surface p-8 text-center">
          <p className="text-sm text-mga-muted">
            {hasActiveFilters
              ? 'No stored achievements match the current explorer filters.'
              : 'No supported achievement games are available for this tab.'}
          </p>
        </div>
      ) : (
        <div className="space-y-4">
          {entries.map(({ item, systems }) => (
            <AchievementExplorerGameGroup
              key={`${activeProvider}:${item.game.id}`}
              item={item}
              systems={systems}
              forcedOpen={hasActiveFilters}
              detailLinkState={detailLinkState}
            />
          ))}
        </div>
      )}
    </section>
  )
}

function AllAchievementsSummary({
  data,
  refresh,
  onOpenProvider,
}: {
  data: AchievementsDashboardResponse
  refresh: AchievementsDashboardResponse['refresh']
  onOpenProvider: (source: string) => void
}) {
  const health = refreshHealthLabel(refresh)
  const unlockPercent = percent(data.totals.unlocked_count, data.totals.total_count)

  return (
    <>
      <section className="grid gap-3 md:grid-cols-4">
        <StatTile label="Providers" value={data.systems.length} detail="Stored achievement systems" />
        <StatTile label="Games" value={data.games.length} detail="Games with stored sets" />
        <StatTile
          label="Unlocked"
          value={`${data.totals.unlocked_count}/${data.totals.total_count}`}
          detail={formatPercentValue(unlockPercent)}
        />
        <StatTile label="Refresh Health" value={health.value} detail={health.detail} />
      </section>

      <section className="grid gap-3 md:grid-cols-2 xl:grid-cols-3">
        {data.systems.map((system) => (
          <SystemSummaryCard key={system.source} system={system} onOpen={() => onOpenProvider(system.source)} />
        ))}
      </section>
    </>
  )
}

function ProviderAchievementsSummary({
  system,
  refresh,
}: {
  system: AchievementSystemSummaryDTO
  refresh: AchievementsDashboardResponse['refresh']
}) {
  const health = refreshHealthLabel(refresh)
  const completion = percent(system.unlocked_count, system.total_count)

  return (
    <>
      <section className="grid gap-3 md:grid-cols-4">
        <StatTile label="Games" value={system.game_count} detail={sourceLabel(system.source)} />
        <StatTile label="Unlocked" value={`${system.unlocked_count}/${system.total_count}`} detail={formatPercentValue(completion)} />
        <StatTile label="Points" value={pointsText(system) ?? 'Unknown'} detail="Provider-specific scoring" />
        <StatTile label="Refresh Health" value={health.value} detail={health.detail} />
      </section>
      <section>
        <SystemSummaryCard system={system} />
      </section>
    </>
  )
}

export function AchievementsPage() {
  const location = useLocation()
  const [searchParams, setSearchParams] = useSearchParams()
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
  const [statusFilter, setStatusFilter] = useState<'all' | 'unlocked' | 'locked'>('all')
  const [showEmptyGames, setShowEmptyGames] = useState(false)
  const detailLinkState = buildAchievementDetailLinkState(location.pathname, location.search)
  const data = dashboard.data ?? emptyDashboard
  const explorerData = explorer.data ?? { games: [] }
  const hasStoredAchievements = data.games.length > 0
  const refresh = data.refresh ?? { total: 0, success_count: 0, failed_count: 0, skipped_count: 0 }
  const latestFailureText = storedRefreshErrorMessage(refresh.latest_failure_text)
  const failedStates = (data.refresh_states ?? []).filter((state) => state.status === 'failed')
  const failedProviderSummaries = useMemo(() => summarizeFailedProviders(data.refresh_states ?? []), [data.refresh_states])
  const refreshRunning = Boolean(activeRefreshJob && !isRefreshTerminal(activeRefreshJob))
  const refreshProgress =
    activeRefreshJob?.items_total && activeRefreshJob.items_total > 0
      ? (activeRefreshJob.items_completed / activeRefreshJob.items_total) * 100
      : 0
  const canRefresh = currentProfile?.role === 'admin_player'
  const providerSources = useMemo(() => data.systems.map((system) => system.source), [data.systems])
  const requestedTabParam = searchParams.get('tab')
  const requestedTab = requestedTabParam ?? 'all'
  const activeTab = requestedTab === 'all' || providerSources.includes(requestedTab) ? requestedTab : 'all'
  const activeProvider = activeTab === 'all' ? 'all' : activeTab
  const activeProviderSystem = activeProvider === 'all' ? undefined : data.systems.find((system) => system.source === activeProvider)
  const tabs = useMemo<Tab[]>(
    () => [
      { id: 'all', label: 'All Achievements', icon: <Trophy size={16} /> },
      ...data.systems.map((system) => ({
        id: system.source,
        label: sourceLabel(system.source),
        icon: <BrandIcon brand={system.source} />,
      })),
    ],
    [data.systems],
  )
  const normalizedGameQuery = gameQuery.trim().toLowerCase()
  const normalizedAchievementQuery = achievementQuery.trim().toLowerCase()
  const filteredExplorerGames = useMemo(() => {
    return explorerData.games
      .map((item) => {
        if (normalizedGameQuery && !item.game.title.toLowerCase().includes(normalizedGameQuery)) {
          return null
        }

        const systems = item.systems
          .filter((system) => activeProvider === 'all' || system.source === activeProvider)
          .filter((system) => system.total_count > 0)
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

        const totalCount = systems.reduce((sum, entry) => sum + entry.system.total_count, 0)
        const unlockedCount = systems.reduce((sum, entry) => sum + entry.system.unlocked_count, 0)
        if (!showEmptyGames && unlockedCount <= 0) {
          return null
        }
        return {
          item,
          systems,
          totalCount,
          unlockedCount,
          completion: percent(unlockedCount, totalCount),
        }
      })
      .filter((item): item is ExplorerGameEntry => item !== null)
      .sort(sortExplorerEntries)
  }, [activeProvider, explorerData.games, normalizedAchievementQuery, normalizedGameQuery, showEmptyGames, statusFilter])
  const hasActiveFilters =
    normalizedGameQuery.length > 0 ||
    normalizedAchievementQuery.length > 0 ||
    statusFilter !== 'all'

  const changeTab = (tabID: string) => {
    const next = new URLSearchParams(searchParams)
    next.set('tab', tabID)
    setSearchParams(next, { replace: true })
  }

  useEffect(() => {
    if (requestedTabParam === activeTab) return
    const next = new URLSearchParams(searchParams)
    next.set('tab', activeTab)
    setSearchParams(next, { replace: true })
  }, [activeTab, requestedTabParam, searchParams, setSearchParams])

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
                provider_id: data.provider_id ?? current.provider_id,
                provider_label: data.provider_label ?? current.provider_label,
                items_completed: data.items_completed ?? current.items_completed,
                items_total: data.items_total ?? current.items_total,
                current_item: data.current_item ?? current.current_item,
                waiting_until: data.waiting_until ?? current.waiting_until,
                message: data.message ?? current.message,
              }
            : current,
        )
      }),
      subscribe('achievement_refresh_waiting', (raw: unknown) => {
        const data = raw as Partial<AchievementRefreshJobStatus>
        setActiveRefreshJob((current) =>
          current && data.job_id === current.job_id
            ? {
                ...current,
                provider_id: data.provider_id ?? current.provider_id,
                provider_label: data.provider_label ?? current.provider_label,
                items_completed: data.items_completed ?? current.items_completed,
                items_total: data.items_total ?? current.items_total,
                current_item: data.current_item ?? current.current_item,
                waiting_until: data.waiting_until ?? current.waiting_until,
                message: data.message ?? current.message,
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
                  {activeRefreshJob?.provider_label || activeRefreshJob?.provider_id ? (
                    <p className="text-xs text-mga-muted">
                      Provider: {activeRefreshJob.provider_label ?? activeRefreshJob.provider_id}
                    </p>
                  ) : null}
                  {activeRefreshJob?.message ? (
                    <p className="text-xs text-mga-accent">{activeRefreshJob.message}</p>
                  ) : null}
                  {activeRefreshJob?.waiting_until ? (
                    <p className="text-xs text-mga-muted">Waiting until {formatDateTime(activeRefreshJob.waiting_until)}</p>
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
                <p>{storedRefreshErrorMessage(refreshError) || refreshError}</p>
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
              {latestFailureText ? (
                <p className="mt-2 text-sm text-mga-muted">Latest failure: {latestFailureText}</p>
              ) : null}
            </div>
          ) : null}

          {failedProviderSummaries.length > 0 ? (
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
                    {failedProviderSummaries.slice(0, 5).map((provider) => (
                      <div key={provider.pluginID} className="rounded-mga border border-amber-500/20 bg-mga-bg/50 p-3 text-sm">
                        <p className="font-medium text-amber-100">
                          {sourceLabel(provider.pluginID)} · {provider.count} failed {provider.count === 1 ? 'attempt' : 'attempts'}
                        </p>
                        <p className="mt-1 break-words text-amber-100/75">{provider.latestError}</p>
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
        <div className="space-y-6">
          <Tabs tabs={tabs} active={activeTab} onChange={changeTab} className="overflow-x-auto overflow-y-hidden" />

          {activeTab === 'all' ? (
            <AllAchievementsSummary data={data} refresh={refresh} onOpenProvider={changeTab} />
          ) : activeProviderSystem ? (
            <ProviderAchievementsSummary system={activeProviderSystem} refresh={refresh} />
          ) : null}

          <AchievementExplorerSection
            entries={filteredExplorerGames}
            activeProvider={activeProvider}
            gameQuery={gameQuery}
            achievementQuery={achievementQuery}
            statusFilter={statusFilter}
            showEmptyGames={showEmptyGames}
            hasActiveFilters={hasActiveFilters}
            detailLinkState={detailLinkState}
            onGameQueryChange={setGameQuery}
            onAchievementQueryChange={setAchievementQuery}
            onStatusFilterChange={setStatusFilter}
            onShowEmptyGamesChange={setShowEmptyGames}
          />
        </div>
      )}
    </div>
  )
}
