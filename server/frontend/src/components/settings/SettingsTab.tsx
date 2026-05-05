import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query'
import { Download, ExternalLink, Play, RefreshCw } from 'lucide-react'
import {
  ApiError,
  applyUpdate,
  checkForUpdates,
  downloadUpdate,
  getUpdateStatus,
} from '@/api/client'
import { Button } from '@/components/ui/button'
import { Skeleton } from '@/components/ui/skeleton'

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

export function UpdateTab() {
  const queryClient = useQueryClient()
  const updateQuery = useQuery({
    queryKey: ['update-status'],
    queryFn: getUpdateStatus,
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
    onSuccess: invalidateUpdateStatus,
  })
  const applyMutation = useMutation({
    mutationFn: applyUpdate,
    onSuccess: invalidateUpdateStatus,
  })

  const update = updateQuery.data
  const updateBusy = checkMutation.isPending || downloadMutation.isPending || applyMutation.isPending
  const updateErrors = [checkMutation.error, downloadMutation.error, applyMutation.error].filter(Boolean)

  return (
    <div className="space-y-6">
      <section className="rounded-mga border border-mga-border bg-mga-surface p-5 shadow-sm shadow-black/10 md:p-6">
        <div className="flex flex-col gap-4 lg:flex-row lg:items-start lg:justify-between">
          <div>
            <h2 className="text-lg font-semibold text-mga-text">Updates</h2>
            <p className="mt-1 max-w-3xl text-sm leading-6 text-mga-muted">
              MGA checks the release manifest, downloads the matching Windows asset, and verifies
              SHA256 before applying installer updates. Portable builds download the ZIP and show
              the verified path for manual replacement.
            </p>
          </div>
          <div className="flex flex-wrap gap-2">
            <Button
              type="button"
              variant="outline"
              onClick={() => checkMutation.mutate()}
              disabled={updateBusy}
            >
              <RefreshCw size={16} />
              Check
            </Button>
            <Button
              type="button"
              variant="outline"
              onClick={() => downloadMutation.mutate()}
              disabled={updateBusy || !update?.selected_asset}
            >
              <Download size={16} />
              Download
            </Button>
            <Button
              type="button"
              onClick={() => applyMutation.mutate()}
              disabled={updateBusy || !update?.downloaded_path}
            >
              <Play size={16} />
              Apply
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
