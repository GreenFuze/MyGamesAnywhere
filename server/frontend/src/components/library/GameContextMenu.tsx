import { useEffect, useMemo, useState } from 'react'
import { useQueryClient } from '@tanstack/react-query'
import { useLocation, useNavigate } from 'react-router-dom'
import {
  clearGameCoverOverride,
  setGameCoverOverride,
  type GameDetailResponse,
  type GameMediaDetailDTO,
} from '@/api/client'
import { Button } from '@/components/ui/button'
import { CoverImage } from '@/components/ui/cover-image'
import { Dialog } from '@/components/ui/dialog'
import { buildGameRouteState } from '@/lib/gameNavigation'
import { GameMediaCollection, mediaUrl } from '@/lib/gameMedia'
import { isPlayable } from '@/lib/gameUtils'

type MenuPoint = { x: number; y: number }

interface GameContextMenuProps {
  game: GameDetailResponse
  point: MenuPoint | null
  onClose: () => void
}

export function GameContextMenu({ game, point, onClose }: GameContextMenuProps) {
  const navigate = useNavigate()
  const location = useLocation()
  const queryClient = useQueryClient()
  const [coverDialogOpen, setCoverDialogOpen] = useState(false)
  const [busy, setBusy] = useState(false)
  const [error, setError] = useState<string | null>(null)

  const imageMedia = useMemo(
    () => new GameMediaCollection(game.media).imageMedia(),
    [game.media],
  )
  const playable = isPlayable(game)

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

  const invalidateGame = async () => {
    await Promise.all([
      queryClient.invalidateQueries({ queryKey: ['games'] }),
      queryClient.invalidateQueries({ queryKey: ['game', game.id] }),
    ])
  }

  const openDetails = () => {
    onClose()
    navigate(`/game/${encodeURIComponent(game.id)}`, {
      state: buildGameRouteState(location.pathname, location.search),
    })
  }

  const play = () => {
    onClose()
    if (playable) {
      navigate(`/game/${encodeURIComponent(game.id)}/play`, {
        state: buildGameRouteState(location.pathname, location.search),
      })
      return
    }
    if (game.xcloud_url) {
      window.open(game.xcloud_url, '_blank', 'noopener,noreferrer')
    }
  }

  const chooseCover = async (media: GameMediaDetailDTO) => {
    setBusy(true)
    setError(null)
    try {
      await setGameCoverOverride(game.id, media.asset_id)
      await invalidateGame()
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
      await clearGameCoverOverride(game.id)
      await invalidateGame()
      onClose()
    } catch (err) {
      setError(err instanceof Error ? err.message : String(err))
    } finally {
      setBusy(false)
    }
  }

  return (
    <>
      {point && (
        <div
          className="fixed z-50 min-w-52 rounded-mga border border-mga-border bg-mga-surface p-1 shadow-xl shadow-black/30"
          style={{ left: point.x, top: point.y }}
          onClick={(event) => event.stopPropagation()}
          onPointerDown={(event) => event.stopPropagation()}
          role="menu"
        >
          <button type="button" role="menuitem" onClick={openDetails} className="block w-full rounded-mga px-3 py-2 text-left text-sm hover:bg-mga-elevated">
            View details
          </button>
          {(playable || game.xcloud_url) && (
            <button type="button" role="menuitem" onClick={play} className="block w-full rounded-mga px-3 py-2 text-left text-sm hover:bg-mga-elevated">
              Play
            </button>
          )}
          <button type="button" role="menuitem" onClick={() => setCoverDialogOpen(true)} className="block w-full rounded-mga px-3 py-2 text-left text-sm hover:bg-mga-elevated">
            Change cover photo
          </button>
          {game.cover_override && (
            <button type="button" role="menuitem" onClick={clearCover} disabled={busy} className="block w-full rounded-mga px-3 py-2 text-left text-sm text-mga-muted hover:bg-mga-elevated hover:text-mga-text disabled:opacity-60">
              Clear cover override
            </button>
          )}
        </div>
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
    </>
  )
}
