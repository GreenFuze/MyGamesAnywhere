import type { GameDetailResponse } from '@/api/client'
import { AchievementProgressRing } from '@/components/library/AchievementProgressRing'
import { GameContextMenu } from '@/components/library/GameContextMenu'
import { BrandBadge } from '@/components/ui/brand-icon'
import { Badge } from '@/components/ui/badge'
import { Button } from '@/components/ui/button'
import { CoverImage } from '@/components/ui/cover-image'
import { PlatformIcon } from '@/components/ui/platform-icon'
import { useState } from 'react'
import { useLocation, useNavigate } from 'react-router-dom'
import {
  formatHLTB,
  isPlayable,
  preferredSecondaryText,
  primarySourcePlugin,
  selectCoverUrl,
  selectSourcePlugins,
  sourceMatchCount,
  sourceLabel,
} from '@/lib/gameUtils'
import { buildGameRouteState } from '@/lib/gameNavigation'

interface GameRowProps {
  game: GameDetailResponse
}

export function GameRow({ game }: GameRowProps) {
  const navigate = useNavigate()
  const location = useLocation()
  const [contextMenuPoint, setContextMenuPoint] = useState<{ x: number; y: number } | null>(null)
  const coverUrl = selectCoverUrl(game.media, game.cover_override)
  const playable = isPlayable(game)
  const sources = selectSourcePlugins(game)
  const primarySource = primarySourcePlugin(game)
  const hltb = formatHLTB(game.completion_time)
  const matchCount = sourceMatchCount(game)
  const secondaryText = preferredSecondaryText(game)

  const openGame = () => {
    navigate(`/game/${encodeURIComponent(game.id)}`, {
      state: buildGameRouteState(location.pathname, location.search),
    })
  }

  return (
    <tr
      className="border-b border-mga-border/80 last:border-0 hover:bg-mga-elevated/40"
      onContextMenu={(event) => {
        event.preventDefault()
        event.stopPropagation()
        setContextMenuPoint({
          x: Math.min(event.clientX, window.innerWidth - 224),
          y: Math.min(event.clientY, window.innerHeight - 260),
        })
      }}
    >
      {/* Title + cover thumbnail */}
      <td className="px-3 py-2">
        <div className="flex items-center gap-3">
          <div className="h-12 w-8 shrink-0 overflow-hidden rounded-sm">
            <CoverImage
              src={coverUrl}
              alt={game.title}
              fit="contain"
              variant="compact"
              className="h-full w-full"
            />
          </div>
          <div className="min-w-0">
            <p className="line-clamp-2 text-sm font-medium text-mga-text">{game.title || '\u2014'}</p>
            {secondaryText && (
              <p className="line-clamp-1 text-xs text-mga-muted">{secondaryText}</p>
            )}
            {game.achievement_summary && (
              <AchievementProgressRing
                summary={game.achievement_summary}
                size={30}
                strokeWidth={3}
                className="mt-2"
              />
            )}
          </div>
        </div>
      </td>

      {/* Platform */}
      <td className="whitespace-nowrap px-3 py-2">
        <PlatformIcon platform={game.platform} showLabel />
      </td>

      {/* Sources */}
      <td className="px-3 py-2">
        <div className="flex flex-wrap items-center gap-1.5">
          {primarySource ? (
            <BrandBadge brand={primarySource} label={sourceLabel(primarySource)} />
          ) : (
            <Badge variant="source">Unknown</Badge>
          )}
          {sources.length > 1 && <Badge variant="muted">+{sources.length - 1}</Badge>}
        </div>
      </td>

      {/* Flags */}
      <td className="px-3 py-2">
        <div className="flex flex-wrap gap-1">
          {game.xcloud_available && <Badge variant="xcloud">xCloud</Badge>}
          {game.is_game_pass && <Badge variant="gamepass">GP</Badge>}
          {playable && <Badge variant="playable">Playable</Badge>}
        </div>
      </td>

      {/* HLTB */}
      <td className="whitespace-nowrap px-3 py-2 text-sm text-mga-muted">
        {hltb ?? '\u2014'}
      </td>

      {/* Confidence */}
      <td className="whitespace-nowrap px-3 py-2 text-sm text-mga-muted">
        {matchCount > 0 ? `${matchCount}` : '\u2014'}
      </td>

      {/* Action */}
      <td className="px-3 py-2 text-right">
        <Button variant="ghost" size="sm" onClick={openGame}>
          {playable ? 'Play' : 'View'}
        </Button>
        <GameContextMenu game={game} point={contextMenuPoint} onClose={() => setContextMenuPoint(null)} />
      </td>
    </tr>
  )
}
