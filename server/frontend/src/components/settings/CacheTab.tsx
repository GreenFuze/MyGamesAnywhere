import { useState } from 'react'
import { useQuery, useQueryClient } from '@tanstack/react-query'
import { Button } from '@/components/ui/button'
import {
  clearCacheEntries,
  clearMediaCache,
  deleteCacheEntry,
  getMediaQueueStatus,
  listCacheEntries,
  listCacheJobs,
  retryFailedMediaDownloads,
} from '@/api/client'

function formatBytes(value: number): string {
  if (value <= 0) return '0 B'
  const units = ['B', 'KB', 'MB', 'GB', 'TB']
  let size = value
  let unitIndex = 0
  while (size >= 1024 && unitIndex < units.length - 1) {
    size /= 1024
    unitIndex += 1
  }
  return `${size.toFixed(size >= 10 || unitIndex === 0 ? 0 : 1)} ${units[unitIndex]}`
}

function formatTime(value?: string) {
  if (!value) return 'Never'
  return new Date(value).toLocaleString()
}

export function CacheTab() {
  const queryClient = useQueryClient()
  const [busyEntryId, setBusyEntryId] = useState<string | null>(null)
  const [busyClear, setBusyClear] = useState(false)
  const [busyMediaRetry, setBusyMediaRetry] = useState(false)
  const [busyMediaClear, setBusyMediaClear] = useState(false)
  const [sourceActionError, setSourceActionError] = useState('')
  const [mediaActionError, setMediaActionError] = useState('')

  const entries = useQuery({
    queryKey: ['cache-entries'],
    queryFn: listCacheEntries,
  })

  const jobs = useQuery({
    queryKey: ['cache-jobs'],
    queryFn: () => listCacheJobs(20),
    refetchInterval: 2000,
  })

  const mediaStatus = useQuery({
    queryKey: ['media-queue-status'],
    queryFn: getMediaQueueStatus,
    refetchInterval: (query) => {
      const status = query.state.data
      if (!status) return 3000
      return status.items_left > 0 || status.downloading > 0 || status.queued > 0 ? 3000 : false
    },
  })

  const totals = (entries.data ?? []).reduce(
    (acc, entry) => {
      acc.entries += 1
      acc.files += entry.file_count
      acc.bytes += entry.size
      return acc
    },
    { entries: 0, files: 0, bytes: 0 },
  )

  const refreshAll = async () => {
    await Promise.all([
      queryClient.invalidateQueries({ queryKey: ['cache-entries'] }),
      queryClient.invalidateQueries({ queryKey: ['cache-jobs'] }),
      queryClient.invalidateQueries({ queryKey: ['media-queue-status'] }),
    ])
  }

  const handleDeleteEntry = async (entryId: string) => {
    setBusyEntryId(entryId)
    setSourceActionError('')
    try {
      await deleteCacheEntry(entryId)
      await refreshAll()
    } catch (error) {
      setSourceActionError(error instanceof Error ? error.message : 'Failed to delete cache entry.')
    } finally {
      setBusyEntryId(null)
    }
  }

  const handleClear = async () => {
    const confirmed = window.confirm('Clear all cached source entries?')
    if (!confirmed) return
    setBusyClear(true)
    setSourceActionError('')
    try {
      await clearCacheEntries()
      await refreshAll()
    } catch (error) {
      setSourceActionError(error instanceof Error ? error.message : 'Failed to clear cache entries.')
    } finally {
      setBusyClear(false)
    }
  }

  const refreshMedia = async () => {
    await queryClient.invalidateQueries({ queryKey: ['media-queue-status'] })
  }

  const handleRetryMedia = async () => {
    setBusyMediaRetry(true)
    setMediaActionError('')
    try {
      await retryFailedMediaDownloads()
      await refreshMedia()
    } catch (error) {
      setMediaActionError(error instanceof Error ? error.message : 'Failed to retry media downloads.')
    } finally {
      setBusyMediaRetry(false)
    }
  }

  const handleClearMedia = async () => {
    const confirmed = window.confirm(
      'Clear local downloaded artwork/video files? Games and metadata stay intact. MGA will download media again as needed.',
    )
    if (!confirmed) return
    setBusyMediaClear(true)
    setMediaActionError('')
    try {
      await clearMediaCache()
      await refreshMedia()
    } catch (error) {
      setMediaActionError(error instanceof Error ? error.message : 'Failed to clear media cache.')
    } finally {
      setBusyMediaClear(false)
    }
  }

  const media = mediaStatus.data
  const failedMediaCount = (media?.retry_waiting ?? 0) + (media?.failed_permanent ?? 0)
  const currentMediaDownloads = media?.current ?? []
  const recentMediaErrors = media?.recent_errors ?? []
  const mediaLine = media
    ? media.items_left > 0
      ? `Downloading ${media.items_left} item${media.items_left === 1 ? '' : 's'}`
      : 'Artwork is up to date'
    : 'Loading artwork…'

  return (
    <div className="space-y-6">
      <section className="rounded-mga border border-mga-border bg-mga-surface p-5">
        <div className="flex flex-wrap items-start justify-between gap-4">
          <div>
            <h2 className="text-lg font-semibold text-mga-text">Artwork and videos</h2>
          </div>
          <div className="flex flex-wrap gap-2">
            <Button variant="outline" size="sm" onClick={() => void refreshMedia()} disabled={mediaStatus.isFetching}>
              Refresh
            </Button>
            <Button
              variant="outline"
              size="sm"
              onClick={() => void handleRetryMedia()}
              disabled={busyMediaRetry || !media || media.retry_waiting === 0}
            >
              {busyMediaRetry ? 'Retrying…' : 'Retry failed'}
            </Button>
            <Button variant="outline" size="sm" onClick={() => void handleClearMedia()} disabled={busyMediaClear || !media || media.total === 0}>
              {busyMediaClear ? 'Clearing…' : 'Clear'}
            </Button>
          </div>
        </div>

        <div className="mt-4 rounded-mga border border-mga-border bg-mga-bg px-4 py-3">
          <p className="text-sm font-medium text-mga-text">{mediaLine}</p>
          {media?.last_error && recentMediaErrors.length === 0 && (
            <p className="mt-2 text-xs text-red-400">Last error: {media.last_error}</p>
          )}
          {media?.last_activity_at && <p className="mt-1 text-xs text-mga-muted">Last activity: {formatTime(media.last_activity_at)}</p>}
        </div>

        {currentMediaDownloads.length > 0 && (
          <div className="mt-4 rounded-mga border border-blue-500/30 bg-blue-500/10 px-4 py-3">
            <p className="text-sm font-semibold text-blue-200">Downloading now</p>
            <div className="mt-3 space-y-2">
              {currentMediaDownloads.map((item) => (
                <div key={item.asset_id} className="rounded-mga border border-blue-500/20 bg-mga-bg/70 px-3 py-2">
                  <p className="text-xs text-mga-muted">Asset {item.asset_id}</p>
                  <p className="mt-1 break-all font-mono text-xs text-blue-100">{item.url}</p>
                </div>
              ))}
            </div>
          </div>
        )}

        {recentMediaErrors.length > 0 && (
          <details className="mt-4 rounded-mga border border-red-500/30 bg-red-500/10 px-4 py-3">
            <summary className="cursor-pointer text-sm font-semibold text-red-200">
              {recentMediaErrors.length} recent download problem{recentMediaErrors.length === 1 ? '' : 's'}
            </summary>
            <div className="flex flex-wrap items-center justify-between gap-2">
              <p className="mt-3 text-sm font-semibold text-red-200">Technical details</p>
              <p className="text-xs text-mga-muted">Showing {recentMediaErrors.length} of last 10</p>
            </div>
            <div className="mt-3 space-y-2">
              {recentMediaErrors.map((item) => (
                <div key={`${item.asset_id}-${item.failed_at ?? item.error}`} className="rounded-mga border border-red-500/20 bg-mga-bg/70 px-3 py-2">
                  <div className="flex flex-wrap items-center gap-x-3 gap-y-1 text-xs text-mga-muted">
                    <span>Asset {item.asset_id}</span>
                    <span>{item.attempts} attempt{item.attempts === 1 ? '' : 's'}</span>
                    <span>{item.permanent ? 'Permanent' : 'Retryable'}</span>
                    {item.failed_at && <span>{formatTime(item.failed_at)}</span>}
                  </div>
                  <p className="mt-1 break-all font-mono text-xs text-mga-muted">{item.url}</p>
                  <p className="mt-1 text-xs text-red-200">{item.error}</p>
                </div>
              ))}
            </div>
          </details>
        )}

        <div className="mt-4 grid gap-3 md:grid-cols-4">
          <div className="rounded-mga border border-mga-border bg-mga-bg px-4 py-3">
            <p className="text-xs uppercase tracking-wide text-mga-muted">Downloading</p>
            <p className="mt-1 text-2xl font-semibold text-mga-text">{media?.downloading ?? 0}</p>
          </div>
          <div className="rounded-mga border border-mga-border bg-mga-bg px-4 py-3">
            <p className="text-xs uppercase tracking-wide text-mga-muted">Queued</p>
            <p className="mt-1 text-2xl font-semibold text-mga-text">{media?.queued ?? 0}</p>
          </div>
          <div className="rounded-mga border border-mga-border bg-mga-bg px-4 py-3">
            <p className="text-xs uppercase tracking-wide text-mga-muted">Failed</p>
            <p className="mt-1 text-2xl font-semibold text-mga-text">{failedMediaCount}</p>
          </div>
          <div className="rounded-mga border border-mga-border bg-mga-bg px-4 py-3">
            <p className="text-xs uppercase tracking-wide text-mga-muted">Downloaded</p>
            <p className="mt-1 text-2xl font-semibold text-mga-text">
              {media?.downloaded ?? 0}/{media?.total ?? 0}
            </p>
          </div>
        </div>

        {mediaStatus.error && (
          <p className="mt-4 text-sm text-red-400">
            {mediaStatus.error instanceof Error ? mediaStatus.error.message : 'Failed to load media cache status.'}
          </p>
        )}
        {mediaActionError && <p className="mt-4 text-sm text-red-400">{mediaActionError}</p>}
      </section>

      <section className="rounded-mga border border-mga-border bg-mga-surface p-5">
        <div className="flex flex-wrap items-start justify-between gap-4">
          <div>
            <h2 className="text-lg font-semibold text-mga-text">Prepared game files</h2>
            <p className="mt-1 text-sm text-mga-muted">
              Temporary downloads used to start remote games faster or play them in your browser.
            </p>
          </div>
          <div className="flex flex-wrap gap-2">
            <Button variant="outline" size="sm" onClick={() => void refreshAll()}>
              Refresh
            </Button>
            <Button variant="outline" size="sm" onClick={() => void handleClear()} disabled={busyClear || totals.entries === 0}>
              {busyClear ? 'Clearing…' : 'Clear all'}
            </Button>
          </div>
        </div>

        <div className="mt-4 grid gap-3 md:grid-cols-3">
          <div className="rounded-mga border border-mga-border bg-mga-bg px-4 py-3">
            <p className="text-xs uppercase tracking-wide text-mga-muted">Entries</p>
            <p className="mt-1 text-2xl font-semibold text-mga-text">{totals.entries}</p>
          </div>
          <div className="rounded-mga border border-mga-border bg-mga-bg px-4 py-3">
            <p className="text-xs uppercase tracking-wide text-mga-muted">Files</p>
            <p className="mt-1 text-2xl font-semibold text-mga-text">{totals.files}</p>
          </div>
          <div className="rounded-mga border border-mga-border bg-mga-bg px-4 py-3">
            <p className="text-xs uppercase tracking-wide text-mga-muted">Size</p>
            <p className="mt-1 text-2xl font-semibold text-mga-text">{formatBytes(totals.bytes)}</p>
          </div>
        </div>

        {sourceActionError && <p className="mt-4 text-sm text-red-400">{sourceActionError}</p>}
      </section>

      <section className="rounded-mga border border-mga-border bg-mga-surface p-5">
        <div className="flex items-center justify-between gap-3">
          <div>
            <h3 className="text-base font-semibold text-mga-text">Recent activity</h3>
          </div>
        </div>

        <div className="mt-4 space-y-3">
          {jobs.isLoading && <p className="text-sm text-mga-muted">Loading cache jobs...</p>}
          {!jobs.isLoading && (jobs.data ?? []).length === 0 && (
            <p className="text-sm text-mga-muted">No games have needed preparation yet.</p>
          )}
          {(jobs.data ?? []).map((job) => (
            <div key={job.job_id} className="rounded-mga border border-mga-border bg-mga-bg px-4 py-3">
              <div className="flex flex-wrap items-center justify-between gap-2">
                <div>
                  <p className="text-sm font-medium text-mga-text">{job.canonical_title || job.source_title || job.source_game_id}</p>
                  <p className="mt-1 text-xs text-mga-muted">
                    {job.profile} · {job.status} · updated {formatTime(job.updated_at)}
                  </p>
                </div>
                <p className="text-xs text-mga-muted">
                  {job.progress_current ?? 0}/{job.progress_total ?? 0} files
                </p>
              </div>
              {(job.message || job.error) && (
                <p className={`mt-2 text-xs ${job.error ? 'text-red-400' : 'text-mga-muted'}`}>{job.error || job.message}</p>
              )}
            </div>
          ))}
        </div>
      </section>

      <section className="rounded-mga border border-mga-border bg-mga-surface p-5">
        <div>
          <h3 className="text-base font-semibold text-mga-text">Prepared games</h3>
        </div>

        <div className="mt-4 space-y-3">
          {entries.isLoading && <p className="text-sm text-mga-muted">Loading cache entries...</p>}
          {!entries.isLoading && (entries.data ?? []).length === 0 && (
            <p className="text-sm text-mga-muted">No prepared game files are stored.</p>
          )}
          {(entries.data ?? []).map((entry) => (
            <div key={entry.id} className="rounded-mga border border-mga-border bg-mga-bg px-4 py-3">
              <div className="flex flex-wrap items-start justify-between gap-3">
                <div className="space-y-1">
                  <p className="text-sm font-medium text-mga-text">{entry.canonical_title || entry.source_title || entry.source_game_id}</p>
                  <p className="text-xs text-mga-muted">
                    {entry.integration_label || entry.integration_id} · {entry.plugin_id} · {entry.profile}
                  </p>
                  <p className="text-xs text-mga-muted">
                    {entry.mode} · {entry.status} · {entry.file_count} files · {formatBytes(entry.size)}
                  </p>
                  {entry.source_path && <p className="text-xs text-mga-muted">{entry.source_path}</p>}
                  <p className="text-xs text-mga-muted">
                    Updated {formatTime(entry.updated_at)}
                    {entry.last_accessed_at ? ` · last accessed ${formatTime(entry.last_accessed_at)}` : ''}
                  </p>
                </div>
                <Button
                  variant="outline"
                  size="sm"
                  onClick={() => void handleDeleteEntry(entry.id)}
                  disabled={busyEntryId === entry.id}
                >
                  {busyEntryId === entry.id ? 'Removing…' : 'Remove'}
                </Button>
              </div>
            </div>
          ))}
        </div>
      </section>
    </div>
  )
}
