import { useState } from 'react'
import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query'
import { CheckCircle2, Download, RefreshCw, ShieldCheck } from 'lucide-react'
import { applyUpdate, checkForUpdates, downloadUpdate, getUpdateStatus, type UpdateStatus } from '@/api/client'
import { SectionCard, StatusPill } from '@/components/management/ManagementPrimitives'
import { ConfirmDialog } from '@/components/management/ManagementActions'
import { describeUpdate, formatBytes, shortDigest } from '@/lib/updateStatus'

/**
 * Updating the server, in three steps that each say what they will do.
 *
 * The server has always been able to do this — check, download and apply have
 * existed and worked the whole time — but the only way to reach them was to
 * post to the API by hand. The buttons are the missing part.
 *
 * Applying is the one irreversible step, so it states its consequences before
 * it runs rather than after.
 */
export function ServerUpdatePanel({ admin }: { admin: boolean }) {
  const queryClient = useQueryClient()
  const [confirmingApply, setConfirmingApply] = useState(false)
  const [failure, setFailure] = useState<string | null>(null)

  const status = useQuery({
    queryKey: ['management', 'update-status'],
    queryFn: getUpdateStatus,
    enabled: admin,
    // Only while bytes are moving. Polling a server that is doing nothing is
    // just noise in its logs.
    refetchInterval: (query) => (query.state.data?.download_in_progress ? 1000 : false),
  })

  const refresh = () => queryClient.invalidateQueries({ queryKey: ['management', 'update-status'] })
  const record = (error: unknown) => setFailure(error instanceof Error ? error.message : 'The server could not be reached.')

  const check = useMutation({
    mutationFn: checkForUpdates,
    onMutate: () => setFailure(null),
    onSuccess: () => void refresh(),
    onError: record,
  })
  const download = useMutation({
    mutationFn: downloadUpdate,
    onMutate: () => setFailure(null),
    onSuccess: () => void refresh(),
    onError: record,
  })
  const apply = useMutation({
    mutationFn: applyUpdate,
    onMutate: () => setFailure(null),
    onSuccess: () => void refresh(),
    onError: record,
  })

  if (!admin) {
    return (
      <SectionCard title="Updates" description="Keep this server up to date.">
        <p className="text-xs text-mga-muted">Only an administrator can update the server.</p>
      </SectionCard>
    )
  }

  const data = status.data
  const summary = describeUpdate(data)
  const busy = check.isPending || download.isPending || apply.isPending || Boolean(data?.download_in_progress)

  return (
    <SectionCard title="Updates" description="Keep this server up to date.">
      <div className="space-y-4">
        <div className="flex flex-wrap items-center justify-between gap-3">
          <div className="min-w-0">
            <p className="text-sm font-medium text-mga-text">{summary.headline}</p>
            <p className="mt-1 text-xs leading-5 text-mga-muted">{summary.detail}</p>
          </div>
          <StatusPill label={summary.pill} tone={summary.tone} />
        </div>

        {data?.download_in_progress && <DownloadProgress status={data} />}

        {failure && (
          <div className="rounded-lg border border-rose-400/25 bg-rose-500/5 p-3" role="alert">
            <p className="text-xs font-medium text-rose-200">The update could not continue</p>
            <p className="mt-1 text-xs leading-5 text-mga-muted">{failure}</p>
          </div>
        )}

        {/* The server's own message, when it has something to say that the
            status fields do not cover. */}
        {data?.message && !failure && <p className="text-xs leading-5 text-mga-muted">{data.message}</p>}

        {summary.readyToApply && data && (
          <div className="rounded-lg border border-mga-border bg-mga-elevated/40 p-3">
            <p className="flex items-center gap-1.5 text-xs font-medium text-mga-text">
              <ShieldCheck className="h-3.5 w-3.5 text-emerald-300" />
              Downloaded and verified
            </p>
            <p className="mt-1 text-xs leading-5 text-mga-muted">
              {formatBytes(data.downloaded_size)} · checksum {shortDigest(data.downloaded_sha256)}
            </p>
          </div>
        )}

        <div className="flex flex-wrap gap-2">
          <ActionButton onClick={() => check.mutate()} disabled={busy} icon={<RefreshCw className="h-3.5 w-3.5" />}>
            {check.isPending ? 'Checking…' : 'Check for updates'}
          </ActionButton>
          {summary.canDownload && (
            <ActionButton primary onClick={() => download.mutate()} disabled={busy} icon={<Download className="h-3.5 w-3.5" />}>
              {download.isPending || data?.download_in_progress ? 'Downloading…' : 'Download'}
            </ActionButton>
          )}
          {summary.readyToApply && (
            <ActionButton primary onClick={() => setConfirmingApply(true)} disabled={busy} icon={<CheckCircle2 className="h-3.5 w-3.5" />}>
              Install and restart
            </ActionButton>
          )}
          {data?.release_notes_url && (
            <a
              href={data.release_notes_url}
              target="_blank"
              rel="noreferrer noopener"
              className="inline-flex items-center rounded-md border border-mga-border px-3 py-1.5 text-xs text-mga-muted transition hover:text-mga-text"
            >
              What changed →
            </a>
          )}
        </div>
      </div>

      {/* Saying any of this afterwards is no use to someone whose scan just
          died, so it uses the console's existing consequences dialog. */}
      <ConfirmDialog
        open={confirmingApply}
        title={`Install ${data?.latest_version ?? 'the update'} and restart?`}
        confirmLabel="Install and restart"
        submitting={apply.isPending}
        error={apply.error}
        consequences={[
          'Stop this server and start it again on the new version',
          'End anything running now, including a library scan or a download in progress',
          'Disconnect any connected app until the server is back',
        ]}
        preserves={[
          'Your games, sources and profiles',
          'Your sign-ins to Steam, Xbox, Google Drive and the rest',
          'Saved games and anything already downloaded',
        ]}
        onClose={() => setConfirmingApply(false)}
        onConfirm={() => { setConfirmingApply(false); apply.mutate() }}
      />
    </SectionCard>
  )
}

function DownloadProgress({ status }: { status: UpdateStatus }) {
  const percent = Math.max(0, Math.min(100, Math.round(status.download_percent ?? 0)))
  return (
    <div>
      <div className="flex items-center justify-between text-xs text-mga-muted">
        <span>
          {formatBytes(status.download_bytes)}
          {status.download_total_bytes ? ` of ${formatBytes(status.download_total_bytes)}` : ''}
        </span>
        <span>{percent}%</span>
      </div>
      <div className="mt-1.5 h-1.5 overflow-hidden rounded-full bg-mga-elevated">
        <div className="h-full rounded-full bg-mga-accent transition-[width] duration-500" style={{ width: `${percent}%` }} />
      </div>
    </div>
  )
}

function ActionButton({
  children, onClick, disabled, icon, primary = false,
}: {
  children: React.ReactNode
  onClick: () => void
  disabled?: boolean
  icon?: React.ReactNode
  primary?: boolean
}) {
  return (
    <button
      type="button"
      onClick={onClick}
      disabled={disabled}
      className={`inline-flex items-center gap-1.5 rounded-md px-3 py-1.5 text-xs transition disabled:opacity-50 ${
        primary
          ? 'bg-mga-accent text-mga-bg hover:brightness-110'
          : 'border border-mga-border text-mga-muted hover:text-mga-text'
      }`}
    >
      {icon}
      {children}
    </button>
  )
}
