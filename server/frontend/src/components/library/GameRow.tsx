import type { GameDetailResponse } from '@/api/client'
import { AchievementProgressRing } from '@/components/library/AchievementProgressRing'
import { GameContextMenu } from '@/components/library/GameContextMenu'
import { BrandBadge } from '@/components/ui/brand-icon'
import { Badge } from '@/components/ui/badge'
import { Button } from '@/components/ui/button'
import { CoverImage } from '@/components/ui/cover-image'
import { PlatformIcon } from '@/components/ui/platform-icon'
import { StatusBadge } from '@/components/ui/status-badge'
import { useState } from 'react'
import { useLocation, useNavigate } from 'react-router-dom'
import {
  formatHLTB,
  isPlayable,
  preferredSecondaryText,
  selectCoverUrl,
  selectSourceIntegrations,
  sourceMatchCount,
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
  const sourceIntegrations = selectSourceIntegrations(game)
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
          x: event.clientX,
          y: event.clientY,
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
          {sourceIntegrations.length > 0 ? (
            sourceIntegrations.map((source) => (
              <BrandBadge key={source.key} brand={source.pluginId} label={source.label} />
            ))
          ) : (
            <Badge variant="source">Unknown</Badge>
          )}
        </div>
      </td>

      {/* Flags */}
      <td className="px-3 py-2">
        <div className="flex flex-wrap gap-1">
          {game.xcloud_available && <StatusBadge kind="xcloud" className="border-sky-500/20 bg-mga-surface text-mga-text" />}
          {game.is_game_pass && <StatusBadge kind="gamepass" className="border-emerald-500/20 bg-mga-surface text-mga-text" />}
          {playable && <StatusBadge kind="playable" className="border-green-500/20 bg-mga-surface text-green-300" />}
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
