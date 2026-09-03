import { useState } from 'react'
import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query'
import {
  CircleCheck, CircleX, ExternalLink, Pencil, PlugZap, Plus, RefreshCw, Trash2,
} from 'lucide-react'
import {
  cancelScanJob,
  deleteIntegration,
  getBackgroundScanStatus,
  getIntegrationStatus,
  isOAuthRequired,
  listIntegrations,
  listPlugins,
  removeMissingIntegrationRecords,
  setBackgroundScanConfig,
  startIntegrationAuth,
  startIntegrationRefresh,
  triggerScan,
  validateIntegrationFiles,
  type Integration,
} from '@/api/client'
import { Button } from '@/components/ui/button'
import { Input } from '@/components/ui/input'
import {
  ActionError, ConfirmDialog, RestrictedNotice,
} from '@/components/management/ManagementActions'
import { ConnectionFormDialog } from '@/components/management/ConnectionFormDialog'
import {
  MetricCard, PageIntro, QueryFeedback, SectionCard, StatusPill, formatCount, formatDate,
} from '@/components/management/ManagementPrimitives'
import { useProfiles } from '@/hooks/useProfiles'
import { DESTRUCTIVE_ACTIONS, ManagementPolicy } from '@/lib/managementPolicy'

/** Scan-job states that mean no work is in flight. */
const SCAN_TERMINAL_STATES = new Set(['completed', 'failed', 'cancelled'])

export function SourcesPage() {
  const { currentProfile } = useProfiles()
  const policy = new ManagementPolicy(currentProfile)
  const isAdmin = policy.can('source.create')
  const queryClient = useQueryClient()

  const integrations = useQuery({ queryKey: ['management', 'integrations'], queryFn: listIntegrations })
  const statuses = useQuery({ queryKey: ['management', 'source-status'], queryFn: getIntegrationStatus })
  const plugins = useQuery({ queryKey: ['management', 'plugins'], queryFn: listPlugins, enabled: isAdmin })

  const [creating, setCreating] = useState(false)
  const [editing, setEditing] = useState<Integration | null>(null)
  const [deleting, setDeleting] = useState<Integration | null>(null)

  const sources = integrations.data ?? []
  const statusByID = new Map((statuses.data ?? []).map((status) => [status.integration_id, status]))
  const healthy = (statuses.data ?? []).filter((status) => status.status === 'ok').length
  const error = integrations.error ?? statuses.error

  const invalidateSources = async () => {
    await Promise.all([
      queryClient.invalidateQueries({ queryKey: ['management', 'integrations'] }),
      queryClient.invalidateQueries({ queryKey: ['management', 'source-status'] }),
    ])
  }

  const remove = useMutation({
    mutationFn: (id: string) => deleteIntegration(id),
    onSuccess: async () => { setDeleting(null); await invalidateSources() },
  })

  return (
    <div className="mga-page-enter space-y-7">
      <PageIntro
        eyebrow="Connectors"
        title="Sources and provider sync"
        description="Manage the storefronts, subscription catalogs, cloud libraries, and metadata providers that feed the control plane."
        actions={isAdmin ? (
          <Button onClick={() => setCreating(true)}><Plus className="h-4 w-4" /> Add source</Button>
        ) : undefined}
      />

      <div className="grid gap-4 sm:grid-cols-3">
        <MetricCard label="Configured sources" value={formatCount(sources.length)} detail="Connections visible to this profile" icon={<PlugZap className="h-4 w-4" />} />
        <MetricCard label="Healthy" value={formatCount(healthy)} detail="Providers reporting an operational connection" tone={healthy === sources.length ? 'good' : 'attention'} icon={<CircleCheck className="h-4 w-4" />} />
        <MetricCard label="Needs attention" value={formatCount(Math.max(sources.length - healthy, 0))} detail="Authentication, availability, or provider errors" tone={sources.length - healthy > 0 ? 'attention' : 'good'} icon={<CircleX className="h-4 w-4" />} />
      </div>

      {!isAdmin && (
        <RestrictedNotice>
          Adding, editing, and removing source connections requires an administrator profile.
        </RestrictedNotice>
      )}

      <ScanControls isAdmin={isAdmin} sources={sources} />

      <SectionCard title="Connected source inventory" description="Health, authorization, and maintenance for each connection.">
        <QueryFeedback
          pending={integrations.isPending || statuses.isPending}
          error={error}
          empty={!integrations.isPending && sources.length === 0}
          emptyTitle="No sources connected"
          emptyDescription="Add a supported provider to begin normalizing its games, offers, metadata, and availability history."
        />
        {sources.length > 0 && (
          <div className="grid gap-3 lg:grid-cols-2">
            {sources.map((source) => (
              <SourceCard
                key={source.id}
                source={source}
                status={statusByID.get(source.id)}
                canManage={isAdmin}
                onEdit={() => setEditing(source)}
                onDelete={() => { remove.reset(); setDeleting(source) }}
                onChanged={invalidateSources}
              />
            ))}
          </div>
        )}
      </SectionCard>

      {creating && (
        <ConnectionFormDialog
          plugins={plugins.data ?? []}
          onClose={() => setCreating(false)}
          onSaved={async () => { setCreating(false); await invalidateSources() }}
        />
      )}

      {editing && (
        <ConnectionFormDialog
          plugins={plugins.data ?? []}
          existing={editing}
          onClose={() => setEditing(null)}
          onSaved={async () => { setEditing(null); await invalidateSources() }}
        />
      )}

      <ConfirmDialog
        open={deleting !== null}
        title={`Remove ${deleting?.label ?? 'connection'}?`}
        confirmLabel="Remove connection"
        submitting={remove.isPending}
        error={remove.error}
        onClose={() => setDeleting(null)}
        onConfirm={() => deleting && remove.mutate(deleting.id)}
        consequences={DESTRUCTIVE_ACTIONS['source.delete'].consequences}
        preserves={DESTRUCTIVE_ACTIONS['source.delete'].preserves}
      />
    </div>
  )
}

