import { useState } from 'react'
import { useQuery, useQueryClient } from '@tanstack/react-query'
import { Button } from '@/components/ui/button'
import { clearCacheEntries, deleteCacheEntry, listCacheEntries, listCacheJobs } from '@/api/client'

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
  const [actionError, setActionError] = useState('')

  const entries = useQuery({
    queryKey: ['cache-entries'],
    queryFn: listCacheEntries,
  })

  const jobs = useQuery({
    queryKey: ['cache-jobs'],
    queryFn: () => listCacheJobs(20),
    refetchInterval: 2000,
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
    ])
  }

  const handleDeleteEntry = async (entryId: string) => {
    setBusyEntryId(entryId)
    setActionError('')
    try {
      await deleteCacheEntry(entryId)
      await refreshAll()
    } catch (error) {
      setActionError(error instanceof Error ? error.message : 'Failed to delete cache entry.')
    } finally {
      setBusyEntryId(null)
    }
  }

  const handleClear = async () => {
    const confirmed = window.confirm('Clear all cached source entries?')
    if (!confirmed) return
    setBusyClear(true)
    setActionError('')
    try {
      await clearCacheEntries()
      await refreshAll()
    } catch (error) {
      setActionError(error instanceof Error ? error.message : 'Failed to clear cache entries.')
    } finally {
      setBusyClear(false)
    }
  }

  return (
    <div className="space-y-6">
      <section className="rounded-mga border border-mga-border bg-mga-surface p-5">
        <div className="flex flex-wrap items-start justify-between gap-4">
          <div>
            <h2 className="text-lg font-semibold text-mga-text">Source Cache</h2>
            <p className="mt-1 text-sm text-mga-muted">
              Reusable materialized source files for remote integrations such as Google Drive.
            </p>
          </div>
          <div className="flex flex-wrap gap-2">
            <Button variant="outline" size="sm" onClick={() => void refreshAll()}>
              Refresh
            </Button>
            <Button variant="outline" size="sm" onClick={() => void handleClear()} disabled={busyClear || totals.entries === 0}>
              {busyClear ? 'Clearing...' : 'Clear All'}
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

        {actionError && <p className="mt-4 text-sm text-red-400">{actionError}</p>}
      </section>

      <section className="rounded-mga border border-mga-border bg-mga-surface p-5">
        <div className="flex items-center justify-between gap-3">
          <div>
            <h3 className="text-base font-semibold text-mga-text">Recent Jobs</h3>
            <p className="mt-1 text-sm text-mga-muted">Persisted prepare jobs for source materialization.</p>
          </div>
        </div>

        <div className="mt-4 space-y-3">
          {jobs.isLoading && <p className="text-sm text-mga-muted">Loading cache jobs...</p>}
          {!jobs.isLoading && (jobs.data ?? []).length === 0 && (
            <p className="text-sm text-mga-muted">No cache jobs have been recorded yet.</p>
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
          <h3 className="text-base font-semibold text-mga-text">Entries</h3>
          <p className="mt-1 text-sm text-mga-muted">Ready and failed cache entries managed by source game and profile.</p>
        </div>

        <div className="mt-4 space-y-3">
          {entries.isLoading && <p className="text-sm text-mga-muted">Loading cache entries...</p>}
          {!entries.isLoading && (entries.data ?? []).length === 0 && (
            <p className="text-sm text-mga-muted">No cached source entries are stored yet.</p>
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
                  {busyEntryId === entry.id ? 'Removing...' : 'Evict'}
                </Button>
              </div>
            </div>
          ))}
        </div>
      </section>
    </div>
  )
}
