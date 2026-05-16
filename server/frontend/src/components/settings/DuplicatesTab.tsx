import { useEffect, useMemo, useState } from 'react'
import { Link } from 'react-router-dom'
import { useQuery, useQueryClient } from '@tanstack/react-query'
import { FileText, Loader2, RefreshCw, Trash2 } from 'lucide-react'
import {
  ApiError,
  deleteSourceGames,
  getDuplicateGames,
  previewDeleteSourceGame,
  type DeleteSourceGamePreview,
  type DuplicateGameMode,
  type DuplicateGameSource,
} from '@/api/client'
import { Button } from '@/components/ui/button'
import { CoverImage } from '@/components/ui/cover-image'
import { Dialog } from '@/components/ui/dialog'
import { platformLabel, selectCoverUrl } from '@/lib/gameUtils'

const MODES: Array<{ id: DuplicateGameMode; label: string; description: string }> = [
  {
    id: 'loose',
    label: 'Possible duplicates',
    description: 'Groups matching titles across sources and platforms.',
  },
  {
    id: 'strict',
    label: 'Exact variants',
    description: 'Groups same-title source records inside the same canonical game.',
  },
]

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

function sourceLabel(source: DuplicateGameSource): string {
  const record = source.source
  return `${record.integration_label || record.integration_id} · ${record.raw_title || record.external_id}`
}

function duplicateTitle(source: DuplicateGameSource): string {
  return source.game?.title || source.canonical_title || source.source.raw_title || source.source.external_id
}

function duplicateSubtitle(source: DuplicateGameSource): string {
  const platform = platformLabel(source.game?.platform || source.source.platform || 'unknown')
  const kind = source.game?.kind || source.source.kind || 'unknown'
  return `${platform} · ${kind}`
}

function sourceKey(source: DuplicateGameSource): string {
  return source.source.id
}

function errorText(error: unknown, fallback: string): string {
  if (error instanceof ApiError) return error.responseText?.trim() || error.message
  if (error instanceof Error) return error.message
  return fallback
}

type BatchPreviewEntry = {
  source: DuplicateGameSource
  preview?: DeleteSourceGamePreview
  error?: string
}

type BatchProgress = {
  completed: number
  total: number
  current?: string
}

