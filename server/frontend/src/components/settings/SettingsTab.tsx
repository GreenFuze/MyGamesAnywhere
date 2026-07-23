import { useEffect, useMemo, useState } from 'react'
import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query'
import { Download, ExternalLink, Play, RefreshCw, RotateCw } from 'lucide-react'
import {
  ApiError,
  applyUpdate,
  checkForUpdates,
  downloadUpdate,
  getUpdateStatus,
  type UpdateStatus,
} from '@/api/client'
import { Button } from '@/components/ui/button'
import { Skeleton } from '@/components/ui/skeleton'
import { useSSE } from '@/hooks/useSSE'
import { resolveUpdateActionPresentation } from '@/lib/updateActions'

function UpdateInfoCard({ label, value, detail }: { label: string; value: string; detail?: string }) {
  return (
    <article className="rounded-mga border border-mga-border bg-mga-bg p-4">
      <p className="text-xs uppercase tracking-[0.18em] text-mga-muted">{label}</p>
      <p className="mt-2 break-all text-lg font-semibold text-mga-text">{value}</p>
      {detail ? <p className="mt-1 text-xs text-mga-muted">{detail}</p> : null}
    </article>
  )
}

function UpdateInfoCardSkeleton() {
  return (
    <article className="rounded-mga border border-mga-border bg-mga-bg p-4">
      <Skeleton className="h-3 w-20" />
      <Skeleton className="mt-3 h-6 w-32" />
      <Skeleton className="mt-2 h-3 w-24" />
    </article>
  )
}

function updateErrorMessage(error: unknown) {
  if (error instanceof ApiError && error.responseText) {
    return error.responseText.trim()
  }
  if (error instanceof Error) {
    return error.message
  }
  return 'Update operation failed.'
}

function formatBytes(value?: number) {
  if (!value || value <= 0) return '0 B'
  const units = ['B', 'KB', 'MB', 'GB']
  let amount = value
  let unit = 0
  while (amount >= 1024 && unit < units.length - 1) {
    amount /= 1024
    unit += 1
  }
  return `${amount.toFixed(unit === 0 ? 0 : 1)} ${units[unit]}`
}

function normalizeVersion(value?: string) {
  return (value || '').trim().toLowerCase().replace(/^v/, '')
}

type UpdateProgressEvent = {
  latest_version?: string
  download_in_progress?: boolean
  download_bytes?: number
  download_total_bytes?: number
  download_percent?: number
  downloaded_path?: string
  downloaded_sha256?: string
  downloaded_size?: number
  apply_started?: boolean
  message?: string
  error?: string
}

const updateApplyStateKey = 'mga-update-apply-state'
const updateApplyStateMaxAgeMs = 30 * 60 * 1000

type StoredApplyState = {
  targetVersion: string
  startedAt: number
}

function readStoredApplyState(): StoredApplyState | null {
  try {
    const raw = window.localStorage.getItem(updateApplyStateKey)
    if (!raw) return null
    const parsed = JSON.parse(raw) as Partial<StoredApplyState>
    if (!parsed.targetVersion || !parsed.startedAt) return null
    if (Date.now() - parsed.startedAt > updateApplyStateMaxAgeMs) {
      window.localStorage.removeItem(updateApplyStateKey)
      return null
    }
    return { targetVersion: parsed.targetVersion, startedAt: parsed.startedAt }
  } catch {
    return null
  }
}

function writeStoredApplyState(targetVersion: string, startedAt: number) {
  try {
    window.localStorage.setItem(updateApplyStateKey, JSON.stringify({ targetVersion, startedAt }))
  } catch {
    // The in-memory state still keeps the current page useful if storage is unavailable.
  }
}

function clearStoredApplyState() {
  try {
    window.localStorage.removeItem(updateApplyStateKey)
  } catch {
    // Ignore storage failures; this state is only a UI recovery hint.
  }
}

function mergeProgress(base: UpdateStatus | undefined, progress: UpdateProgressEvent | null): UpdateStatus | undefined {
  if (!base && !progress) return undefined
  return {
    ...(base || {
      current_version: '',
      update_available: false,
      install_type: '',
    }),
    latest_version: progress?.latest_version || base?.latest_version,
    downloaded_path: progress?.downloaded_path || base?.downloaded_path,
    downloaded_sha256: progress?.downloaded_sha256 || base?.downloaded_sha256,
    downloaded_size: progress?.downloaded_size ?? base?.downloaded_size,
    download_in_progress: progress?.download_in_progress ?? base?.download_in_progress,
    download_bytes: progress?.download_bytes ?? base?.download_bytes,
    download_total_bytes: progress?.download_total_bytes ?? base?.download_total_bytes,
    download_percent: progress?.download_percent ?? base?.download_percent,
    apply_started: progress?.apply_started ?? base?.apply_started,
    message: progress?.message || base?.message,
  }
}

