import { useEffect, useLayoutEffect, useMemo, useRef, useState } from 'react'
import { useQueryClient } from '@tanstack/react-query'
import { createPortal } from 'react-dom'
import { useLocation, useNavigate } from 'react-router-dom'
import {
  ApiError,
  clearGameCoverOverride,
  deleteSourceGame,
  refreshGameMetadata,
  setGameCoverOverride,
  type GameDetailResponse,
  type GameMediaDetailDTO,
  type SourceGameDetailDTO,
} from '@/api/client'
import { Button } from '@/components/ui/button'
import { CoverImage } from '@/components/ui/cover-image'
import { Dialog } from '@/components/ui/dialog'
import { buildGameRouteState } from '@/lib/gameNavigation'
import { GameMediaCollection, mediaUrl } from '@/lib/gameMedia'
import { isPlayable } from '@/lib/gameUtils'

type MenuPoint = { x: number; y: number }
const VIEWPORT_MARGIN = 8

interface GameContextMenuProps {
  game: GameDetailResponse
  point: MenuPoint | null
  onClose: () => void
}

function sourceRecordLabel(source: SourceGameDetailDTO): string {
  return `${source.integration_label || source.integration_id} · ${source.raw_title || source.external_id}`
}

export function GameContextMenu({ game, point, onClose }: GameContextMenuProps) {
  const navigate = useNavigate()
  const location = useLocation()
  const queryClient = useQueryClient()
  const menuRef = useRef<HTMLDivElement | null>(null)
  const [coverDialogOpen, setCoverDialogOpen] = useState(false)
  const [deleteDialogOpen, setDeleteDialogOpen] = useState(false)
  const [busy, setBusy] = useState(false)
  const [deleteBusy, setDeleteBusy] = useState(false)
  const [error, setError] = useState<string | null>(null)
  const [deleteError, setDeleteError] = useState<string | null>(null)
  const [deleteTarget, setDeleteTarget] = useState<SourceGameDetailDTO | null>(null)
  const [menuPosition, setMenuPosition] = useState<MenuPoint | null>(null)

  const imageMedia = useMemo(
    () => new GameMediaCollection(game.media).imageMedia(),
    [game.media],
  )
  const playable = isPlayable(game)
  const deletableSources = useMemo(
    () => game.source_games.filter((source) => source.hard_delete?.eligible),
    [game.source_games],
  )

  useEffect(() => {
    if (!point) return
    const onKeyDown = (event: KeyboardEvent) => {
      if (event.key === 'Escape') onClose()
    }
    const onPointerDown = () => onClose()
    window.addEventListener('keydown', onKeyDown)
    window.addEventListener('pointerdown', onPointerDown)
    return () => {
      window.removeEventListener('keydown', onKeyDown)
      window.removeEventListener('pointerdown', onPointerDown)
    }
  }, [onClose, point])

  useEffect(() => {
    if (!point) {
      setMenuPosition(null)
      return
    }
    setMenuPosition(point)
  }, [point])

  useLayoutEffect(() => {
    if (!point || !menuRef.current) return

    const rect = menuRef.current.getBoundingClientRect()
    const nextX = Math.max(
      VIEWPORT_MARGIN,
      Math.min(point.x, window.innerWidth - rect.width - VIEWPORT_MARGIN),
    )
    const nextY = Math.max(
      VIEWPORT_MARGIN,
      Math.min(point.y, window.innerHeight - rect.height - VIEWPORT_MARGIN),
    )

    if (!menuPosition || menuPosition.x !== nextX || menuPosition.y !== nextY) {
      setMenuPosition({ x: nextX, y: nextY })
    }
  }, [menuPosition, point])

  const invalidateGame = async (gameID = game.id) => {
    await Promise.all([
      queryClient.invalidateQueries({ queryKey: ['games'] }),
      queryClient.invalidateQueries({ queryKey: ['game', gameID] }),
      queryClient.invalidateQueries({ queryKey: ['game', gameID, 'achievements'] }),
    ])
  }

  const openDetails = () => {
    onClose()
    navigate(`/game/${encodeURIComponent(game.id)}`, {
      state: buildGameRouteState(location.pathname, location.search),
    })
  }

  const playInBrowser = () => {
    onClose()
    navigate(`/game/${encodeURIComponent(game.id)}/play`, {
      state: buildGameRouteState(location.pathname, location.search),
    })
  }

  const openXcloud = () => {
    onClose()
    if (!game.xcloud_url) return
    window.open(game.xcloud_url, '_blank', 'noopener,noreferrer')
  }

  const reclassify = () => {
    const params = new URLSearchParams()
    params.set('tab', 'undetected')
    const candidateId = game.source_games[0]?.id
    if (candidateId) {
      params.set('candidate_id', candidateId)
    }
    params.set('reclassify_game_id', game.id)
    params.set('reclassify_title', game.title)
    params.set('reclassify_platform', game.platform)
    const primarySource = game.source_games[0]?.plugin_id
    if (primarySource) {
      params.set('reclassify_source', primarySource)
    }
    onClose()
    navigate({ pathname: '/settings', search: params.toString() })
  }

  const triggerRefreshMetadata = async () => {
    setBusy(true)
    setError(null)
    try {
      const refreshed = await refreshGameMetadata(game.id)
      queryClient.setQueryData(['game', refreshed.id], refreshed)
      await invalidateGame(refreshed.id)
      onClose()
    } catch (err) {
      const message =
        err instanceof ApiError
          ? err.responseText?.trim() || err.message
          : (err instanceof Error ? err.message : 'Refresh failed.')
      setError(message)
    } finally {
      setBusy(false)
    }
  }

  const chooseCover = async (media: GameMediaDetailDTO) => {
    setBusy(true)
    setError(null)
    try {
      const updated = await setGameCoverOverride(game.id, media.asset_id)
      queryClient.setQueryData(['game', updated.id], updated)
      await invalidateGame(updated.id)
      setCoverDialogOpen(false)
      onClose()
    } catch (err) {
      setError(err instanceof Error ? err.message : String(err))
    } finally {
      setBusy(false)
    }
  }

  const clearCover = async () => {
    setBusy(true)
    setError(null)
    try {
      const updated = await clearGameCoverOverride(game.id)
      queryClient.setQueryData(['game', updated.id], updated)
      await invalidateGame(updated.id)
      onClose()
    } catch (err) {
      setError(err instanceof Error ? err.message : String(err))
    } finally {
      setBusy(false)
    }
  }

  const openDeleteDialog = () => {
    setDeleteError(null)
    setDeleteTarget(deletableSources.length === 1 ? deletableSources[0] : null)
    setDeleteDialogOpen(true)
    onClose()
  }

  const confirmDelete = async () => {
    if (!deleteTarget || deleteBusy) return
    setDeleteBusy(true)
    setDeleteError(null)
    try {
      const result = await deleteSourceGame(game.id, deleteTarget.id)
      await Promise.all([
        queryClient.invalidateQueries({ queryKey: ['games'] }),
        queryClient.invalidateQueries({ queryKey: ['game', game.id, 'achievements'] }),
      ])
      if (result.game) {
        queryClient.setQueryData(['game', result.game.id], result.game)
      }
      setDeleteDialogOpen(false)
      setDeleteTarget(null)
      if (result.canonical_exists && result.game) {
        if (result.game.id !== game.id) {
          navigate(`/game/${encodeURIComponent(result.game.id)}`, {
            replace: true,
            state: buildGameRouteState(location.pathname, location.search),
          })
        } else {
          await invalidateGame(result.game.id)
        }
      } else {
        navigate('/library', { replace: true })
      }
    } catch (err) {
      const message =
        err instanceof ApiError
          ? err.responseText?.trim() || err.message
          : (err instanceof Error ? err.message : 'Hard delete failed.')
      setDeleteError(message)
    } finally {
      setDeleteBusy(false)
    }
  }

  return (
    <>
      {point &&
        typeof document !== 'undefined' &&
        createPortal(
          <div
            ref={menuRef}
            className="fixed z-[200] min-w-60 rounded-mga border border-mga-border bg-mga-surface p-1 shadow-xl shadow-black/30"
            style={{ left: menuPosition?.x ?? point.x, top: menuPosition?.y ?? point.y }}
            onClick={(event) => event.stopPropagation()}
            onPointerDown={(event) => event.stopPropagation()}
            onContextMenu={(event) => event.preventDefault()}
            role="menu"
          >
            <button type="button" role="menuitem" onClick={openDetails} className="block w-full rounded-mga px-3 py-2 text-left text-sm hover:bg-mga-elevated">
              View details
            </button>
            {playable && (
              <button type="button" role="menuitem" onClick={playInBrowser} className="block w-full rounded-mga px-3 py-2 text-left text-sm hover:bg-mga-elevated">
                Play in Browser
              </button>
            )}
            {game.xcloud_url && (
              <button type="button" role="menuitem" onClick={openXcloud} className="block w-full rounded-mga px-3 py-2 text-left text-sm hover:bg-mga-elevated">
                Open xCloud
              </button>
            )}
            <button
              type="button"
              role="menuitem"
              onClick={() => {
                setCoverDialogOpen(true)
                onClose()
              }}
              className="block w-full rounded-mga px-3 py-2 text-left text-sm hover:bg-mga-elevated"
            >
              Change cover photo
            </button>
            {game.cover_override && (
              <button type="button" role="menuitem" onClick={clearCover} disabled={busy} className="block w-full rounded-mga px-3 py-2 text-left text-sm text-mga-muted hover:bg-mga-elevated hover:text-mga-text disabled:opacity-60">
                Clear cover override
              </button>
            )}
            <button type="button" role="menuitem" onClick={() => void triggerRefreshMetadata()} disabled={busy} className="block w-full rounded-mga px-3 py-2 text-left text-sm hover:bg-mga-elevated disabled:opacity-60">
              {busy ? 'Refreshing...' : 'Refresh Metadata & Media'}
            </button>
            <button type="button" role="menuitem" onClick={reclassify} className="block w-full rounded-mga px-3 py-2 text-left text-sm hover:bg-mga-elevated">
              Reclassify
            </button>
            <button
              type="button"
              role="menuitem"
              onClick={openDeleteDialog}
              disabled={deletableSources.length === 0}
              className="block w-full rounded-mga px-3 py-2 text-left text-sm text-red-200 hover:bg-red-500/10 disabled:text-mga-muted disabled:hover:bg-transparent"
            >
              Delete Source Record...
            </button>
            {error && <p className="px-3 py-2 text-xs text-red-400">{error}</p>}
          </div>,
          document.body,
        )}

      <Dialog open={coverDialogOpen} onClose={() => setCoverDialogOpen(false)} title="Change cover photo" className="max-w-3xl">
        {imageMedia.length === 0 ? (
          <p className="text-sm text-mga-muted">No image media is available for this game.</p>
        ) : (
          <div className="grid max-h-[65vh] grid-cols-2 gap-3 overflow-y-auto pr-1 sm:grid-cols-3 md:grid-cols-4">
            {imageMedia.map((media) => (
              <button
                key={`${media.asset_id}:${media.type}`}
                type="button"
                onClick={() => chooseCover(media)}
                disabled={busy}
                className="rounded-mga border border-mga-border bg-mga-bg p-2 text-left transition-colors hover:border-mga-accent disabled:opacity-60"
              >
                <CoverImage src={mediaUrl(media)} alt={media.type} className="aspect-[2/3] w-full" />
                <p className="mt-2 line-clamp-1 text-xs text-mga-muted">{media.type}</p>
              </button>
            ))}
          </div>
        )}
        {error && <p className="mt-3 text-sm text-red-400">{error}</p>}
        <div className="mt-4 flex justify-end">
          <Button type="button" variant="ghost" size="sm" onClick={() => setCoverDialogOpen(false)}>
            Close
          </Button>
        </div>
      </Dialog>

      <Dialog open={deleteDialogOpen} onClose={() => !deleteBusy && setDeleteDialogOpen(false)} title="Delete Source Record">
        <div className="space-y-4">
          <p className="text-sm text-mga-muted">
            Delete exactly one eligible source record, including its backing files, without implicitly deleting sibling source records.
          </p>

          {deletableSources.length > 1 && (
            <div className="space-y-2">
              <p className="text-xs font-medium uppercase tracking-wide text-mga-muted">Choose source record</p>
              <div className="max-h-56 space-y-2 overflow-y-auto pr-1">
                {deletableSources.map((source) => (
                  <button
                    key={source.id}
                    type="button"
                    onClick={() => setDeleteTarget(source)}
                    className={`block w-full rounded-mga border px-3 py-2 text-left text-sm transition-colors ${
                      deleteTarget?.id === source.id
                        ? 'border-red-400/70 bg-red-500/10 text-red-100'
                        : 'border-mga-border bg-mga-bg text-mga-text hover:border-mga-accent'
                    }`}
                  >
                    <span className="block font-medium">{sourceRecordLabel(source)}</span>
                    {source.root_path && <span className="mt-1 block text-xs text-mga-muted">{source.root_path}</span>}
                  </button>
                ))}
              </div>
            </div>
          )}

          {deleteTarget && (
            <div className="space-y-2 rounded-mga border border-mga-border bg-mga-bg/70 p-3">
              <p className="text-sm text-mga-text">
                This permanently deletes <span className="font-medium">{sourceRecordLabel(deleteTarget)}</span>.
              </p>
              {deleteTarget.root_path && (
                <p className="break-all text-xs text-mga-muted">Root path: {deleteTarget.root_path}</p>
              )}
            </div>
          )}

          {deleteError && <p className="text-sm text-red-400">{deleteError}</p>}

          <div className="flex justify-end gap-3">
            <Button type="button" variant="outline" onClick={() => setDeleteDialogOpen(false)} disabled={deleteBusy}>
              Cancel
            </Button>
            <Button
              type="button"
              variant="outline"
              onClick={() => void confirmDelete()}
              disabled={!deleteTarget || deleteBusy}
              className="border-red-500/30 text-red-200 hover:bg-red-500/10"
            >
              {deleteBusy ? 'Deleting...' : 'Delete Source Record'}
            </Button>
          </div>
        </div>
      </Dialog>
    </>
  )
}
