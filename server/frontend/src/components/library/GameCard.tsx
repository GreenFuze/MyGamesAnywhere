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

interface GameCardProps {
  game: GameDetailResponse
}

export function GameCard({ game }: GameCardProps) {
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
    <article className="group relative flex flex-col overflow-hidden rounded-mga border border-mga-border bg-mga-surface transition-shadow hover:ring-1 hover:ring-mga-accent">
      {/* Cover art with badge overlay */}
      <div className="relative">
        <CoverImage src={coverUrl} alt={game.title} />

        {/* Status badges — top-left overlay */}
        <div className="absolute left-1.5 top-1.5 flex flex-wrap gap-1">
          {game.xcloud_available && <Badge variant="xcloud">xCloud</Badge>}
          {game.is_game_pass && <Badge variant="gamepass">GP</Badge>}
          {playable && <Badge variant="playable">Playable</Badge>}
        </div>
      </div>

      {/* Card footer */}
      <div className="flex flex-1 flex-col gap-1.5 p-2">
        {/* Title */}
        <p className="line-clamp-2 text-sm font-medium leading-tight text-mga-text">
          {game.title || '\u2014'}
        </p>

        {/* Platform + source badges */}
        <div className="flex flex-wrap items-center gap-1">
          <Badge variant="platform">
            <PlatformIcon platform={game.platform} showLabel />
          </Badge>
          {sources.map((s) => (
            <Badge key={s} variant="source">
              {pluginLabel(s)}
            </Badge>
          ))}
        </div>

        {/* Metadata row: HLTB + confidence */}
        {(hltb || matchCount > 0) && (
          <div className="flex flex-wrap gap-1">
            {hltb && <Badge>{'\u{1F550}'} {hltb}</Badge>}
            {matchCount > 0 && (
              <Badge>
                {matchCount} {matchCount === 1 ? 'source' : 'sources'}
              </Badge>
            )}
          </div>
        )}

        {/* Spacer + action button */}
        <div className="mt-auto flex justify-end pt-1">
          <Button variant="ghost" size="sm" onClick={openGame}>
            {playable ? 'Play' : 'View'}
          </Button>
        </div>
      </div>
    </article>
  )
}
