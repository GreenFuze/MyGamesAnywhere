import { useEffect, useMemo, useState } from 'react'
import { FileText, Loader2 } from 'lucide-react'
import {
  ApiError,
  deleteSourceGame,
  previewDeleteSourceGame,
  type DeleteSourceGameResponse,
  type DeleteSourceGamePreview,
  type SourceGameDetailDTO,
} from '@/api/client'
import { Button } from '@/components/ui/button'
import { Dialog } from '@/components/ui/dialog'
import {
  HardDeleteOperationRegistry,
  type HardDeleteOperationSnapshot,
} from '@/lib/hardDeleteOperation'

const hardDeleteOperations = new HardDeleteOperationRegistry<DeleteSourceGameResponse>()

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

export type SourceGameHardDeleteDialogProps = {
  canonicalGameId: string | null
  source: SourceGameDetailDTO | null
  title?: string
  confirmLabel?: string
  sourceLabel: (source: SourceGameDetailDTO) => string
  onClose: () => void
  onDeleted: (result: DeleteSourceGameResponse, source: SourceGameDetailDTO) => void | Promise<void>
}

export function SourceGameHardDeleteDialog({
  canonicalGameId,
  source,
  title = 'Hard Delete Source Record',
  confirmLabel = 'Delete Source Record',
  sourceLabel,
  onClose,
  onDeleted,
}: SourceGameHardDeleteDialogProps) {
  const [preview, setPreview] = useState<DeleteSourceGamePreview | null>(null)
  const [confirmed, setConfirmed] = useState(false)
  const [previewBusy, setPreviewBusy] = useState(false)
  const [error, setError] = useState('')
  const operationKey = canonicalGameId && source ? `${canonicalGameId}:${source.id}` : ''
  const operation = useMemo(
    () => (operationKey ? hardDeleteOperations.get(operationKey) : null),
    [operationKey],
  )
  const [operationState, setOperationState] = useState<HardDeleteOperationSnapshot<DeleteSourceGameResponse>>({
    status: 'idle',
    result: null,
    error: null,
  })
  const deleteBusy = operationState.status === 'submitting'

  useEffect(() => {
    if (!operation) {
      setOperationState({ status: 'idle', result: null, error: null })
      return
    }
    return operation.subscribe(setOperationState)
  }, [operation])

  useEffect(() => {
    let cancelled = false

    setPreview(null)
    setConfirmed(false)
    setError('')

    if (!canonicalGameId || !source) {
      setPreviewBusy(false)
      return
    }

    const sourceId = source.id
    setPreviewBusy(true)
    previewDeleteSourceGame(canonicalGameId, sourceId)
      .then((nextPreview) => {
        if (cancelled) return
        setPreview(nextPreview)
        setConfirmed(false)
      })
      .catch((err: unknown) => {
        if (cancelled) return
        setError(errorText(err, 'Delete preview failed.'))
      })
      .finally(() => {
        if (!cancelled) setPreviewBusy(false)
      })

    return () => {
      cancelled = true
    }
  }, [canonicalGameId, source?.id])

  const requestClose = () => {
    if (previewBusy || deleteBusy) return
    onClose()
  }

  const confirmDelete = async () => {
    if (!canonicalGameId || !source || !preview || preview.items.length === 0 || !confirmed || deleteBusy || !operation) return
    const authorizedSource = source
    setError('')
    try {
      await operation.authorize(async () => {
        const result = await deleteSourceGame(canonicalGameId, authorizedSource.id)
        await onDeleted(result, authorizedSource)
        return result
      })
    } catch (err) {
      setError(errorText(err, 'Hard delete failed.'))
    }
  }

  const prepareRetry = () => {
    if (!operation?.prepareRetry()) return
    setConfirmed(false)
    setError('')
  }

  return (
    <Dialog open={source !== null} onClose={requestClose} title={title}>
      {source ? (
        <div className="space-y-4">
          <p className="text-sm text-mga-muted">
            This {preview?.plugin_id === 'game-source-google-drive'
              ? 'moves the backing files shown below to Google Drive trash'
              : preview?.action === 'trash'
                ? 'moves the backing files shown below to trash'
                : 'permanently deletes the backing files shown below'} and removes the stored source record for{' '}
            <span className="font-medium text-mga-text">{sourceLabel(source)}</span>.
          </p>
          {previewBusy ? (
            <div className="flex items-center gap-2 rounded-mga border border-mga-border bg-mga-bg px-3 py-2 text-sm text-mga-muted">
              <Loader2 size={16} className="animate-spin" />
              Building delete preview from source plugin...
            </div>
          ) : null}
          {preview ? (
            <div className="space-y-3 rounded-mga border border-red-500/30 bg-red-500/10 p-4 text-sm leading-6 text-red-100">
              <p className="font-medium text-red-50">{preview.summary}</p>
              <div className="max-h-56 space-y-2 overflow-auto">
                {preview.items.map((item) => (
                  <div key={`${item.path}:${item.object_id ?? item.action}`} className="flex items-start justify-between gap-3">
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
              {preview.warnings?.length ? (
                <div className="border-t border-red-500/30 pt-3">
                  {preview.warnings.map((warning) => (
                    <p key={warning} className="text-red-100/80">{warning}</p>
                  ))}
                </div>
              ) : null}
            </div>
          ) : null}
          {!preview && source.root_path ? (
            <p className="rounded-mga border border-mga-border bg-mga-bg px-3 py-2 text-xs text-mga-muted">
              Root path: {source.root_path}
            </p>
          ) : null}
          {error ? <p className="text-xs text-red-400">{error}</p> : null}
          {operationState.status === 'submitting' ? (
            <p className="rounded-mga border border-amber-400/25 bg-amber-400/10 px-3 py-2 text-xs text-amber-200">
              Delete is already in progress. Returning to this window will not submit it again.
            </p>
          ) : null}
          {operationState.status === 'succeeded' ? (
            <p className="rounded-mga border border-green-500/25 bg-green-500/10 px-3 py-2 text-xs text-green-300">
              Delete completed. MGA is updating the library.
            </p>
          ) : null}
          {operationState.status === 'failed' ? (
            <div className="flex items-center justify-between gap-3 rounded-mga border border-red-500/25 bg-red-500/10 px-3 py-2 text-xs text-red-200">
              <span>The delete did not complete. Retry requires a new confirmation.</span>
              <Button type="button" variant="outline" size="sm" onClick={prepareRetry}>
                Try again
              </Button>
            </div>
          ) : null}
          {source.hard_delete?.reason && !source.hard_delete.eligible ? (
            <p className="text-xs text-amber-300">{source.hard_delete.reason}</p>
          ) : null}
          {preview ? (
            <label className="flex items-start gap-3 rounded-mga border border-red-500/30 bg-red-500/5 p-3 text-sm leading-6 text-red-100">
              <input
                type="checkbox"
                checked={confirmed}
                onChange={(event) => setConfirmed(event.target.checked)}
                disabled={operationState.status !== 'idle' || preview.items.length === 0}
                className="mt-1 h-4 w-4 rounded border-red-400 bg-mga-bg accent-red-600"
              />
              <span>I understand this is the real delete action and want to continue.</span>
            </label>
          ) : null}
          <div className="flex justify-end gap-3">
            <Button type="button" variant="outline" onClick={requestClose} disabled={deleteBusy || previewBusy}>
              Cancel
            </Button>
            {preview ? (
              <Button
                type="button"
                variant="outline"
                onClick={() => void confirmDelete()}
                disabled={operationState.status !== 'idle' || preview.items.length === 0 || !confirmed}
                className="border-red-500/30 text-red-200 hover:bg-red-500/10"
              >
                <FileText size={16} />
                {deleteBusy ? 'Deleting...' : confirmLabel}
              </Button>
            ) : null}
          </div>
        </div>
      ) : null}
    </Dialog>
  )
}
