import { useState } from 'react'
import { Link } from 'react-router-dom'
import { useQuery, useQueryClient } from '@tanstack/react-query'
import { Loader2, RefreshCw, Trash2 } from 'lucide-react'
import {
  getDuplicateGames,
  type DuplicateGameMode,
  type DuplicateGameSource,
  type SourceGameDetailDTO,
} from '@/api/client'
import { SourceGameHardDeleteDialog } from '@/components/library/SourceGameHardDeleteDialog'
import { Button } from '@/components/ui/button'
import { CoverImage } from '@/components/ui/cover-image'
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

function sourceRecordLabel(source: SourceGameDetailDTO): string {
  return `${source.integration_label || source.integration_id} · ${source.raw_title || source.external_id}`
}

function duplicateTitle(source: DuplicateGameSource): string {
  return source.game?.title || source.canonical_title || source.source.raw_title || source.source.external_id
}

function duplicateSubtitle(source: DuplicateGameSource): string {
  const platform = platformLabel(source.game?.platform || source.source.platform || 'unknown')
  const kind = source.game?.kind || source.source.kind || 'unknown'
  return `${platform} · ${kind}`
}

export function DuplicatesTab() {
  const queryClient = useQueryClient()
  const [mode, setMode] = useState<DuplicateGameMode>('loose')
  const [deleteTarget, setDeleteTarget] = useState<DuplicateGameSource | null>(null)
  const [notice, setNotice] = useState('')

  const duplicates = useQuery({
    queryKey: ['duplicate-games', mode],
    queryFn: () => getDuplicateGames(mode),
  })

  const refreshAfterDelete = async (source: DuplicateGameSource) => {
    await Promise.all([
      queryClient.invalidateQueries({ queryKey: ['duplicate-games'] }),
      queryClient.invalidateQueries({ queryKey: ['games'] }),
      queryClient.invalidateQueries({ queryKey: ['game', source.canonical_game_id] }),
      queryClient.invalidateQueries({ queryKey: ['game', source.canonical_game_id, 'achievements'] }),
      queryClient.invalidateQueries({ queryKey: ['stats'] }),
      queryClient.invalidateQueries({ queryKey: ['library-statistics'] }),
      queryClient.invalidateQueries({ queryKey: ['gamer-statistics'] }),
      queryClient.invalidateQueries({ queryKey: ['cache-entries'] }),
      queryClient.invalidateQueries({ queryKey: ['cache-jobs'] }),
    ])
  }

  const handleDeleted = async () => {
    if (!deleteTarget) return
    await refreshAfterDelete(deleteTarget)
    setNotice(`Deleted ${sourceLabel(deleteTarget)}.`)
    setDeleteTarget(null)
  }

  const groups = duplicates.data?.groups ?? []
  const loading = duplicates.isPending

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
                              onClick={() => {
                                setNotice('')
                                setDeleteTarget(source)
                              }}
                              disabled={!eligible}
                              className="border-red-500/30 text-red-200 hover:bg-red-500/10"
                            >
                              <Trash2 size={14} />
                              Remove
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

      <SourceGameHardDeleteDialog
        canonicalGameId={deleteTarget?.canonical_game_id ?? null}
        source={deleteTarget?.source ?? null}
        title="Hard Delete Duplicate Source"
        confirmLabel="Delete Source Record"
        sourceLabel={sourceRecordLabel}
        onClose={() => setDeleteTarget(null)}
        onDeleted={handleDeleted}
      />
    </div>
  )
}
