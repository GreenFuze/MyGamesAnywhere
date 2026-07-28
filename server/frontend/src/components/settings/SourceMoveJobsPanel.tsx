import { Link } from 'react-router-dom'
import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query'
import { ArrowRightLeft, Loader2, RotateCcw, Trash2 } from 'lucide-react'
import { listSourceMoveJobs, runSourceMoveAction, type SourceMoveJob } from '@/api/client'
import { Button } from '@/components/ui/button'
import { ProgressBar } from '@/components/ui/progress-bar'

function statusLabel(status: string): string {
  switch (status) {
    case 'queued': return 'Waiting to start'
    case 'materializing_source': return 'Reading the original'
    case 'staging_destination': return 'Copying and checking'
    case 'destination_committed': return 'New copy verified'
    case 'deleting_source': return 'Removing the original'
    case 'refreshing_library': return 'Refreshing the library'
    case 'completed': return 'Completed'
    case 'failed_before_commit': return 'Original unchanged'
    case 'source_cleanup_required': return 'Needs attention'
    case 'interrupted': return 'Interrupted'
    default: return status.replaceAll('_', ' ')
  }
}

function interruptedAfterCommit(job: SourceMoveJob): boolean {
  return ['destination_committed', 'deleting_source', 'refreshing_library'].includes(job.recovery_phase ?? '')
}

function active(job: SourceMoveJob): boolean {
  return !job.finished_at && !['failed_before_commit', 'source_cleanup_required', 'interrupted'].includes(job.status)
}

function needsAttention(job: SourceMoveJob): boolean {
  return !job.finished_at && ['failed_before_commit', 'source_cleanup_required', 'interrupted'].includes(job.status)
}

export function SourceMoveJobsPanel() {
  const queryClient = useQueryClient()
  const jobs = useQuery({
    queryKey: ['source-move-jobs'],
    queryFn: () => listSourceMoveJobs(50),
    refetchInterval: (query) => (query.state.data ?? []).some(active) ? 1200 : false,
  })
  const action = useMutation({
    mutationFn: ({ id, name }: { id: string; name: 'retry' | 'cleanup' | 'keep-both' }) =>
      runSourceMoveAction(id, name),
    onSuccess: (job) => {
      queryClient.setQueryData(['source-move-job', job.id], job)
      void queryClient.invalidateQueries({ queryKey: ['source-move-jobs'] })
      void queryClient.invalidateQueries({ queryKey: ['games'] })
    },
  })
  const ordered = [...(jobs.data ?? [])].sort((left, right) => {
    const priority = (job: SourceMoveJob) => needsAttention(job) ? 0 : active(job) ? 1 : 2
    return priority(left) - priority(right) || new Date(right.updated_at).getTime() - new Date(left.updated_at).getTime()
  })

  return (
    <section className="rounded-mga border border-mga-border bg-mga-surface p-5 shadow-lg">
      <div className="flex items-start gap-3">
        <div className="grid h-10 w-10 shrink-0 place-items-center rounded-mga bg-mga-accent/10 text-mga-accent">
          <ArrowRightLeft className="h-5 w-5" />
        </div>
        <div>
          <h2 className="text-lg font-black text-mga-text">Game file moves</h2>
          <p className="mt-1 max-w-2xl text-sm leading-6 text-mga-muted">
            Moves stay here if you leave the page, restart MGA, or need to retry a safe cleanup.
          </p>
        </div>
      </div>

      {jobs.isPending ? (
        <p className="mt-5 flex items-center gap-2 text-sm text-mga-muted"><Loader2 size={16} className="animate-spin" /> Loading moves…</p>
      ) : ordered.length === 0 ? (
        <p className="mt-5 rounded-mga border border-mga-border bg-mga-bg px-4 py-3 text-sm text-mga-muted">No game file moves yet.</p>
      ) : (
        <div className="mt-5 space-y-3">
          {ordered.map((job) => {
            const postCommitAttention = job.status === 'source_cleanup_required' || (job.status === 'interrupted' && interruptedAfterCommit(job))
            const preCommitAttention = job.status === 'failed_before_commit' || (job.status === 'interrupted' && !interruptedAfterCommit(job))
            const progress = job.progress_total > 0 ? (job.progress_current / job.progress_total) * 100 : undefined
            return (
              <details key={job.id} open={needsAttention(job) || active(job)} className="rounded-mga border border-mga-border bg-mga-bg px-4 py-3">
                <summary className="cursor-pointer list-none">
                  <div className="flex items-start justify-between gap-3">
                    <div className="min-w-0">
                      <p className="truncate text-sm font-semibold text-mga-text">{job.canonical_title}</p>
                      <p className="mt-1 text-xs text-mga-muted">{job.destination_label} · {statusLabel(job.status)}</p>
                    </div>
                    {active(job) ? <Loader2 size={16} className="shrink-0 animate-spin text-mga-accent" /> : (
                      <span className={needsAttention(job) ? 'text-xs font-medium text-amber-200' : 'text-xs text-mga-muted'}>
                        {needsAttention(job) ? 'Action needed' : 'View'}
                      </span>
                    )}
                  </div>
                </summary>
                <div className="mt-4 space-y-3 border-t border-mga-border pt-3">
                  <div className="grid gap-2 text-xs text-mga-muted sm:grid-cols-2">
                    <p><span className="text-mga-text/60">From</span><br /><span className="break-all">{job.source_root_path}</span></p>
                    <p><span className="text-mga-text/60">To</span><br /><span className="break-all">{job.destination_label} / {job.destination_path}</span></p>
                  </div>
                  {active(job) ? <ProgressBar value={progress} label={statusLabel(job.status)} /> : null}
                  <p className="text-xs leading-5 text-mga-muted">{job.message}</p>
                  {job.error ? <p className="rounded-mga border border-red-500/25 bg-red-500/10 p-3 text-xs leading-5 text-red-200">{job.error}</p> : null}
                  <div className="flex flex-wrap gap-2">
                    <Link to={`/games/${encodeURIComponent(job.canonical_game_id)}`}>
                      <Button size="sm" variant="outline">View game</Button>
                    </Link>
                    {preCommitAttention && !job.finished_at ? (
                      <>
                        <Button size="sm" disabled={action.isPending} onClick={() => action.mutate({ id: job.id, name: 'retry' })}>
                          <RotateCcw size={14} /> Retry
                        </Button>
                        <Button size="sm" variant="outline" disabled={action.isPending} onClick={() => action.mutate({ id: job.id, name: 'cleanup' })}>
                          <Trash2 size={14} /> Clean up temporary files
                        </Button>
                      </>
                    ) : null}
                    {postCommitAttention ? (
                      <>
                        <Button size="sm" disabled={action.isPending} onClick={() => action.mutate({ id: job.id, name: 'retry' })}>
                          <RotateCcw size={14} /> {job.recovery_phase === 'refreshing_library' ? 'Retry library refresh' : 'Retry original cleanup'}
                        </Button>
                        <Button size="sm" variant="outline" disabled={action.isPending} onClick={() => action.mutate({ id: job.id, name: 'keep-both' })}>
                          Keep both
                        </Button>
                      </>
                    ) : null}
                  </div>
                </div>
              </details>
            )
          })}
        </div>
      )}
      {jobs.error || action.error ? (
        <p className="mt-4 text-sm text-red-400">
          {(jobs.error || action.error) instanceof Error ? (jobs.error || action.error as Error).message : 'Could not load game file moves.'}
        </p>
      ) : null}
    </section>
  )
}
