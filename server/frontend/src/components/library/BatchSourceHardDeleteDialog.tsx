import { useEffect, useMemo, useState } from 'react'
import { Loader2, Trash2 } from 'lucide-react'
import {
  ApiError,
  deleteSourceGames,
  previewDeleteSourceGame,
  type DeleteSourceGamePreview,
} from '@/api/client'
import { Button } from '@/components/ui/button'
import { Dialog } from '@/components/ui/dialog'

export type BatchSourceDeleteTarget = {
  key: string
  canonicalGameId: string
  sourceGameId: string
  sourceLabel: string
  gameTitle: string
}

type BatchPreviewEntry = {
  target: BatchSourceDeleteTarget
  preview?: DeleteSourceGamePreview
  error?: string
}

type BatchProgress = {
  completed: number
  total: number
  current?: string
}

interface BatchSourceHardDeleteDialogProps {
  open: boolean
  title: string
  description: string
  confirmCopy: string
  targets: BatchSourceDeleteTarget[]
  onClose: () => void
  onDeleted: (deleted: BatchSourceDeleteTarget[], warnings: string[]) => void | Promise<void>
}

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

function errorText(error: unknown, fallback: string): string {
  if (error instanceof ApiError) return error.responseText?.trim() || error.message
  if (error instanceof Error) return error.message
  return fallback
}