/** One connection with its authorization and maintenance actions. */
function SourceCard({
  source, status, canManage, onEdit, onDelete, onChanged,
}: {
  source: Integration
  status?: { integration_id: string; status: string; message?: string }
  canManage: boolean
  onEdit: () => void
  onDelete: () => void
  onChanged: () => Promise<void>
}) {
  const [notice, setNotice] = useState<string | null>(null)

  const authorize = useMutation({
    mutationFn: () => startIntegrationAuth(source.id),
    onSuccess: async (result) => {
      // A provider that needs consent returns a URL rather than completing here.
      if (isOAuthRequired(result) && result.authorize_url) {
        window.open(result.authorize_url, '_blank', 'noopener,noreferrer')
        setNotice('Finish the provider sign-in in the new tab, then refresh this connection.')
        return
      }
      setNotice('Connection re-authorized.')
      await onChanged()
    },
  })

  const refresh = useMutation({
    mutationFn: () => startIntegrationRefresh(source.id),
    onSuccess: async (result) => {
      setNotice(result.accepted ? 'Refresh started.' : 'A refresh is already running for this connection.')
      await onChanged()
    },
  })

  const validate = useMutation({
    mutationFn: () => validateIntegrationFiles(source.id),
    onSuccess: (report) => {
      const missing = report.missing?.length ?? 0
      setNotice(missing === 0
        ? 'All source files were found.'
        : `${missing} record${missing === 1 ? '' : 's'} reference files that are no longer present.`)
    },
  })

  const removeMissing = useMutation({
    mutationFn: (ids: string[]) => removeMissingIntegrationRecords(source.id, ids),
    onSuccess: async () => { setNotice('Missing records removed. No files were deleted.'); await onChanged() },
  })

  const missingIDs = validate.data?.missing?.map((item) => item.id) ?? []
  const tone = status?.status === 'ok' ? 'good' : status?.status === 'oauth_required' ? 'attention' : status ? 'danger' : 'neutral'
  const busy = authorize.isPending || refresh.isPending || validate.isPending || removeMissing.isPending

  return (
    <article className="rounded-lg border border-mga-border bg-mga-elevated/40 p-4">
      <div className="flex items-start justify-between gap-4">
        <div className="min-w-0">
          <p className="truncate text-sm font-semibold text-mga-text">{source.label}</p>
          <p className="mt-1 text-xs text-mga-muted">{source.plugin_id} · {source.integration_type}</p>
        </div>
        <StatusPill label={status?.status.replace('_', ' ') ?? 'not checked'} tone={tone} />
      </div>
      <p className="mt-3 text-xs leading-5 text-mga-muted">
        {status?.message || 'Provider status has not been reported yet.'}
      </p>
      <p className="mt-1 text-[0.68rem] text-mga-muted">Configuration updated {formatDate(source.updated_at)}</p>

      {canManage && (
        <div className="mt-4 flex flex-wrap gap-2">
          {(status?.status === 'oauth_required' || source.needs_reauth) && (
            <Button size="sm" disabled={busy} onClick={() => authorize.mutate()}>
              <ExternalLink className="h-3.5 w-3.5" /> {authorize.isPending ? 'Starting…' : 'Authorize'}
            </Button>
          )}
          <Button variant="outline" size="sm" disabled={busy} onClick={() => refresh.mutate()}>
            <RefreshCw className="h-3.5 w-3.5" /> {refresh.isPending ? 'Refreshing…' : 'Refresh'}
          </Button>
          <Button variant="ghost" size="sm" disabled={busy} onClick={() => validate.mutate()}>
            {validate.isPending ? 'Checking…' : 'Validate files'}
          </Button>
          <Button variant="ghost" size="sm" onClick={onEdit}><Pencil className="h-3.5 w-3.5" /> Edit</Button>
          <Button variant="ghost" size="sm" className="text-rose-300 hover:bg-rose-500/10" onClick={onDelete}>
            <Trash2 className="h-3.5 w-3.5" /> Remove
          </Button>
        </div>
      )}

      {missingIDs.length > 0 && (
        <div className="mt-3 rounded-md border border-amber-400/25 bg-amber-400/5 p-3">
          <p className="text-xs leading-5 text-mga-text">
            {missingIDs.length} record{missingIDs.length === 1 ? '' : 's'} point at files that are gone.
            Removing them clears the library entries only; no file on disk or at the provider is deleted.
          </p>
          <Button
            variant="outline"
            size="sm"
            className="mt-2"
            disabled={removeMissing.isPending}
            onClick={() => removeMissing.mutate(missingIDs)}
          >
            {removeMissing.isPending ? 'Removing…' : 'Remove missing records'}
          </Button>
        </div>
      )}

      {notice && <p className="mt-3 text-xs leading-5 text-emerald-300">{notice}</p>}
      <ActionError error={authorize.error ?? refresh.error ?? validate.error ?? removeMissing.error} className="mt-3" />
    </article>
  )
}

