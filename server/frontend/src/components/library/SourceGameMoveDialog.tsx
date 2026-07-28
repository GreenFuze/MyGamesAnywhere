import { useEffect, useMemo, useRef, useState } from 'react'
import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query'
import { ArrowRightLeft, Loader2, RotateCcw, ShieldCheck, Trash2 } from 'lucide-react'
import {
  getSourceMoveJob,
  listSourceMoveDestinations,
  listSourceMoveJobs,
  previewSourceMoves,
  runSourceMoveAction,
  startSourceMoves,
  type SourceGameDetailDTO,
  type SourceMoveJob,
  type SourceMovePreviewItem,
  type SourceMoveSelection,
} from '@/api/client'
import { Button } from '@/components/ui/button'
import { Dialog } from '@/components/ui/dialog'
import { Input } from '@/components/ui/input'
import { ProgressBar } from '@/components/ui/progress-bar'
import { Select } from '@/components/ui/select'

function formatBytes(value: number): string {
  if (!Number.isFinite(value) || value <= 0) return '0 B'
  const units = ['B', 'KB', 'MB', 'GB', 'TB']
  const exponent = Math.min(Math.floor(Math.log(value) / Math.log(1024)), units.length - 1)
  const amount = value / 1024 ** exponent
  return `${amount.toFixed(amount >= 10 || exponent === 0 ? 0 : 1)} ${units[exponent]}`
}

