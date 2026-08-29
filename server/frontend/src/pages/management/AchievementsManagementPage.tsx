import { useQuery } from '@tanstack/react-query'
import { Award, RefreshCw, Trophy } from 'lucide-react'
import { getAchievementsDashboard } from '@/api/client'
import { MetricCard, PageIntro, QueryFeedback, SectionCard, StatusPill, formatCount, formatDate } from '@/components/management/ManagementPrimitives'

export function AchievementsManagementPage() {
  const query = useQuery({ queryKey: ['management', 'achievements'], queryFn: getAchievementsDashboard })
  const data = query.data
  const systems = data?.systems ?? []
  return <div className="mga-page-enter space-y-7">
    <PageIntro eyebrow="Progress data" title="Achievement normalization" description="Monitor provider coverage, normalized progress, and refresh health for the selected profile without turning the management console into a player surface." />
    <div className="grid gap-4 sm:grid-cols-3"><MetricCard label="Achievements" value={formatCount(data?.totals.total_count)} detail={`${formatCount(data?.totals.unlocked_count)} unlocked`} icon={<Trophy className="h-4 w-4" />} /><MetricCard label="Games covered" value={formatCount(systems.reduce((sum, item) => sum + item.game_count, 0))} detail={`${formatCount(systems.length)} achievement systems`} icon={<Award className="h-4 w-4" />} /><MetricCard label="Refresh failures" value={formatCount(data?.refresh.failed_count)} detail={`Last success ${formatDate(data?.refresh.last_successful_at)}`} tone={(data?.refresh.failed_count ?? 0) > 0 ? 'attention' : 'good'} icon={<RefreshCw className="h-4 w-4" />} /></div>
    <SectionCard title="Achievement providers" description="Normalized coverage and last refresh evidence by provider system.">
      <QueryFeedback pending={query.isPending} error={query.error} empty={!query.isPending && systems.length === 0} emptyTitle="No achievement data yet" emptyDescription="Achievement-capable sources will appear after their first successful synchronization." />
      {systems.length > 0 && <div className="divide-y divide-mga-border/70 overflow-hidden rounded-lg border border-mga-border">{systems.map((system) => <div key={system.source} className="grid gap-3 bg-mga-elevated/35 px-4 py-3 sm:grid-cols-[1fr_auto_auto] sm:items-center"><div><p className="text-sm font-medium capitalize text-mga-text">{system.source}</p><p className="mt-1 text-xs text-mga-muted">{formatCount(system.game_count)} games</p></div><p className="text-xs text-mga-muted">{formatCount(system.unlocked_count)} / {formatCount(system.total_count)} unlocked</p><StatusPill label={system.total_count ? `${Math.round(system.unlocked_count / system.total_count * 100)}%` : 'Empty'} tone={system.total_count ? 'good' : 'neutral'} /></div>)}</div>}
    </SectionCard>
  </div>
}
