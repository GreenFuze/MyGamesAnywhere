import type { GameDetailResponse } from '@/api/client'
import { AchievementProgressRing } from '@/components/library/AchievementProgressRing'
import { BrandBadge } from '@/components/ui/brand-icon'
import { Badge } from '@/components/ui/badge'
import { CoverImage } from '@/components/ui/cover-image'
import { PlatformIcon } from '@/components/ui/platform-icon'
import { useLocation, useNavigate } from 'react-router-dom'
import {
  formatHLTB,
  isPlayable,
  preferredSecondaryText,
  primarySourcePlugin,
  selectCoverUrl,
  sourceMatchCount,
  sourceLabel,
} from '@/lib/gameUtils'
import { buildGameRouteState } from '@/lib/gameNavigation'

interface GameCardProps {
  game: GameDetailResponse
}

export function GameCard({ game }: GameCardProps) {
  const navigate = useNavigate()
  const location = useLocation()
  const coverUrl = selectCoverUrl(game.media)
  const playable = isPlayable(game)
  const primarySource = primarySourcePlugin(game)
  const hltb = formatHLTB(game.completion_time)
  const matchCount = sourceMatchCount(game)
  const secondaryText = preferredSecondaryText(game) ?? 'Unknown source'

  const openGame = () => {
    navigate(`/game/${encodeURIComponent(game.id)}`, {
      state: buildGameRouteState(location.pathname, location.search),
    })
  }

  return (
    <article
      role="button"
      tabIndex={0}
      onClick={openGame}
      onKeyDown={(event) => {
        if (event.key === 'Enter' || event.key === ' ') {
          event.preventDefault()
          openGame()
        }
      }}
      className="group relative flex h-full cursor-pointer flex-col overflow-hidden rounded-mga border border-mga-border bg-mga-surface transition-all hover:-translate-y-0.5 hover:border-mga-accent/60 hover:shadow-lg hover:shadow-black/20 focus:outline-none focus-visible:ring-2 focus-visible:ring-mga-accent"
    >
      <div className="relative">
        <CoverImage
          src={coverUrl}
          alt={game.title}
          fit="contain"
          variant="card"
          className="aspect-[2/3] w-full"
        />

        <div className="absolute left-2 top-2 flex max-w-[calc(100%-1rem)] flex-wrap gap-1">
          {game.xcloud_available && (
            <Badge variant="xcloud" className="bg-black/70 text-[10px] uppercase tracking-wide text-white backdrop-blur">
              xCloud
            </Badge>
          )}
          {game.is_game_pass && (
            <Badge variant="gamepass" className="bg-black/70 text-[10px] uppercase tracking-wide text-white backdrop-blur">
              Game Pass
            </Badge>
          )}
          {playable && (
            <Badge variant="playable" className="bg-black/70 text-[10px] uppercase tracking-wide text-white backdrop-blur">
              Playable
            </Badge>
          )}
        </div>

        {game.achievement_summary && (
          <div className="absolute bottom-2 right-2 rounded-full border border-white/10 bg-black/70 p-1.5 backdrop-blur">
            <AchievementProgressRing
              summary={game.achievement_summary}
              size={42}
              strokeWidth={4}
              showLabel={false}
              className="text-white"
            />
          </div>
        )}

        <div className="pointer-events-none absolute inset-0 bg-gradient-to-t from-black/85 via-black/35 to-transparent opacity-0 transition-opacity duration-200 group-hover:opacity-100" />
        <div className="pointer-events-none absolute inset-x-0 bottom-0 hidden translate-y-2 p-2 opacity-0 transition-all duration-200 group-hover:translate-y-0 group-hover:opacity-100 sm:block">
          <div className="space-y-2 rounded-mga border border-white/10 bg-black/55 p-2 text-white shadow-lg shadow-black/30 backdrop-blur">
            <div className="flex flex-wrap items-center gap-1.5">
              <Badge variant="platform" className="border-white/15 bg-white/10 text-white">
                <PlatformIcon platform={game.platform} showLabel />
              </Badge>
              {primarySource && (
                <BrandBadge
                  brand={primarySource}
                  label={sourceLabel(primarySource)}
                  className="border-white/15 bg-white/10 text-white"
                />
              )}
            </div>
            {(hltb || matchCount > 0) && (
              <div className="flex flex-wrap gap-1.5 text-[11px]">
                {hltb && (
                  <Badge variant="muted" className="bg-white/10 text-white">
                    {hltb}
                  </Badge>
                )}
                {matchCount > 0 && (
                  <Badge variant="muted" className="bg-white/10 text-white">
                    {matchCount} {matchCount === 1 ? 'source' : 'sources'}
                  </Badge>
                )}
              </div>
            )}
          </div>
        </div>
      </div>

      <div className="flex min-h-[4.75rem] flex-1 flex-col justify-center gap-1.5 p-3">
        <p className="line-clamp-2 text-sm font-semibold leading-tight text-mga-text">
          {game.title || '\u2014'}
        </p>
        <p className="line-clamp-1 text-sm text-mga-muted">{secondaryText}</p>
        {game.achievement_summary && (
          <AchievementProgressRing
            summary={game.achievement_summary}
            size={34}
            strokeWidth={4}
            className="mt-1 md:hidden"
          />
        )}
      </div>
    </article>
  )
}
