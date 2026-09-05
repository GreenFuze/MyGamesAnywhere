import { useEffect, useState } from 'react'
import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query'
import {
  CircleCheck, CircleX, ExternalLink, Pencil, PlugZap, Plus, QrCode, RefreshCw, Trash2,
} from 'lucide-react'
import {
  cancelScanJob,
  deleteIntegration,
  getBackgroundScanStatus,
  getIntegrationRefreshJob,
  getIntegrationStatus,
  getScanJob,
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
  type IntegrationRefreshJobStatus,
  type PluginInfo,
} from '@/api/client'
import { Button } from '@/components/ui/button'
import { Input } from '@/components/ui/input'
import {
  ActionError, ConfirmDialog, RestrictedNotice,
} from '@/components/management/ManagementActions'
import { ConnectionFormDialog } from '@/components/management/ConnectionFormDialog'
import { QRSignIn } from '@/components/management/QRSignIn'
import { pluginQRSignInField } from '@/lib/gameUtils'
import { providerAppName, qrSignInPurpose, qrSignInReason } from '@/lib/qrSignInCopy'
import { ScanProgressPanel } from '@/components/management/ScanProgressPanel'
import { ProgressBar } from '@/components/ui/progress-bar'
import {
  MetricCard, PageIntro, QueryFeedback, SectionCard, StatusPill, formatCount, formatDate,
} from '@/components/management/ManagementPrimitives'
import { useProfiles } from '@/hooks/useProfiles'
import { DESTRUCTIVE_ACTIONS, ManagementPolicy } from '@/lib/managementPolicy'
import { cn } from '@/lib/utils'
import { SCAN_TERMINAL_STATES, describeRefreshProgress } from '@/lib/scanProgress'
import { useSSE } from '@/hooks/useSSE'

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
  // The scan panel follows this id, so a scan started anywhere on the page
  // stays visible until another one replaces it.
  const [scanJobId, setScanJobId] = useState<string | null>(null)

  const sources = integrations.data ?? []
  const statusByID = new Map((statuses.data ?? []).map((status) => [status.integration_id, status]))
  // Health is only reportable once the statuses have actually arrived. While
  // the query is in flight there are no 'ok' rows yet, so counting them says
  // every source is broken — which is alarming, wrong, and briefly visible on
  // every page load.
  const healthKnown = statuses.data !== undefined
  const healthy = (statuses.data ?? []).filter((status) => status.status === 'ok').length
  const needsAttention = Math.max(sources.length - healthy, 0)
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

  // A connection that has never been read shows an empty library, which looks
  // like a broken connection. Scan it as soon as it is configured, and let the
  // scan panel show that happening.
  const scanNewConnection = useMutation({
    mutationFn: (integrationId: string) => triggerScan([integrationId]),
    onSuccess: (result) => {
      setScanJobId(result.job.job_id)
      queryClient.invalidateQueries({ queryKey: ['management', 'scan-schedule'] })
    },
  })

  return (
    <div className="mga-page-enter space-y-7">
      <PageIntro
        eyebrow="Connectors"
        title="Sources"
        description="The stores, subscriptions, drives and folders MGA gets your games and their details from."
        actions={isAdmin ? (
          <Button onClick={() => setCreating(true)}><Plus className="h-4 w-4" /> Add source</Button>
        ) : undefined}
      />

      <div className="grid gap-4 sm:grid-cols-3">
        <MetricCard label="Configured sources" value={formatCount(sources.length)} detail="Connected to this profile" icon={<PlugZap className="h-4 w-4" />} />
        <MetricCard label="Healthy" value={healthKnown ? formatCount(healthy) : '—'} detail={healthKnown ? 'Working normally' : 'Checking…'} tone={!healthKnown ? 'neutral' : healthy === sources.length ? 'good' : 'attention'} icon={<CircleCheck className="h-4 w-4" />} />
        <MetricCard label="Needs attention" value={healthKnown ? formatCount(needsAttention) : '—'} detail={healthKnown ? 'Needs you to sign in again, or is unreachable' : 'Checking…'} tone={!healthKnown ? 'neutral' : needsAttention > 0 ? 'attention' : 'good'} icon={<CircleX className="h-4 w-4" />} />
      </div>

      {!isAdmin && (
        <RestrictedNotice>
          Adding, editing, and removing source connections requires an administrator profile.
        </RestrictedNotice>
      )}

      <ScanControls isAdmin={isAdmin} sources={sources} jobId={scanJobId} onJobStarted={setScanJobId} />

      <SectionCard title="Your sources" description="How each connection is doing, and what you can do with it.">
        <QueryFeedback
          pending={integrations.isPending || statuses.isPending}
          error={error}
          empty={!integrations.isPending && sources.length === 0}
          emptyTitle="No sources connected"
          emptyDescription="Add a store, drive or folder and MGA will start finding your games."
        />
        {sources.length > 0 && (
          <div className="grid gap-3 lg:grid-cols-2">
            {sources.map((source) => (
              <SourceCard
                key={source.id}
                source={source}
                status={statusByID.get(source.id)}
                plugin={(plugins.data ?? []).find((item) => item.plugin_id === source.plugin_id)}
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
          connections={sources}
          onClose={() => setCreating(false)}
          onSaved={async (created) => {
            setCreating(false)
            await invalidateSources()
            if (created) scanNewConnection.mutate(created.id)
          }}
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
  source, status, plugin, canManage, onEdit, onDelete, onChanged,
}: {
  source: Integration
  status?: { integration_id: string; status: string; message?: string }
  plugin?: PluginInfo
  canManage: boolean
  onEdit: () => void
  onDelete: () => void
  onChanged: () => Promise<void>
}) {
  const [notice, setNotice] = useState<string | null>(null)
  const [signingIn, setSigningIn] = useState(false)

  // A provider that hands out its own credential through its app is signed in
  // here rather than through a text box. The panel was lost with the old
  // settings shell; the endpoints behind it never stopped working.
  const qrField = pluginQRSignInField(plugin?.config as Record<string, unknown> | undefined)

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

  // Follow the refresh by job id for the same reason the scan panel does:
  // "Refresh started." followed by silence tells the operator nothing.
  const [refreshJobId, setRefreshJobId] = useState<string | null>(null)
  const refresh = useMutation({
    mutationFn: () => startIntegrationRefresh(source.id),
    onSuccess: async (result) => {
      setRefreshJobId(result.job.job_id)
      if (!result.accepted) setNotice('A refresh is already running for this connection.')
      await onChanged()
    },
  })

  const refreshJob = useQuery({
    queryKey: ['management', 'integration-refresh', refreshJobId],
    queryFn: () => getIntegrationRefreshJob(refreshJobId as string),
    enabled: Boolean(refreshJobId),
    refetchInterval: (query) => {
      const current = query.state.data
      return current && !SCAN_TERMINAL_STATES.has(current.status) ? 1000 : false
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

      {refreshJob.data && <RefreshProgress job={refreshJob.data} />}

      {qrField && signingIn && (
        <div className="mt-4">
          {qrSignInReason(source.plugin_id) && (
            <p className="mb-2 text-xs leading-5 text-mga-muted">{qrSignInReason(source.plugin_id)}</p>
          )}
          <QRSignIn
            pluginId={source.plugin_id}
            integrationId={source.id}
            providerAppName={providerAppName(source.plugin_id, source.label)}
            purposeLabel={qrSignInPurpose(source.plugin_id)}
            autoStart
            onSignedIn={() => { setNotice('Signed in. Scan this connection to pick up what it can now see.'); return onChanged() }}
          />
        </div>
      )}

      {canManage && (
        <div className="mt-4 flex flex-wrap gap-2">
          {(status?.status === 'oauth_required' || source.needs_reauth) && !qrField && (
            <Button size="sm" disabled={busy} onClick={() => authorize.mutate()}>
              <ExternalLink className="h-3.5 w-3.5" /> {authorize.isPending ? 'Starting…' : 'Authorize'}
            </Button>
          )}
          {qrField && (
            <Button
              size={status?.status === 'ok' ? undefined : 'sm'}
              variant={status?.status === 'ok' ? 'outline' : undefined}
              onClick={() => setSigningIn((current) => !current)}
            >
              <QrCode className="h-3.5 w-3.5" />
              {signingIn ? 'Hide sign-in' : qrSignInPurpose(source.plugin_id)}
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

/** Compact progress for a single connection's metadata refresh. */
function RefreshProgress({ job }: { job: IntegrationRefreshJobStatus }) {
  const view = describeRefreshProgress(job)

  return (
    <div className="mt-3 rounded-md border border-mga-border/70 bg-mga-elevated/60 p-3" role="status" aria-live="polite">
      <p className={cn('text-xs font-medium', view.failed ? 'text-rose-300' : 'text-mga-text')}>{view.headline}</p>
      {!view.finished && <ProgressBar className="mt-2" value={view.bar.value} label={view.bar.label} />}
      {view.currentItem && (
        <p className="mt-1 truncate text-xs text-mga-muted" title={view.currentItem}>Working on {view.currentItem}</p>
      )}
    </div>
  )
}

/** Library scan control: run now, cancel a running scan, and set the schedule. */
function ScanControls({
  isAdmin, sources, jobId, onJobStarted,
}: {
  isAdmin: boolean
  sources: Integration[]
  jobId: string | null
  onJobStarted: (jobId: string) => void
}) {
  const queryClient = useQueryClient()
  const { lastEvent } = useSSE()
  const schedule = useQuery({
    queryKey: ['management', 'scan-schedule'],
    queryFn: getBackgroundScanStatus,
    enabled: isAdmin,
    refetchInterval: (query) => {
      const job = query.state.data?.active_job
      return job && !SCAN_TERMINAL_STATES.has(job.status) ? 2000 : false
    },
  })

  // Follow the job by id rather than by the server's "currently active"
  // pointer. That pointer is cleared the instant a scan ends, so a fast scan
  // used to finish without the operator ever seeing that it ran.
  const job = useQuery({
    queryKey: ['management', 'scan-job', jobId],
    queryFn: () => getScanJob(jobId as string),
    enabled: isAdmin && Boolean(jobId),
    refetchInterval: (query) => {
      const current = query.state.data
      return current && !SCAN_TERMINAL_STATES.has(current.status) ? 1000 : false
    },
  })

  // The server publishes a scan event for every phase change; refetching on
  // them makes the panel track the work instead of lagging a poll behind.
  useEffect(() => {
    if (!isAdmin || !lastEvent?.type?.startsWith('scan_')) return
    queryClient.invalidateQueries({ queryKey: ['management', 'scan-schedule'] })
    if (jobId) queryClient.invalidateQueries({ queryKey: ['management', 'scan-job', jobId] })
    // A finished scan changes what the rest of the page is reporting.
    if (lastEvent.type === 'scan_complete') {
      queryClient.invalidateQueries({ queryKey: ['management', 'integrations'] })
      queryClient.invalidateQueries({ queryKey: ['management', 'source-status'] })
    }
  }, [isAdmin, jobId, lastEvent, queryClient])

  // Adopt a scan someone else started — a schedule, or another window — so it
  // is just as visible as one started here.
  const reportedActive = schedule.data?.active_job
  useEffect(() => {
    if (reportedActive && reportedActive.job_id !== jobId) onJobStarted(reportedActive.job_id)
  }, [jobId, onJobStarted, reportedActive])

  const start = useMutation({
    mutationFn: () => triggerScan(),
    onSuccess: (result) => {
      onJobStarted(result.job.job_id)
      queryClient.invalidateQueries({ queryKey: ['management', 'scan-schedule'] })
    },
  })
  const cancel = useMutation({
    mutationFn: (id: string) => cancelScanJob(id),
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
  const reported = job.data ?? reportedActive
  const active = reported && !SCAN_TERMINAL_STATES.has(reported.status) ? reported : undefined
  const interval = intervalDraft ?? String(status?.interval_minutes ?? 360)

  return (
    <SectionCard title="Library scans" description="Check every connected source for new, changed and missing games.">
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
        {/* After a reload there is no job to follow, so the page would say
            nothing at all about scanning. Report the last run instead. */}
        {!reported && sources.length > 0 && status?.last_finished_at && (
          <span className="text-xs text-mga-muted">
            Last scan {status.last_status === 'completed' ? 'completed' : status.last_status ?? 'ran'} {formatDate(status.last_finished_at)}
          </span>
        )}
      </div>

      {/* Keep the finished job on screen: a scan that vanishes the moment it
          ends never tells the operator what it found. */}
      {reported && <ScanProgressPanel job={reported} className="mt-4" />}

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