/** Library scan control: run now, cancel a running scan, and set the schedule. */
function ScanControls({ isAdmin, sources }: { isAdmin: boolean; sources: Integration[] }) {
  const queryClient = useQueryClient()
  const schedule = useQuery({
    queryKey: ['management', 'scan-schedule'],
    queryFn: getBackgroundScanStatus,
    enabled: isAdmin,
    refetchInterval: (query) => {
      const job = query.state.data?.active_job
      return job && !SCAN_TERMINAL_STATES.has(job.status) ? 4000 : false
    },
  })

  const start = useMutation({
    mutationFn: () => triggerScan(),
    onSuccess: () => queryClient.invalidateQueries({ queryKey: ['management', 'scan-schedule'] }),
  })
  const cancel = useMutation({
    mutationFn: (jobId: string) => cancelScanJob(jobId),
    onSuccess: () => queryClient.invalidateQueries({ queryKey: ['management', 'scan-schedule'] }),
  })
  const configure = useMutation({
    mutationFn: (config: { enabled: boolean; interval_minutes: number }) => setBackgroundScanConfig(config),
    onSuccess: () => queryClient.invalidateQueries({ queryKey: ['management', 'scan-schedule'] }),
  })

  // Every hook must run before the administrator gate, or the hook order
  // changes when the selected profile does.
  const [intervalDraft, setIntervalDraft] = useState<string | null>(null)

  if (!isAdmin) return null

  const status = schedule.data
  // The server keeps reporting the last job briefly after it ends. Only a job
  // that is still working may be cancelled or shown as progress.
  const reported = status?.active_job
  const active = reported && !SCAN_TERMINAL_STATES.has(reported.status) ? reported : undefined
  const interval = intervalDraft ?? String(status?.interval_minutes ?? 360)

  return (
    <SectionCard title="Library scans" description="Reconcile every connected source into canonical games and copies.">
      <div className="flex flex-wrap items-center gap-2">
        <Button disabled={start.isPending || Boolean(active) || sources.length === 0} onClick={() => start.mutate()}>
          <RefreshCw className="h-4 w-4" /> {start.isPending ? 'Starting…' : 'Scan now'}
        </Button>
        {active && (
          <Button variant="outline" disabled={cancel.isPending} onClick={() => cancel.mutate(active.job_id)}>
            {cancel.isPending ? 'Cancelling…' : 'Cancel scan'}
          </Button>
        )}
        {sources.length === 0 && <span className="text-xs text-mga-muted">Connect a source before scanning.</span>}
      </div>

      {active && (
        <div className="mt-4 rounded-lg border border-mga-border bg-mga-elevated/40 p-4">
          <p className="text-sm font-medium text-mga-text">
            Scanning {active.current_integration_label || 'sources'} · {active.integrations_completed}/{active.integration_count}
          </p>
          {active.current_phase && <p className="mt-1 text-xs text-mga-muted">{active.current_phase}</p>}
        </div>
      )}

      <div className="mt-5 flex flex-wrap items-end gap-3 border-t border-mga-border/70 pt-4">
        <Input
          label="Automatic scan interval (minutes)"
          type="number"
          min={5}
          className="w-56"
          value={interval}
          onChange={(event) => setIntervalDraft(event.target.value)}
        />
        <Button
          variant="outline"
          disabled={configure.isPending}
          onClick={() => configure.mutate({ enabled: true, interval_minutes: Number(interval) || 360 })}
        >
          {configure.isPending ? 'Saving…' : 'Enable schedule'}
        </Button>
        <Button
          variant="ghost"
          disabled={configure.isPending || status?.enabled === false}
          onClick={() => configure.mutate({ enabled: false, interval_minutes: Number(interval) || 360 })}
        >
          Disable
        </Button>
        <p className="text-xs text-mga-muted">
          {status?.enabled ? `Next run ${formatDate(status.next_run_at)}` : 'Automatic scanning is off.'}
        </p>
      </div>
      <ActionError error={start.error ?? cancel.error ?? configure.error} className="mt-3" />
    </SectionCard>
  )
}
