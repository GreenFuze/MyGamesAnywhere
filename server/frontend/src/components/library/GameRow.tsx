import type { GameDetailResponse } from '@/api/client'
import { Badge } from '@/components/ui/badge'
import { Button } from '@/components/ui/button'
import { CoverImage } from '@/components/ui/cover-image'
import { PlatformIcon } from '@/components/ui/platform-icon'
import { useLocation, useNavigate } from 'react-router-dom'
import {
  formatHLTB,
  isPlayable,
  pluginLabel,
  resolverMatchCount,
  selectCoverUrl,
  selectSourcePlugins,
} from '@/lib/gameUtils'

interface GameRowProps {
  game: GameDetailResponse
}

export function GameRow({ game }: GameRowProps) {
  const navigate = useNavigate()
  const location = useLocation()
  const coverUrl = selectCoverUrl(game.media)
  const playable = isPlayable(game.platform)
  const sources = selectSourcePlugins(game)
  const hltb = formatHLTB(game.completion_time)
  const matchCount = resolverMatchCount(game)
  const from = `${location.pathname}${location.search}`

  const openGame = () => {
    navigate(`/game/${encodeURIComponent(game.id)}`, { state: { from } })
  }

  return (
    <tr className="border-b border-mga-border/80 last:border-0 hover:bg-mga-elevated/40">
      {/* Title + cover thumbnail */}
      <td className="px-3 py-2">
        <div className="flex items-center gap-2">
          <div className="h-12 w-8 shrink-0 overflow-hidden rounded-sm">
            <CoverImage src={coverUrl} alt={game.title} className="h-full w-full" />
          </div>
          <span className="line-clamp-2 text-sm font-medium text-mga-text">
            {game.title || '\u2014'}
          </span>
        </div>
      </td>

      {/* Platform */}
      <td className="whitespace-nowrap px-3 py-2">
        <PlatformIcon platform={game.platform} showLabel />
      </td>

      {/* Sources */}
      <td className="px-3 py-2">
        <div className="flex flex-wrap gap-1">
          {sources.map((s) => (
            <Badge key={s} variant="source">
              {pluginLabel(s)}
            </Badge>
          ))}
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
      </td>
    </tr>
  )
}