export function BatchSourceHardDeleteDialog({
  open,
  title,
  description,
  confirmCopy,
  targets,
  onClose,
  onDeleted,
}: BatchSourceHardDeleteDialogProps) {
  const [entries, setEntries] = useState<BatchPreviewEntry[]>([])
  const [previewBusy, setPreviewBusy] = useState(false)
  const [previewProgress, setPreviewProgress] = useState<BatchProgress>({ completed: 0, total: 0 })
  const [deleteBusy, setDeleteBusy] = useState(false)
  const [deleteProgress, setDeleteProgress] = useState<BatchProgress>({ completed: 0, total: 0 })
  const [confirmed, setConfirmed] = useState(false)
  const [error, setError] = useState('')

  useEffect(() => {
    if (!open) {
      setEntries([])
      setPreviewBusy(false)
      setPreviewProgress({ completed: 0, total: 0 })
      setDeleteBusy(false)
      setDeleteProgress({ completed: 0, total: 0 })
      setConfirmed(false)
      setError('')
      return
    }

    let cancelled = false
    setEntries([])
    setConfirmed(false)
    setError('')
    setDeleteProgress({ completed: 0, total: 0 })
    setPreviewProgress({ completed: 0, total: targets.length })
    setPreviewBusy(true)

    void (async () => {
      const nextEntries: BatchPreviewEntry[] = []
      for (const [index, target] of targets.entries()) {
        if (cancelled) return
        setPreviewProgress({ completed: index, total: targets.length, current: target.sourceLabel })
        try {
          const preview = await previewDeleteSourceGame(target.canonicalGameId, target.sourceGameId)
          nextEntries.push({ target, preview })
        } catch (err) {
          nextEntries.push({ target, error: errorText(err, 'Delete preview failed.') })
        }
        if (cancelled) return
        setEntries([...nextEntries])
        setPreviewProgress({ completed: index + 1, total: targets.length, current: target.sourceLabel })
      }
      if (!cancelled) setPreviewBusy(false)
    })()

    return () => {
      cancelled = true
    }
  }, [open, targets])

  const previewEntriesWithFiles = useMemo(
    () => entries.filter((entry) => entry.preview && entry.preview.items.length > 0),
    [entries],
  )
  const previewErrors = useMemo(() => entries.filter((entry) => entry.error), [entries])
  const totalPreviewItems = entries.reduce((sum, entry) => sum + (entry.preview?.items.length ?? 0), 0)
  const totalPreviewBytes = entries.reduce(
    (sum, entry) => sum + (entry.preview?.items ?? []).reduce((inner, item) => inner + (item.size ?? 0), 0),
    0,
  )
  const canApply =
    !previewBusy &&
    !deleteBusy &&
    confirmed &&
    previewErrors.length === 0 &&
    previewEntriesWithFiles.length > 0

  const close = () => {
    if (previewBusy || deleteBusy) return
    onClose()
  }

  const confirmDelete = async () => {
    if (!canApply) return
    setDeleteBusy(true)
    setError('')
    const deleted: BatchSourceDeleteTarget[] = []
    const warnings: string[] = []
    const targetsToDelete = previewEntriesWithFiles.map((entry) => entry.target)
    setDeleteProgress({ completed: 0, total: targetsToDelete.length })

    try {
      for (const [index, target] of targetsToDelete.entries()) {
        setDeleteProgress({ completed: index, total: targetsToDelete.length, current: `Deleting ${target.sourceLabel}...` })
        const result = await deleteSourceGames([{
          canonical_game_id: target.canonicalGameId,
          source_game_id: target.sourceGameId,
        }])
        const deletedIDSet = new Set(result.deleted_source_game_ids)
        if (!deletedIDSet.has(target.sourceGameId)) {
          throw new Error(`Delete did not return source id ${target.sourceGameId}.`)
        }
        deleted.push(target)
        warnings.push(...(result.warnings ?? []))
        setDeleteProgress({ completed: index + 1, total: targetsToDelete.length, current: `Deleted ${target.sourceLabel}.` })
      }
      if (deleted.length === 0) {
        throw new Error('Delete did not return any deleted source ids.')
      }
      await onDeleted(deleted, warnings)
      onClose()
    } catch (err) {
      if (deleted.length > 0) {
        await onDeleted(deleted, warnings)
      }
      setError(errorText(err, 'Hard delete failed.'))
    } finally {
      setDeleteBusy(false)
    }
  }

  return (
    <Dialog open={open} onClose={close} title={title}>
      <div className="space-y-4">
        <p className="text-sm text-mga-muted">{description}</p>

        {previewBusy ? (
          <div className="rounded-mga border border-mga-border bg-mga-bg p-3 text-sm text-mga-muted">
            <div className="flex items-center gap-2">
              <Loader2 size={16} className="animate-spin" />
              Building delete previews: {previewProgress.completed}/{previewProgress.total}
            </div>
            {previewProgress.current ? <p className="mt-1 truncate text-xs">{previewProgress.current}</p> : null}
          </div>
        ) : null}

        {entries.length > 0 ? (
          <div className="rounded-mga border border-red-500/30 bg-red-500/10 p-4 text-sm text-red-100">
            <div className="flex flex-wrap items-center justify-between gap-2">
              <p className="font-medium">
                {totalPreviewItems} file{totalPreviewItems === 1 ? '' : 's'} from {previewEntriesWithFiles.length} source record{previewEntriesWithFiles.length === 1 ? '' : 's'}
              </p>
              <p className="text-red-100/80">{formatBytes(totalPreviewBytes)}</p>
            </div>
            <div className="mt-3 max-h-[22rem] space-y-4 overflow-auto pr-1">
              {entries.map((entry) => (
                <div key={entry.target.key} className="rounded-mga border border-red-500/25 bg-red-950/20 p-3">
                  <div className="flex flex-wrap items-start justify-between gap-2">
                    <div>
                      <p className="font-medium text-red-50">{entry.target.sourceLabel}</p>
                      <p className="mt-1 text-xs text-red-100/80">{entry.target.gameTitle}</p>
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
                        <div key={`${entry.target.sourceGameId}:${item.path}:${item.object_id ?? item.action}`} className="flex items-start justify-between gap-3 text-xs">
                          <span className="break-all text-red-50">
                            {item.path}
                            {item.is_dir ? <span className="ml-2 text-red-100/70">(directory)</span> : null}
                          </span>
                          <span className="shrink-0 text-red-100/80">
                            {item.is_dir ? 'directory' : formatBytes(item.size ?? 0)}
                          </span>
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

        {error ? <p className="text-xs text-red-300">{error}</p> : null}
        {previewErrors.length > 0 ? <p className="text-xs text-amber-300">Resolve preview errors before applying deletion.</p> : null}

        {entries.length > 0 && !previewBusy ? (
          <label className="flex items-start gap-3 rounded-mga border border-red-500/30 bg-red-500/5 p-3 text-sm leading-6 text-red-100">
            <input
              type="checkbox"
              checked={confirmed}
              onChange={(event) => setConfirmed(event.target.checked)}
              disabled={deleteBusy || previewErrors.length > 0 || previewEntriesWithFiles.length === 0}
              className="mt-1 h-4 w-4 rounded border-red-400 bg-mga-bg accent-red-600"
            />
            <span>{confirmCopy}</span>
          </label>
        ) : null}

        <div className="flex justify-end gap-3">
          <Button type="button" variant="outline" onClick={close} disabled={previewBusy || deleteBusy}>
            Cancel
          </Button>
          <Button
            type="button"
            variant="outline"
            onClick={() => void confirmDelete()}
            disabled={!canApply}
            className="border-red-500/30 text-red-200 hover:bg-red-500/10"
          >
            <Trash2 size={16} />
            {deleteBusy ? 'Deleting...' : 'Apply Delete'}
          </Button>
        </div>
      </div>
    </Dialog>
  )
}