function sourceFolderName(source: SourceGameDetailDTO): string {
  const normalized = (source.root_path || source.raw_title).replaceAll('\\', '/').replace(/\/+$/, '')
  const name = normalized.split('/').filter(Boolean).at(-1) || source.raw_title
  return name.replace(/[<>:"/\\|?*]/g, ' ').replace(/\s+/g, ' ').trim() || 'Game'
}

function joinLogicalPath(root: string | undefined, child: string): string {
  return [root?.replaceAll('\\', '/').replace(/^\/+|\/+$/g, ''), child.replace(/^\/+|\/+$/g, '')]
    .filter(Boolean)
    .join('/')
}

function isActive(job: SourceMoveJob | null | undefined): boolean {
  return Boolean(job && !job.finished_at && !['failed_before_commit', 'source_cleanup_required', 'interrupted'].includes(job.status))
}

function movePhaseLabel(status: string): string {
  switch (status) {
    case 'queued': return 'Waiting to start'
    case 'materializing_source': return 'Reading the original'
    case 'staging_destination': return 'Copying and checking the new copy'
    case 'destination_committed': return 'New copy verified'
    case 'deleting_source': return 'Removing the original'
    case 'refreshing_library': return 'Refreshing your library'
    case 'completed': return 'Move complete'
    case 'failed_before_commit': return 'Original left unchanged'
    case 'source_cleanup_required': return 'New copy is safe; original needs attention'
    case 'interrupted': return 'Move interrupted'
    default: return status.replaceAll('_', ' ')
  }
}

export function SourceGameMoveDialog({
  canonicalGameId,
  canonicalTitle,
  source,
  onClose,
}: {
  canonicalGameId: string
  canonicalTitle: string
  source: SourceGameDetailDTO | null
  onClose: () => void
}) {
  const queryClient = useQueryClient()
  const [destinationId, setDestinationId] = useState('')
  const [destinationPath, setDestinationPath] = useState('')
  const [preview, setPreview] = useState<SourceMovePreviewItem | null>(null)
  const [jobId, setJobId] = useState('')
  const completionHandled = useRef('')

  const destinations = useQuery({
    queryKey: ['source-move-destinations'],
    queryFn: listSourceMoveDestinations,
    enabled: Boolean(source),
  })
  const existingJobs = useQuery({
    queryKey: ['source-move-jobs'],
    queryFn: () => listSourceMoveJobs(50),
    enabled: Boolean(source),
  })
  const jobQuery = useQuery({
    queryKey: ['source-move-job', jobId],
    queryFn: () => getSourceMoveJob(jobId),
    enabled: Boolean(jobId),
    refetchInterval: (query) => isActive(query.state.data) ? 800 : false,
  })
  const job = jobQuery.data

  useEffect(() => {
    setPreview(null)
    setJobId('')
    completionHandled.current = ''
    if (!source) {
      setDestinationId('')
      setDestinationPath('')
      return
    }
    const unfinished = existingJobs.data?.find((candidate) =>
      candidate.source_game_id === source.id && !candidate.finished_at,
    )
    if (unfinished) {
      setJobId(unfinished.id)
      return
    }
    const destination = destinations.data?.find((candidate) => candidate.integration_id !== source.integration_id)
    setDestinationId(destination?.integration_id ?? '')
    setDestinationPath(joinLogicalPath(destination?.suggested_root, sourceFolderName(source)))
  }, [source?.id, destinations.data, existingJobs.data])

  useEffect(() => {
    if (!job || job.status !== 'completed' || completionHandled.current === job.id) return
    completionHandled.current = job.id
    void Promise.all([
      queryClient.invalidateQueries({ queryKey: ['game', canonicalGameId] }),
      queryClient.invalidateQueries({ queryKey: ['games'] }),
      queryClient.invalidateQueries({ queryKey: ['source-move-jobs'] }),
    ])
  }, [canonicalGameId, job, queryClient])

  const selection = useMemo<SourceMoveSelection | null>(() => {
    if (!source || !destinationId || !destinationPath.trim()) return null
    return {
      canonical_game_id: canonicalGameId,
      source_game_id: source.id,
      destination_integration_id: destinationId,
      destination_path: destinationPath.trim(),
    }
  }, [canonicalGameId, destinationId, destinationPath, source])

  const previewMove = useMutation({
    mutationFn: async () => {
      if (!selection) throw new Error('Choose a destination and folder first.')
      const response = await previewSourceMoves([selection])
      if (!response.items[0]) throw new Error('MGA did not return a move preview.')
      return response.items[0]
    },
    onSuccess: setPreview,
  })
  const startMove = useMutation({
    mutationFn: async () => {
      if (!selection || !preview?.can_move) throw new Error('Review a valid move first.')
      const response = await startSourceMoves([selection])
      if (!response.jobs[0]) throw new Error('MGA did not create a move job.')
      return response.jobs[0]
    },
    onSuccess: (started) => {
      setJobId(started.id)
      queryClient.setQueryData(['source-move-job', started.id], started)
      void queryClient.invalidateQueries({ queryKey: ['source-move-jobs'] })
    },
  })
  const jobAction = useMutation({
    mutationFn: ({ id, action }: { id: string; action: 'retry' | 'cleanup' | 'keep-both' }) =>
      runSourceMoveAction(id, action),
    onSuccess: (updated) => {
      queryClient.setQueryData(['source-move-job', updated.id], updated)
      void queryClient.invalidateQueries({ queryKey: ['source-move-job', updated.id] })
      void queryClient.invalidateQueries({ queryKey: ['source-move-jobs'] })
    },
  })

  const selectedDestination = destinations.data?.find((candidate) => candidate.integration_id === destinationId)
  const requestBusy = previewMove.isPending || startMove.isPending || jobAction.isPending
  const progress = job && job.progress_total > 0
    ? (job.progress_current / job.progress_total) * 100
    : undefined
  const error = previewMove.error ?? startMove.error ?? jobAction.error ?? jobQuery.error

  return (
    <Dialog open={source !== null} onClose={requestBusy ? () => undefined : onClose} title="Move game files" className="max-w-3xl">
      {source ? (
        <div className="space-y-4">
          <p className="text-sm leading-6 text-mga-muted">
            Move <span className="font-semibold text-mga-text">{canonicalTitle}</span> from{' '}
            <span className="font-semibold text-mga-text">{source.integration_label || source.integration_id}</span>.
            MGA copies and verifies the new files before removing the original.
          </p>

          {!jobId ? (
            <>
              <div className="grid gap-3 sm:grid-cols-2">
                <Select
                  label="Move to"
                  value={destinationId}
                  options={(destinations.data ?? [])
                    .filter((item) => item.integration_id !== source.integration_id)
                    .map((item) => ({ value: item.integration_id, label: item.label }))}
                  placeholder={destinations.isPending ? 'Loading storage…' : 'Choose storage'}
                  onChange={(event) => {
                    const nextId = event.target.value
                    const next = destinations.data?.find((item) => item.integration_id === nextId)
                    setDestinationId(nextId)
                    setDestinationPath(joinLogicalPath(next?.suggested_root, sourceFolderName(source)))
                    setPreview(null)
                  }}
                />
                <Input
                  label="New folder"
                  value={destinationPath}
                  placeholder={selectedDestination?.suggested_root ? `${selectedDestination.suggested_root}/Game` : 'Games/Game'}
                  onChange={(event) => {
                    setDestinationPath(event.target.value)
                    setPreview(null)
                  }}
                />
              </div>
              {destinations.isSuccess && (destinations.data ?? []).filter((item) => item.integration_id !== source.integration_id).length === 0 ? (
                <p className="rounded-mga border border-amber-400/25 bg-amber-400/10 p-3 text-sm text-amber-100">
                  Add another Google Drive or network storage connection before moving this copy.
                </p>
              ) : null}
              {preview ? (
                <div className="overflow-hidden rounded-mga border border-mga-border">
                  <div className="grid grid-cols-[minmax(0,1fr)_minmax(0,1fr)_auto] gap-3 bg-mga-bg px-3 py-2 text-xs font-medium text-mga-muted">
                    <span>Original</span><span>New copy</span><span>Files</span>
                  </div>
                  <div className="grid grid-cols-[minmax(0,1fr)_minmax(0,1fr)_auto] gap-3 px-3 py-3 text-sm">
                    <span className="break-all">{preview.source_root_path}</span>
                    <span className="break-all">{preview.destination_label} / {preview.destination_path}</span>
                    <span className="whitespace-nowrap">{preview.file_count} · {formatBytes(preview.total_size)}</span>
                  </div>
                  <div className="border-t border-mga-border px-3 py-3 text-xs leading-5 text-mga-muted">
                    {preview.whole_directory
                      ? 'The folder belongs only to this game copy, so MGA can move the whole folder safely.'
                      : 'This folder is shared. MGA will move only the files listed for this game copy.'}
                    {preview.warnings?.map((warning) => <p key={warning} className="mt-1 text-amber-200">{warning}</p>)}
                    {!preview.can_move ? <p className="mt-1 text-red-300">{preview.reason || 'This move is not available.'}</p> : null}
                    {preview.source_summary ? <p className="mt-1">{preview.source_summary}</p> : null}
                    {preview.files?.length ? (
                      <details className="mt-2">
                        <summary className="cursor-pointer font-medium text-mga-text/80">Show {preview.files.length} file{preview.files.length === 1 ? '' : 's'}</summary>
                        <div className="mt-2 max-h-44 space-y-1 overflow-auto rounded-mga bg-black/10 p-2 font-mono">
                          {preview.files.map((file) => (
                            <p key={`${file.ordinal}:${file.source_path}`} className="break-all">{file.source_path} → {file.relative_path}</p>
                          ))}
                        </div>
                      </details>
                    ) : null}
                  </div>
                </div>
              ) : null}
            </>
          ) : null}

          {jobId ? (
            <div className="space-y-3 rounded-mga border border-mga-border bg-mga-bg/60 p-4">
              <div className="flex items-start justify-between gap-3">
                <div>
                  <p className="font-medium text-mga-text">{movePhaseLabel(job?.status ?? 'queued')}</p>
                  <p className="mt-1 text-xs leading-5 text-mga-muted">{job?.message || 'Checking move status…'}</p>
                </div>
                {isActive(job) || jobQuery.isPending ? <Loader2 size={18} className="animate-spin text-mga-accent" /> : <ShieldCheck size={18} className="text-emerald-300" />}
              </div>
              {isActive(job) ? <ProgressBar value={progress} label={movePhaseLabel(job?.status ?? 'queued')} /> : null}
              {job?.error ? <p className="rounded-mga border border-red-500/25 bg-red-500/10 p-3 text-xs leading-5 text-red-200">{job.error}</p> : null}
              {job?.status === 'failed_before_commit' || (job?.status === 'interrupted' && !['destination_committed', 'deleting_source', 'refreshing_library'].includes(job.recovery_phase ?? '')) ? (
                <div className="flex flex-wrap gap-2">
                  <Button size="sm" disabled={jobAction.isPending} onClick={() => jobAction.mutate({ id: job.id, action: 'retry' })}>
                    <RotateCcw size={15} /> Retry
                  </Button>
                  <Button size="sm" variant="outline" disabled={jobAction.isPending} onClick={() => jobAction.mutate({ id: job.id, action: 'cleanup' })}>
                    <Trash2 size={15} /> Clean up temporary files
                  </Button>
                </div>
              ) : null}
              {job?.status === 'source_cleanup_required' || (job?.status === 'interrupted' && ['destination_committed', 'deleting_source', 'refreshing_library'].includes(job.recovery_phase ?? '')) ? (
                <div className="space-y-2">
                  <p className="text-xs leading-5 text-amber-100">
                    {job.recovery_phase === 'refreshing_library'
                      ? 'The new copy is verified, but MGA could not add it to your library yet. The original is unchanged.'
                      : 'The new copy is already verified. Retry removing the original, or keep both copies.'}
                  </p>
                  <div className="flex flex-wrap gap-2">
                    <Button size="sm" disabled={jobAction.isPending} onClick={() => jobAction.mutate({ id: job.id, action: 'retry' })}>
                      <RotateCcw size={15} /> {job.recovery_phase === 'refreshing_library' ? 'Retry library refresh' : 'Retry original cleanup'}
                    </Button>
                    <Button size="sm" variant="outline" disabled={jobAction.isPending} onClick={() => jobAction.mutate({ id: job.id, action: 'keep-both' })}>
                      Keep both
                    </Button>
                  </div>
                </div>
              ) : null}
            </div>
          ) : null}

          {error ? <p className="text-sm text-red-300">{error instanceof Error ? error.message : 'MGA could not continue the move.'}</p> : null}
          <div className="flex justify-end gap-2">
            <Button type="button" variant="outline" disabled={requestBusy} onClick={onClose}>
              {jobId ? (job?.status === 'completed' ? 'Done' : 'Close') : 'Cancel'}
            </Button>
            {!jobId && !preview ? (
              <Button type="button" disabled={!selection || previewMove.isPending} onClick={() => previewMove.mutate()}>
                {previewMove.isPending ? <Loader2 size={16} className="animate-spin" /> : <ArrowRightLeft size={16} />}
                Review move
              </Button>
            ) : null}
            {!jobId && preview ? (
              <Button type="button" disabled={!preview.can_move || startMove.isPending} onClick={() => startMove.mutate()}>
                {startMove.isPending ? <Loader2 size={16} className="animate-spin" /> : <ArrowRightLeft size={16} />}
                Move files
              </Button>
            ) : null}
          </div>
        </div>
      ) : null}
    </Dialog>
  )
}
