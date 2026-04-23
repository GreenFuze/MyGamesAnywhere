import type { GameDetailResponse } from '@/api/client'
import { Clock3, Layers3, PlayCircle } from 'lucide-react'
import { AchievementProgressRing } from '@/components/library/AchievementProgressRing'
import { GameContextMenu } from '@/components/library/GameContextMenu'
import { BrandBadge } from '@/components/ui/brand-icon'
import { Badge } from '@/components/ui/badge'
import { CoverImage } from '@/components/ui/cover-image'
import { PlatformIcon } from '@/components/ui/platform-icon'
import { StatusBadge } from '@/components/ui/status-badge'
import { useState, type ReactNode } from 'react'
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
  hoverAction?: ReactNode
}

export function GameCard({ game, hoverAction }: GameCardProps) {
  const navigate = useNavigate()
  const location = useLocation()
  const [contextMenuPoint, setContextMenuPoint] = useState<{ x: number; y: number } | null>(null)
  const coverUrl = selectCoverUrl(game.media, game.cover_override)
  const playable = isPlayable(game)
  const primarySource = primarySourcePlugin(game)
  const hltb = formatHLTB(game.completion_time)
  const matchCount = sourceMatchCount(game)
  const secondaryText = preferredSecondaryText(game) ?? 'Unknown source'
  const browserCapability = playable ? 'Browser Play' : null

  const openGame = () => {
    navigate(`/game/${encodeURIComponent(game.id)}`, {
      state: buildGameRouteState(location.pathname, location.search),
    })
  }

  return (
    <>
      <article
        role="button"
        tabIndex={0}
        onClick={openGame}
        onContextMenu={(event) => {
          event.preventDefault()
          event.stopPropagation()
          setContextMenuPoint({
            x: event.clientX,
            y: event.clientY,
          })
        }}
        onKeyDown={(event) => {
          if (event.key === 'Enter' || event.key === ' ') {
            event.preventDefault()
            openGame()
          }
        }}
        className="group relative flex h-full cursor-pointer flex-col overflow-hidden rounded-[18px] border border-white/10 bg-[#121019] shadow-[0_18px_46px_rgba(0,0,0,0.34)] transition-transform duration-200 ease-out will-change-transform hover:-translate-y-1 hover:shadow-[0_28px_56px_rgba(0,0,0,0.42)] focus:outline-none focus-visible:ring-2 focus-visible:ring-mga-accent"
      >
        <div className="relative">
          {hoverAction && (
            <div className="absolute right-2 top-2 z-10 opacity-0 transition-opacity duration-200 group-hover:opacity-100 group-focus-within:opacity-100">
              {hoverAction}
            </div>
          )}
          <CoverImage
            src={coverUrl}
            alt={game.title}
            fit="contain"
            variant="card"
            className="aspect-[3/4] w-full"
          />

          <div className="absolute left-2 top-2 z-[2] flex max-w-[calc(100%-1rem)] flex-wrap gap-1">
            {game.xcloud_available && <StatusBadge kind="xcloud" />}
            {game.is_game_pass && <StatusBadge kind="gamepass" />}
            {playable && <StatusBadge kind="playable" />}
          </div>

          {game.achievement_summary && (
            <div className="absolute bottom-3 right-3 z-[2] rounded-full border border-white/10 bg-black/70 p-1.5 backdrop-blur">
              <AchievementProgressRing
                summary={game.achievement_summary}
                size={42}
                strokeWidth={4}
                showLabel={false}
                className="text-white"
              />
            </div>
          )}

          <div className="pointer-events-none absolute inset-0 bg-gradient-to-t from-black/95 via-black/35 to-black/10" />
          <div className="pointer-events-none absolute inset-x-0 bottom-0 z-[1] p-3 text-white">
            <div className="hidden translate-y-2 opacity-0 transition-all duration-200 group-hover:translate-y-0 group-hover:opacity-100 group-focus-within:translate-y-0 group-focus-within:opacity-100 sm:block">
              <div className="mb-3 space-y-3 rounded-[16px] border border-white/10 bg-black/60 p-3 shadow-[0_18px_40px_rgba(0,0,0,0.34)] backdrop-blur-md">
                <div className="flex flex-wrap items-center gap-1.5">
                  <Badge variant="platform" className="border-white/10 bg-white/10 text-white">
                    <PlatformIcon platform={game.platform} showLabel />
                  </Badge>
                  {primarySource && (
                    <BrandBadge
                      brand={primarySource}
                      label={sourceLabel(primarySource)}
                      className="border-white/10 bg-white/10 text-white"
                    />
                  )}
                </div>
                <div className="grid grid-cols-2 gap-2 text-[11px] text-white/80">
                  {hltb && (
                    <div className="flex items-center gap-1.5 rounded-full bg-white/10 px-2.5 py-1.5">
                      <Clock3 size={12} className="text-white/70" />
                      <span>{hltb} main</span>
                    </div>
                  )}
                  {matchCount > 0 && (
                    <div className="flex items-center gap-1.5 rounded-full bg-white/10 px-2.5 py-1.5">
                      <Layers3 size={12} className="text-white/70" />
                      <span>
                        {matchCount} {matchCount === 1 ? 'source' : 'sources'}
                      </span>
                    </div>
                  )}
                  {browserCapability && (
                    <div className="col-span-full flex items-center gap-1.5 rounded-full bg-white/10 px-2.5 py-1.5">
                      <PlayCircle size={12} className="text-white/70" />
                      <span>{browserCapability}</span>
                    </div>
                  )}
                </div>
              </div>
            </div>

            <div className="space-y-1">
              <p className="line-clamp-2 text-[15px] font-semibold leading-tight text-white drop-shadow-[0_1px_8px_rgba(0,0,0,0.35)]">
                {game.title || '\u2014'}
              </p>
              <p className="line-clamp-1 text-xs text-white/70">{secondaryText}</p>
            </div>
          </div>
        </div>

        <div className="border-t border-white/10 bg-gradient-to-b from-[#16131f] to-[#100e16] px-3 py-2.5">
          <div className="flex items-center justify-between gap-3">
            <div className="min-w-0 flex-1">
              <div className="flex flex-wrap items-center gap-1.5 text-[11px] text-mga-muted">
                <span className="inline-flex min-w-0 items-center gap-1.5 truncate rounded-full bg-white/5 px-2 py-1 text-white/80">
                  <PlatformIcon platform={game.platform} showLabel />
                </span>
              </div>
            </div>
            {primarySource && (
              <BrandBadge
                brand={primarySource}
                label={sourceLabel(primarySource)}
                className="shrink-0 border-white/10 bg-white/5 text-white/80"
              />
            )}
          </div>
        </div>
      </article>
      <GameContextMenu game={game} point={contextMenuPoint} onClose={() => setContextMenuPoint(null)} />
    </>
  )
}