export function DuplicatesTab() {
  const queryClient = useQueryClient()
  const [mode, setMode] = useState<DuplicateGameMode>('loose')
  const [selectedIds, setSelectedIds] = useState<Set<string>>(() => new Set())
  const [batchDialogOpen, setBatchDialogOpen] = useState(false)
  const [batchPreviews, setBatchPreviews] = useState<BatchPreviewEntry[]>([])
  const [previewBusy, setPreviewBusy] = useState(false)
  const [previewProgress, setPreviewProgress] = useState<BatchProgress>({ completed: 0, total: 0 })
  const [deleteBusy, setDeleteBusy] = useState(false)
  const [deleteProgress, setDeleteProgress] = useState<BatchProgress>({ completed: 0, total: 0 })
  const [confirmed, setConfirmed] = useState(false)
  const [batchError, setBatchError] = useState('')
  const [notice, setNotice] = useState('')

  const duplicates = useQuery({
    queryKey: ['duplicate-games', mode],
    queryFn: () => getDuplicateGames(mode),
    refetchOnWindowFocus: false,
  })

  const groups = duplicates.data?.groups ?? []
  const loading = duplicates.isPending

  const sourceById = useMemo(() => {
    const next = new Map<string, DuplicateGameSource>()
    for (const group of groups) {
      for (const source of group.sources) {
        next.set(sourceKey(source), source)
      }
    }
    return next
  }, [groups])

  const selectedSources = useMemo(
    () => Array.from(selectedIds).map((id) => sourceById.get(id)).filter((source): source is DuplicateGameSource => Boolean(source)),
    [selectedIds, sourceById],
  )

  const selectedDeleteableSources = selectedSources.filter((source) => source.source.hard_delete?.eligible ?? false)
  const previewEntriesWithFiles = batchPreviews.filter((entry) => entry.preview && entry.preview.items.length > 0)
  const previewErrors = batchPreviews.filter((entry) => entry.error)
  const totalPreviewItems = batchPreviews.reduce((sum, entry) => sum + (entry.preview?.items.length ?? 0), 0)
  const totalPreviewBytes = batchPreviews.reduce(
    (sum, entry) => sum + (entry.preview?.items ?? []).reduce((inner, item) => inner + (item.size ?? 0), 0),
    0,
  )
  const canApplyBatch =
    !previewBusy &&
    !deleteBusy &&
    confirmed &&
    previewErrors.length === 0 &&
    previewEntriesWithFiles.length > 0

  useEffect(() => {
    setSelectedIds(new Set())
  }, [mode])

  useEffect(() => {
    setSelectedIds((prev) => {
      const next = new Set<string>()
      for (const id of prev) {
        if (sourceById.has(id)) next.add(id)
      }
      return next.size === prev.size ? prev : next
    })
  }, [sourceById])

  const refreshAfterDelete = async (sources: DuplicateGameSource[]) => {
    const canonicalIds = Array.from(new Set(sources.map((source) => source.canonical_game_id).filter(Boolean)))
    await Promise.all([
      queryClient.invalidateQueries({ queryKey: ['duplicate-games'] }),
      queryClient.invalidateQueries({ queryKey: ['games'] }),
      ...canonicalIds.flatMap((canonicalId) => [
        queryClient.invalidateQueries({ queryKey: ['game', canonicalId] }),
        queryClient.invalidateQueries({ queryKey: ['game', canonicalId, 'achievements'] }),
      ]),
      queryClient.invalidateQueries({ queryKey: ['stats'] }),
      queryClient.invalidateQueries({ queryKey: ['library-statistics'] }),
      queryClient.invalidateQueries({ queryKey: ['gamer-statistics'] }),
      queryClient.invalidateQueries({ queryKey: ['cache-entries'] }),
      queryClient.invalidateQueries({ queryKey: ['cache-jobs'] }),
    ])
  }

  const toggleSelected = (source: DuplicateGameSource) => {
    const id = sourceKey(source)
    setNotice('')
    setSelectedIds((prev) => {
      const next = new Set(prev)
      if (next.has(id)) {
        next.delete(id)
      } else {
        next.add(id)
      }
      return next
    })
  }

  const openBatchPreview = async () => {
    if (selectedDeleteableSources.length === 0) return
    setBatchDialogOpen(true)
    setBatchPreviews([])
    setConfirmed(false)
    setBatchError('')
    setDeleteProgress({ completed: 0, total: 0 })
    setPreviewBusy(true)
    setPreviewProgress({ completed: 0, total: selectedDeleteableSources.length })

    const entries: BatchPreviewEntry[] = []
    for (const [index, source] of selectedDeleteableSources.entries()) {
      setPreviewProgress({ completed: index, total: selectedDeleteableSources.length, current: sourceLabel(source) })
      try {
        const preview = await previewDeleteSourceGame(source.canonical_game_id, source.source.id)
        entries.push({ source, preview })
      } catch (err) {
        entries.push({ source, error: errorText(err, 'Delete preview failed.') })
      }
      setBatchPreviews([...entries])
      setPreviewProgress({ completed: index + 1, total: selectedDeleteableSources.length, current: sourceLabel(source) })
    }
    setPreviewBusy(false)
  }

  const closeBatchDialog = () => {
    if (previewBusy || deleteBusy) return
    setBatchDialogOpen(false)
    setBatchPreviews([])
    setBatchError('')
    setConfirmed(false)
  }

  const confirmBatchDelete = async () => {
    if (!canApplyBatch) return
    setDeleteBusy(true)
    setBatchError('')
    const entries = previewEntriesWithFiles
    setDeleteProgress({ completed: 0, total: entries.length })
    setDeleteProgress({ completed: 0, total: entries.length, current: 'Deleting selected source records...' })

    try {
      const result = await deleteSourceGames(entries.map((entry) => ({
        canonical_game_id: entry.source.canonical_game_id,
        source_game_id: entry.source.source.id,
      })))
      const deletedIDSet = new Set(result.deleted_source_game_ids)
      if (deletedIDSet.size === 0) {
        throw new Error('Delete did not return any deleted source ids.')
      }
      const deletedSources = entries.map((entry) => entry.source).filter((source) => deletedIDSet.has(source.source.id))
      setDeleteProgress({ completed: deletedSources.length, total: entries.length, current: 'Deleted selected source records.' })
      setSelectedIds((prev) => {
        const next = new Set(prev)
        for (const source of deletedSources) {
          next.delete(sourceKey(source))
        }
        return next
      })
      setNotice(`Deleted ${deletedSources.length} duplicate source record${deletedSources.length === 1 ? '' : 's'}.`)
      setBatchDialogOpen(false)
      setBatchPreviews([])
      setConfirmed(false)
      void refreshAfterDelete(deletedSources)
    } catch (err) {
      setBatchError(errorText(err, 'Hard delete failed.'))
    } finally {
      setDeleteBusy(false)
    }
  }

  return (
    <div className="space-y-6">
      <section className="rounded-mga border border-mga-border bg-mga-surface p-5">
        <div className="flex flex-wrap items-start justify-between gap-4">
          <div>
            <h2 className="text-lg font-semibold text-mga-text">Duplicate Games</h2>
            <p className="mt-1 text-sm text-mga-muted">
              Review duplicate-looking source records and hard delete unwanted file-backed entries.
            </p>
          </div>
          <Button variant="outline" size="sm" onClick={() => void duplicates.refetch()} disabled={duplicates.isFetching}>
            {duplicates.isFetching ? <Loader2 size={16} className="animate-spin" /> : <RefreshCw size={16} />}
            Refresh
          </Button>
        </div>

        <div className="mt-5 grid gap-2 sm:grid-cols-2">
          {MODES.map((item) => (
            <button
              key={item.id}
              type="button"
              onClick={() => setMode(item.id)}
              className={`rounded-mga border px-4 py-3 text-left transition-colors ${
                mode === item.id
                  ? 'border-mga-accent bg-mga-accent/10 text-mga-text'
                  : 'border-mga-border bg-mga-bg text-mga-muted hover:border-mga-accent/50 hover:text-mga-text'
              }`}
            >
              <span className="block text-sm font-semibold">{item.label}</span>
              <span className="mt-1 block text-xs">{item.description}</span>
            </button>
          ))}
        </div>
      </section>

      {notice ? (
        <div className="rounded-mga border border-emerald-500/30 bg-emerald-500/10 px-4 py-3 text-sm text-emerald-100">
          {notice}
        </div>
      ) : null}

      {selectedDeleteableSources.length > 0 ? (
        <div className="rounded-mga border border-red-500/30 bg-red-500/10 p-4">
          <div className="flex flex-wrap items-center justify-between gap-3">
            <div>
              <p className="text-sm font-medium text-red-100">
                {selectedDeleteableSources.length} duplicate source record{selectedDeleteableSources.length === 1 ? '' : 's'} marked for deletion
              </p>
              <p className="mt-1 text-xs text-red-100/80">
                Build a preview to review every backing file before applying the hard delete.
              </p>
            </div>
            <div className="flex flex-wrap gap-2">
              <Button type="button" variant="outline" size="sm" onClick={() => setSelectedIds(new Set())} disabled={previewBusy || deleteBusy}>
                Clear marks
              </Button>
              <Button
                type="button"
                variant="outline"
                size="sm"
                onClick={() => void openBatchPreview()}
                disabled={previewBusy || deleteBusy}
                className="border-red-500/30 text-red-200 hover:bg-red-500/10"
              >
                <FileText size={14} />
                Preview and Apply
              </Button>
            </div>
          </div>
        </div>
      ) : null}

      {duplicates.error ? (
        <div className="rounded-mga border border-red-500/30 bg-red-500/10 px-4 py-3 text-sm text-red-200">
          {duplicates.error instanceof Error ? duplicates.error.message : 'Failed to load duplicate games.'}
        </div>
      ) : null}

      {loading ? (
        <div className="flex items-center gap-2 rounded-mga border border-mga-border bg-mga-surface px-4 py-3 text-sm text-mga-muted">
          <Loader2 size={16} className="animate-spin" />
          Loading duplicate groups...
        </div>
      ) : groups.length === 0 ? (
        <div className="rounded-mga border border-mga-border bg-mga-surface px-4 py-8 text-center text-sm text-mga-muted">
          No duplicate groups found in this mode.
        </div>
      ) : (
        <div className="space-y-4">
          {groups.map((group) => (
            <section key={group.id} className="rounded-mga border border-mga-border bg-mga-surface p-4">
              <div className="flex flex-wrap items-start justify-between gap-3">
                <div>
                  <h3 className="text-base font-semibold text-mga-text">{group.representative_title || group.normalized_title}</h3>
                  <div className="mt-1 flex flex-wrap gap-2 text-xs text-mga-muted">
                    <span>{group.sources.length} source records</span>
                    <span>{group.canonical_ids.length} canonical game{group.canonical_ids.length === 1 ? '' : 's'}</span>
                    <span>{group.mode === 'loose' ? 'Possible duplicate' : 'Exact variant'}</span>
                  </div>
                </div>
              </div>

              <div className="mt-4 overflow-x-auto">
                <table className="min-w-full text-left text-sm">
                  <thead className="text-xs uppercase tracking-wide text-mga-muted">
                    <tr className="border-b border-mga-border">
                      <th className="py-2 pr-4 font-medium">Source</th>
                      <th className="py-2 pr-4 font-medium">Game</th>
                      <th className="py-2 pr-4 font-medium">Files</th>
                      <th className="py-2 pr-4 font-medium">Signals</th>
                      <th className="py-2 font-medium">Action</th>
                    </tr>
                  </thead>
                  <tbody>
                    {group.sources.map((source) => {
                      const eligible = source.source.hard_delete?.eligible ?? false
                      return (
                        <tr key={source.source.id} className="border-b border-mga-border/60 last:border-0">
                          <td className="py-3 pr-4 align-top">
                            <p className="font-medium text-mga-text">{source.source.integration_label || source.source.integration_id}</p>
                            <p className="mt-1 text-xs text-mga-muted">{source.source.plugin_id}</p>
                          </td>
                          <td className="py-3 pr-4 align-top">
                            <Link
                              to={`/game/${encodeURIComponent(source.canonical_game_id)}`}
                              className="group flex min-w-[16rem] max-w-md items-start gap-3 rounded-mga p-1 -m-1 transition-colors hover:bg-mga-elevated/70"
                            >
                              <div className="h-20 w-14 shrink-0 overflow-hidden rounded-mga border border-mga-border bg-mga-bg">
                                <CoverImage
                                  src={selectCoverUrl(source.game?.media, source.game?.cover_override)}
                                  alt={duplicateTitle(source)}
                                  fit="contain"
                                  variant="compact"
                                  className="h-full w-full"
                                />
                              </div>
                              <div className="min-w-0">
                                <p className="font-medium text-mga-text transition-colors group-hover:text-mga-accent">
                                  {duplicateTitle(source)}
                                </p>
                                <p className="mt-1 text-xs text-mga-muted">{duplicateSubtitle(source)}</p>
                                <p className="mt-1 text-xs text-mga-muted">
                                  Source title: {source.source.raw_title || source.source.external_id}
                                </p>
                              </div>
                            </Link>
                            {source.source.root_path ? (
                              <p className="mt-1 max-w-md break-all font-mono text-xs text-mga-muted">{source.source.root_path}</p>
                            ) : null}
                          </td>
                          <td className="py-3 pr-4 align-top text-mga-muted">
                            <p>{source.file_count} file{source.file_count === 1 ? '' : 's'}</p>
                            <p className="mt-1 text-xs">{formatBytes(source.total_size)}</p>
                          </td>
                          <td className="py-3 pr-4 align-top text-xs text-mga-muted">
                            <div className="flex flex-wrap gap-1.5">
                              {source.cached ? <span className="rounded-full bg-sky-500/10 px-2 py-1 text-sky-200">Cached</span> : null}
                              {source.has_cached_achievements ? <span className="rounded-full bg-amber-500/10 px-2 py-1 text-amber-200">Achievements</span> : null}
                              {source.source.play?.launchable ? <span className="rounded-full bg-emerald-500/10 px-2 py-1 text-emerald-200">Playable</span> : null}
                              {!source.cached && !source.has_cached_achievements && !source.source.play?.launchable ? (
                                <span className="text-mga-muted">None</span>
                              ) : null}
                            </div>
                          </td>
                          <td className="py-3 align-top">
                            <Button
                              type="button"
                              variant="outline"
                              size="sm"
                              onClick={() => toggleSelected(source)}
                              disabled={!eligible || previewBusy || deleteBusy}
                              className={
                                selectedIds.has(sourceKey(source))
                                  ? 'border-red-500/50 bg-red-500/15 text-red-100 hover:bg-red-500/20'
                                  : 'border-red-500/30 text-red-200 hover:bg-red-500/10'
                              }
                            >
                              <Trash2 size={14} />
                              {selectedIds.has(sourceKey(source)) ? 'Marked' : 'Mark Delete'}
                            </Button>
                            {!eligible && source.source.hard_delete?.reason ? (
                              <p className="mt-2 max-w-52 text-xs text-amber-300">{source.source.hard_delete.reason}</p>
                            ) : null}
                          </td>
                        </tr>
                      )
                    })}
                  </tbody>
                </table>
              </div>
            </section>
          ))}
        </div>
      )}

      <Dialog open={batchDialogOpen} onClose={closeBatchDialog} title="Apply Duplicate Hard Delete">
        <div className="space-y-4">
          <p className="text-sm text-mga-muted">
            Review the backing files below before applying deletion. Google Drive files will be moved to Drive trash; other file-backed sources use their configured delete behavior.
          </p>

          {previewBusy ? (
            <div className="rounded-mga border border-mga-border bg-mga-bg p-3 text-sm text-mga-muted">
              <div className="flex items-center gap-2">
                <Loader2 size={16} className="animate-spin" />
                Building delete previews: {previewProgress.completed}/{previewProgress.total}
              </div>
              {previewProgress.current ? <p className="mt-1 truncate text-xs">{previewProgress.current}</p> : null}
            </div>
          ) : null}

          {batchPreviews.length > 0 ? (
            <div className="rounded-mga border border-red-500/30 bg-red-500/10 p-4 text-sm text-red-100">
              <div className="flex flex-wrap items-center justify-between gap-2">
                <p className="font-medium">
                  {totalPreviewItems} file{totalPreviewItems === 1 ? '' : 's'} from {previewEntriesWithFiles.length} source record{previewEntriesWithFiles.length === 1 ? '' : 's'}
                </p>
                <p className="text-red-100/80">{formatBytes(totalPreviewBytes)}</p>
              </div>
              <div className="mt-3 max-h-[22rem] space-y-4 overflow-auto pr-1">
                {batchPreviews.map((entry) => (
                  <div key={sourceKey(entry.source)} className="rounded-mga border border-red-500/25 bg-red-950/20 p-3">
                    <div className="flex flex-wrap items-start justify-between gap-2">
                      <div>
                        <p className="font-medium text-red-50">{sourceLabel(entry.source)}</p>
                        <p className="mt-1 text-xs text-red-100/80">{duplicateTitle(entry.source)}</p>
                      </div>
                      {entry.preview ? <span className="text-xs text-red-100/80">{entry.preview.items.length} file{entry.preview.items.length === 1 ? '' : 's'}</span> : null}
                    </div>
                    {entry.error ? <p className="mt-2 text-xs text-red-200">{entry.error}</p> : null}
                    {entry.preview?.warnings?.length ? (
                      <div className="mt-2 space-y-1 text-xs text-amber-200">
                        {entry.preview.warnings.map((warning) => (
                          <p key={warning}>{warning}</p>
                        ))}
                      </div>
                    ) : null}
                    {entry.preview?.items.length ? (
                      <div className="mt-3 space-y-2">
                        {entry.preview.items.map((item) => (
                          <div key={`${entry.source.source.id}:${item.path}:${item.object_id ?? item.action}`} className="flex items-start justify-between gap-3 text-xs">
                            <span className="break-all text-red-50">{item.path}</span>
                            <span className="shrink-0 text-red-100/80">{formatBytes(item.size ?? 0)}</span>
                          </div>
                        ))}
                      </div>
                    ) : null}
                  </div>
                ))}
              </div>
            </div>
          ) : null}

          {deleteBusy ? (
            <div className="rounded-mga border border-mga-border bg-mga-bg p-3 text-sm text-mga-muted">
              <div className="flex items-center gap-2">
                <Loader2 size={16} className="animate-spin" />
                Deleting: {deleteProgress.completed}/{deleteProgress.total}
              </div>
              <div className="mt-2 h-2 overflow-hidden rounded-full bg-mga-surface">
                <div
                  className="h-full rounded-full bg-red-400 transition-all"
                  style={{ width: `${deleteProgress.total > 0 ? (deleteProgress.completed / deleteProgress.total) * 100 : 0}%` }}
                />
              </div>
              {deleteProgress.current ? <p className="mt-1 truncate text-xs">{deleteProgress.current}</p> : null}
            </div>
          ) : null}

          {batchError ? <p className="text-xs text-red-300">{batchError}</p> : null}

          {previewErrors.length > 0 ? (
            <p className="text-xs text-amber-300">
              Resolve preview errors before applying deletion.
            </p>
          ) : null}

          {batchPreviews.length > 0 && !previewBusy ? (
            <label className="flex items-start gap-3 rounded-mga border border-red-500/30 bg-red-500/5 p-3 text-sm leading-6 text-red-100">
              <input
                type="checkbox"
                checked={confirmed}
                onChange={(event) => setConfirmed(event.target.checked)}
                disabled={deleteBusy || previewErrors.length > 0 || previewEntriesWithFiles.length === 0}
                className="mt-1 h-4 w-4 rounded border-red-400 bg-mga-bg accent-red-600"
              />
              <span>I reviewed the listed files and want to hard delete the marked duplicate source records.</span>
            </label>
          ) : null}

          <div className="flex justify-end gap-3">
            <Button type="button" variant="outline" onClick={closeBatchDialog} disabled={previewBusy || deleteBusy}>
              Cancel
            </Button>
            <Button
              type="button"
              variant="outline"
              onClick={() => void confirmBatchDelete()}
              disabled={!canApplyBatch}
              className="border-red-500/30 text-red-200 hover:bg-red-500/10"
            >
              <Trash2 size={16} />
              {deleteBusy ? 'Deleting...' : 'Apply Delete'}
            </Button>
          </div>
        </div>
      </Dialog>
    </div>
  )
}
