import { useEffect, useState } from 'react'
import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query'
import { History, RotateCcw } from 'lucide-react'
import {
  getSaveDomainHistory,
  recoverSaveDomainVersion,
  setSaveDomainHistoryPolicy,
  type SaveDomainHistoryVersion,
} from '@/api/client'
import { Button } from '@/components/ui/button'
import { Dialog } from '@/components/ui/dialog'
import { useProfiles } from '@/hooks/useProfiles'
import { SaveHistoryPresenter } from '@/lib/saveHistory'

export function SaveDomainHistoryPanel({ domainId, domainLabel }: { domainId: string; domainLabel: string }) {
  const queryClient = useQueryClient()
  const { currentProfile } = useProfiles()
  const [open, setOpen] = useState(false)
  const [retainVersions, setRetainVersions] = useState(10)
  const [retainDays, setRetainDays] = useState(30)
  const [recoverTarget, setRecoverTarget] = useState<SaveDomainHistoryVersion | null>(null)
  const [notice, setNotice] = useState('')
  const queryKey = ['save-domain-history', currentProfile?.id ?? '', domainId] as const

  const history = useQuery({
    queryKey,
    queryFn: () => getSaveDomainHistory(domainId),
    enabled: open && Boolean(currentProfile?.id && domainId),
  })

  useEffect(() => {
    if (!history.data) return
    setRetainVersions(history.data.policy.retain_versions)
    setRetainDays(history.data.policy.retain_days)
  }, [history.data])

  const policy = useMutation({
    mutationFn: () => setSaveDomainHistoryPolicy(domainId, {
      retain_versions: retainVersions,
      retain_days: retainDays,
    }),
    onSuccess: (value) => {
      queryClient.setQueryData(queryKey, value)
      setNotice('Save history limit updated.')
    },
  })

  const recover = useMutation({
    mutationFn: (versionId: string) => recoverSaveDomainVersion(versionId),
    onSuccess: async () => {
      setRecoverTarget(null)
      setNotice('Past save restored. MGA kept the previous current save and is updating your backup.')
      await queryClient.invalidateQueries({ queryKey })
      await queryClient.invalidateQueries({ queryKey: ['save-sync-slots'] })
    },
  })

  const policyChanged = history.data
    ? retainVersions !== history.data.policy.retain_versions || retainDays !== history.data.policy.retain_days
    : false

  return (
    <div className="mt-3 border-t border-white/[0.06] pt-3">
      <Button type="button" size="sm" variant="ghost" onClick={() => setOpen((value) => !value)}>
        <History size={14} />
        {open ? 'Hide past saves' : 'Past saves'}
        {history.data ? ` (${history.data.versions.length})` : ''}
      </Button>

      {open ? (
        <div className="mt-3 space-y-3 rounded-xl border border-white/[0.06] bg-black/15 p-3">
          <div>
            <p className="text-xs font-semibold text-white/82">Automatic history</p>
            <p className="mt-1 text-xs leading-5 text-white/52">
              MGA keeps earlier copies before replacing this save. Device clocks do not decide which copy wins.
            </p>
          </div>

          {history.isLoading ? <p className="text-xs text-white/52">Loading past saves…</p> : null}
          {history.isError ? (
            <p className="text-xs text-red-300">
              {history.error instanceof Error ? history.error.message : 'MGA could not load past saves.'}
            </p>
          ) : null}

          {history.data ? (
            <>
              <div className="flex flex-wrap items-end gap-2">
                <label className="text-[11px] text-white/58">
                  Keep versions
                  <input
                    type="number"
                    min={1}
                    max={50}
                    value={retainVersions}
                    onChange={(event) => setRetainVersions(Number(event.target.value))}
                    className="mt-1 block w-24 rounded-lg border border-white/10 bg-black/25 px-2 py-1.5 text-xs text-white"
                  />
                </label>
                <label className="text-[11px] text-white/58">
                  Keep days
                  <input
                    type="number"
                    min={1}
                    max={365}
                    value={retainDays}
                    onChange={(event) => setRetainDays(Number(event.target.value))}
                    className="mt-1 block w-24 rounded-lg border border-white/10 bg-black/25 px-2 py-1.5 text-xs text-white"
                  />
                </label>
                <Button
                  type="button"
                  size="sm"
                  variant="outline"
                  disabled={!policyChanged || policy.isPending || retainVersions < 1 || retainVersions > 50 || retainDays < 1 || retainDays > 365}
                  onClick={() => policy.mutate()}
                >
                  {policy.isPending ? 'Saving…' : 'Save limit'}
                </Button>
              </div>
              {policy.isError ? (
                <p className="text-xs text-red-300">{policy.error instanceof Error ? policy.error.message : 'MGA could not update the save history limit.'}</p>
              ) : null}

              {history.data.versions.length === 0 ? (
                <p className="text-xs leading-5 text-white/52">No earlier copies yet. The first copy appears when MGA replaces this save.</p>
              ) : (
                <div className="space-y-2">
                  {history.data.versions.map((version) => (
                    <div key={version.id} className="rounded-lg border border-white/[0.06] bg-black/20 p-3">
                      <div className="flex items-start justify-between gap-3">
                        <div className="min-w-0">
                          <p className="truncate text-xs font-semibold text-white/82">{version.origin_label} · {version.route_label}</p>
                          <p className="mt-1 text-[11px] text-white/52">{SaveHistoryPresenter.acceptedAt(version)}</p>
                          <p className="mt-1 text-[11px] text-white/52">{SaveHistoryPresenter.fileSummary(version)}</p>
                          {SaveHistoryPresenter.reportedAt(version) ? (
                            <p className="mt-1 text-[11px] text-white/38">{SaveHistoryPresenter.reportedAt(version)}</p>
                          ) : null}
                        </div>
                        <Button type="button" size="sm" variant="outline" disabled={recover.isPending} onClick={() => setRecoverTarget(version)}>
                          <RotateCcw size={13} />
                          Restore
                        </Button>
                      </div>
                      <details className="mt-2 text-[10px] text-white/38">
                        <summary className="cursor-pointer">Technical evidence</summary>
                        <p className="mt-1 break-all">Manifest: {version.manifest_hash}</p>
                      </details>
                    </div>
                  ))}
                </div>
              )}
            </>
          ) : null}

          {notice ? <p className="text-xs text-green-300">{notice}</p> : null}
          {recover.isError ? (
            <p className="text-xs text-red-300">{recover.error instanceof Error ? recover.error.message : 'MGA could not restore that save.'}</p>
          ) : null}
        </div>
      ) : null}

      <Dialog
        open={recoverTarget !== null}
        onClose={recover.isPending ? () => undefined : () => setRecoverTarget(null)}
        title={`Restore a past ${domainLabel} save?`}
      >
        <p className="text-sm leading-6 text-mga-muted">
          MGA will keep the current save in history before restoring this copy, then update the active Save Sync connection.
        </p>
        {recoverTarget ? (
          <div className="mt-4 rounded-mga border border-mga-border bg-mga-bg p-3 text-xs text-mga-muted">
            <p className="font-semibold text-mga-text">{recoverTarget.origin_label} · {recoverTarget.route_label}</p>
            <p className="mt-1">{SaveHistoryPresenter.acceptedAt(recoverTarget)}</p>
            <p className="mt-1">{SaveHistoryPresenter.fileSummary(recoverTarget)}</p>
          </div>
        ) : null}
        <div className="mt-6 flex justify-end gap-2">
          <Button type="button" variant="ghost" disabled={recover.isPending} onClick={() => setRecoverTarget(null)}>Cancel</Button>
          <Button type="button" disabled={recover.isPending || !recoverTarget} onClick={() => recoverTarget && recover.mutate(recoverTarget.id)}>
            {recover.isPending ? 'Restoring…' : 'Restore this save'}
          </Button>
        </div>
      </Dialog>
    </div>
  )
}