export function UpdateTab() {
  const queryClient = useQueryClient()
  const { subscribe } = useSSE()
  const restoredApplyState = useMemo(readStoredApplyState, [])
  const [applyStarted, setApplyStarted] = useState(!!restoredApplyState)
  const [applyTargetVersion, setApplyTargetVersion] = useState<string | null>(restoredApplyState?.targetVersion || null)
  const [applyStartedAt, setApplyStartedAt] = useState<number | null>(restoredApplyState?.startedAt || null)
  const [applyCompletedVersion, setApplyCompletedVersion] = useState<string | null>(null)
  const [applyRuntimeError, setApplyRuntimeError] = useState<string | null>(null)
  const [progressEvent, setProgressEvent] = useState<UpdateProgressEvent | null>(null)
  const updateQuery = useQuery({
    queryKey: ['update-status'],
    queryFn: getUpdateStatus,
    refetchInterval: applyStarted ? 2000 : false,
    retry: applyStarted ? true : 3,
  })

  const invalidateUpdateStatus = () => {
    void queryClient.invalidateQueries({ queryKey: ['update-status'] })
  }

  const checkMutation = useMutation({
    mutationFn: checkForUpdates,
    onSuccess: invalidateUpdateStatus,
  })
  const downloadMutation = useMutation({
    mutationFn: downloadUpdate,
    onSuccess: (result) => {
      setProgressEvent({
        latest_version: result.status.latest_version,
        download_in_progress: false,
        download_bytes: result.size,
        download_total_bytes: result.status.download_total_bytes || result.size,
        download_percent: 100,
        downloaded_path: result.path,
        downloaded_sha256: result.sha256,
        downloaded_size: result.size,
        message: result.status.message,
      })
      invalidateUpdateStatus()
    },
  })
  const applyMutation = useMutation({
    mutationFn: applyUpdate,
    onSuccess: () => {
      const targetVersion = update?.latest_version || ''
      const startedAt = Date.now()
      setApplyStarted(true)
      setApplyRuntimeError(null)
      setApplyCompletedVersion(null)
      setApplyStartedAt(startedAt)
      setApplyTargetVersion(targetVersion || null)
      if (targetVersion) {
        writeStoredApplyState(targetVersion, startedAt)
      }
      invalidateUpdateStatus()
    },
  })

  useEffect(() => {
    const eventTypes = [
      'update_download_started',
      'update_download_progress',
      'update_download_complete',
      'update_download_error',
      'update_apply_started',
      'update_apply_error',
    ]
    const unsubs = eventTypes.map((eventType) =>
      subscribe(eventType, (data) => {
        if (data && typeof data === 'object') {
          const event = data as UpdateProgressEvent
          setProgressEvent(event)
          if (event.error) {
            setApplyRuntimeError(event.error)
          }
        }
      }),
    )
    return () => {
      for (const unsub of unsubs) unsub()
    }
  }, [subscribe])

  const update = useMemo(
    () => mergeProgress(updateQuery.data, progressEvent),
    [progressEvent, updateQuery.data],
  )
  const updateApplied =
    applyStarted &&
    !!applyTargetVersion &&
    normalizeVersion(updateQuery.data?.current_version) === normalizeVersion(applyTargetVersion)
  useEffect(() => {
    if (updateApplied) {
      setApplyCompletedVersion(updateQuery.data?.current_version || applyTargetVersion)
      setApplyStarted(false)
      setApplyStartedAt(null)
      clearStoredApplyState()
    }
  }, [applyTargetVersion, updateApplied, updateQuery.data?.current_version])

  const updateBusy = checkMutation.isPending || downloadMutation.isPending || applyMutation.isPending
  const downloadInProgress = !!update?.download_in_progress || downloadMutation.isPending
  const applyWaiting = applyStarted && !updateApplied
  const updateErrors = [checkMutation.error, downloadMutation.error, applyMutation.error].filter(Boolean)
  const downloadPercent = Math.max(0, Math.min(100, update?.download_percent || 0))
  const downloadBytes = update?.download_bytes || update?.downloaded_size || 0
  const downloadTotal = update?.download_total_bytes || update?.selected_asset?.size || 0
  const downloaded = !!update?.downloaded_path
  const actionPresentation = resolveUpdateActionPresentation(downloaded)
  const actionableUpdate = !!update?.update_available && !!update?.selected_asset
  const applyWaitSeconds = applyStartedAt ? Math.floor((Date.now() - applyStartedAt) / 1000) : 0
  const applyLooksStuck =
    applyWaiting &&
    applyWaitSeconds >= 180 &&
    !!updateQuery.data &&
    !!applyTargetVersion &&
    normalizeVersion(updateQuery.data.current_version) !== normalizeVersion(applyTargetVersion)
  return (
    <div className="space-y-6">
      <section className="rounded-mga border border-mga-border bg-mga-surface p-5 shadow-sm shadow-black/10 md:p-6">
        <div className="flex flex-col gap-4 lg:flex-row lg:items-start lg:justify-between">
          <div>
            <h2 className="text-lg font-semibold text-mga-text">Updates</h2>
            <p className="mt-1 text-sm text-mga-muted">MGA checks for new versions automatically every hour.</p>
          </div>
          <div className="flex flex-wrap items-center gap-2">
            <Button
              type="button"
              variant="outline"
              onClick={() => checkMutation.mutate()}
              disabled={updateBusy || downloadInProgress || applyWaiting}
            >
              <RefreshCw size={16} />
              Check
            </Button>
            <Button
              type="button"
              variant="outline"
              onClick={() => downloadMutation.mutate()}
              disabled={updateBusy || downloadInProgress || applyWaiting || !actionableUpdate}
            >
              {downloaded ? <RotateCw size={16} /> : <Download size={16} />}
              {downloadMutation.isPending || (downloadInProgress && !applyMutation.isPending)
                ? 'Downloading...'
                : actionPresentation.secondaryLabel}
            </Button>
            <Button
              type="button"
              onClick={() => applyMutation.mutate()}
              disabled={updateBusy || applyWaiting || !actionableUpdate}
            >
              <Play size={16} />
              {applyWaiting
                ? 'Applying...'
                : applyMutation.isPending
                  ? downloadInProgress
                    ? 'Downloading...'
                    : 'Starting update...'
                  : actionPresentation.primaryLabel}
            </Button>
          </div>
        </div>

        {updateQuery.isPending ? (
          <div className="mt-5 grid gap-3 md:grid-cols-2 xl:grid-cols-4">
            {Array.from({ length: 4 }, (_, index) => (
              <UpdateInfoCardSkeleton key={`update-skeleton-${index}`} />
            ))}
          </div>
        ) : null}

        {updateQuery.isError ? (
          <p className="mt-4 text-sm text-red-300">
            Failed to load update status: {updateErrorMessage(updateQuery.error)}
          </p>
        ) : null}

        {update ? (
          <>
            <div className="mt-5 grid gap-3 md:grid-cols-2 xl:grid-cols-4">
              <UpdateInfoCard label="Current" value={update.current_version} />
              <UpdateInfoCard label="Latest" value={update.latest_version || 'Not checked'} />
              <UpdateInfoCard label="Install Type" value={update.install_type} />
              <UpdateInfoCard
                label="State"
                value={update.update_available ? 'Update available' : 'No update'}
                detail={update.message}
              />
            </div>
            {update.downloaded_path ? (
              <p className="mt-4 break-all rounded-mga border border-mga-border bg-mga-bg px-3 py-2 text-sm text-mga-muted">
                Verified download: {update.downloaded_path}
              </p>
            ) : null}
            {downloadInProgress || update.download_bytes || update.downloaded_path ? (
              <div className="mt-4 rounded-mga border border-mga-border bg-mga-bg p-3">
                <div className="flex flex-wrap items-center justify-between gap-2 text-sm">
                  <span className="font-medium text-mga-text">
                    {downloadInProgress ? 'Downloading update' : downloaded ? 'Download verified' : 'Download'}
                  </span>
                  <span className="text-mga-muted">
                    {formatBytes(downloadBytes)}{downloadTotal > 0 ? ` / ${formatBytes(downloadTotal)}` : ''}
                  </span>
                </div>
                <div className="mt-3 h-2 overflow-hidden rounded-full bg-mga-border">
                  <div
                    className="h-full rounded-full bg-mga-accent transition-[width]"
                    style={{ width: `${downloadPercent || (downloaded ? 100 : 0)}%` }}
                  />
                </div>
              </div>
            ) : null}
            {applyWaiting ? (
              <div className={`mt-4 rounded-mga border p-3 text-sm ${applyLooksStuck || applyRuntimeError ? 'border-red-400/35 bg-red-400/10 text-red-100' : 'border-sky-400/35 bg-sky-400/10 text-sky-100'}`}>
                <p className="font-medium">Update is applying. MGA may go offline while it restarts.</p>
                <p className={`mt-1 ${applyLooksStuck || applyRuntimeError ? 'text-red-100/80' : 'text-sky-100/80'}`}>
                  This page is polling until MGA comes back on {applyTargetVersion || 'the downloaded version'}.
                </p>
                {applyRuntimeError ? <p className="mt-2 break-words">{applyRuntimeError}</p> : null}
                {applyLooksStuck ? (
                  <p className="mt-2">
                    MGA is still responding on the old version after {applyWaitSeconds}s. The updater may have failed before restarting the service. Check the update/install log on the server, then try Redownload and Apply again.
                  </p>
                ) : null}
              </div>
            ) : null}
            {applyCompletedVersion ? (
              <div className="mt-4 rounded-mga border border-emerald-400/35 bg-emerald-400/10 p-3 text-sm text-emerald-100">
                MGA restarted on version {applyCompletedVersion}.
              </div>
            ) : null}
            {update.release_notes_url ? (
              <a
                href={update.release_notes_url}
                target="_blank"
                rel="noreferrer"
                className="mt-4 inline-flex items-center gap-1 text-sm font-medium text-mga-accent hover:underline"
              >
                Release notes
                <ExternalLink size={14} />
              </a>
            ) : null}
          </>
        ) : null}

        {updateErrors.map((error, index) => (
          <p key={index} className="mt-3 text-sm text-red-300">
            {updateErrorMessage(error)}
          </p>
        ))}
        {applyMutation.data ? (
          <p className="mt-3 text-sm text-mga-muted">{applyMutation.data.message}</p>
        ) : null}
      </section>
    </div>
  )
}
