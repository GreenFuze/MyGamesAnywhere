import { useQuery } from '@tanstack/react-query'
import { CircleCheck, CircleX, PlugZap } from 'lucide-react'
import { getIntegrationStatus, listIntegrations } from '@/api/client'
import { MetricCard, PageIntro, QueryFeedback, SectionCard, StatusPill, formatCount, formatDate } from '@/components/management/ManagementPrimitives'

export function SourcesPage() {
  const integrations = useQuery({ queryKey: ['management', 'integrations'], queryFn: listIntegrations })
  const statuses = useQuery({ queryKey: ['management', 'source-status'], queryFn: getIntegrationStatus })
  const sources = integrations.data ?? []
  const statusByID = new Map((statuses.data ?? []).map((status) => [status.integration_id, status]))
  const healthy = (statuses.data ?? []).filter((status) => status.status === 'ok').length
  const error = integrations.error ?? statuses.error
  return <div className="mga-page-enter space-y-7">
    <PageIntro eyebrow="Connectors" title="Sources and provider sync" description="Manage the storefronts, subscription catalogs, cloud libraries, Google Play Games, and metadata providers that feed the MGA control plane." />
    <div className="grid gap-4 sm:grid-cols-3">
      <MetricCard label="Configured sources" value={formatCount(sources.length)} detail="Connections visible to this profile" icon={<PlugZap className="h-4 w-4" />} />
      <MetricCard label="Healthy" value={formatCount(healthy)} detail="Providers reporting an operational connection" tone={healthy === sources.length ? 'good' : 'attention'} icon={<CircleCheck className="h-4 w-4" />} />
      <MetricCard label="Needs attention" value={formatCount(Math.max(sources.length - healthy, 0))} detail="Authentication, availability, or provider errors" tone={sources.length - healthy > 0 ? 'attention' : 'good'} icon={<CircleX className="h-4 w-4" />} />
    </div>
    <SectionCard title="Connected source inventory" description="This shell reports source health; credential and refresh workflows are expanded by MGA-102.">
      <QueryFeedback pending={integrations.isPending || statuses.isPending} error={error} empty={!integrations.isPending && sources.length === 0} emptyTitle="No sources connected" emptyDescription="Add a supported provider to begin normalizing its games, offers, metadata, and availability history." />
      {sources.length > 0 && <div className="grid gap-3 lg:grid-cols-2">{sources.map((source) => {
        const status = statusByID.get(source.id)
        const tone = status?.status === 'ok' ? 'good' : status?.status === 'oauth_required' ? 'attention' : status ? 'danger' : 'neutral'
        return <article key={source.id} className="rounded-lg border border-mga-border bg-mga-elevated/40 p-4"><div className="flex items-start justify-between gap-4"><div><p className="text-sm font-semibold text-mga-text">{source.label}</p><p className="mt-1 text-xs text-mga-muted">{source.plugin_id} · {source.integration_type}</p></div><StatusPill label={status?.status.replace('_', ' ') ?? 'not checked'} tone={tone} /></div><p className="mt-4 text-xs leading-5 text-mga-muted">{status?.message || 'Provider status has not been reported yet.'}</p><p className="mt-2 text-[0.68rem] text-mga-muted">Configuration updated {formatDate(source.updated_at)}</p></article>
      })}</div>}
    </SectionCard>
  </div>
}
