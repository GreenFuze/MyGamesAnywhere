import type { GameDetailResponse } from '@/api/client'
import { Info, Play } from 'lucide-react'
import { AchievementProgressRing } from '@/components/library/AchievementProgressRing'
import { GameContextMenu } from '@/components/library/GameContextMenu'
import { BrandIcon } from '@/components/ui/brand-icon'
import { CoverImage } from '@/components/ui/cover-image'
import { PlatformIcon } from '@/components/ui/platform-icon'
import { StatusBadge } from '@/components/ui/status-badge'
import { useTheme } from '@/theme/ThemeProvider'
import { useEffect, useMemo, useRef, useState, type MouseEvent, type ReactNode } from 'react'
import { createPortal } from 'react-dom'
import { useLocation, useNavigate } from 'react-router-dom'
import { useGameFavoriteAction } from '@/hooks/useGameFavorite'
import {
  isPlayable,
  preferredSecondaryText,
  selectCoverUrl,
  selectPreviewImageUrl,
  selectSourceIntegrations,
} from '@/lib/gameUtils'
import { buildGameRouteState } from '@/lib/gameNavigation'
import { cn } from '@/lib/utils'

interface GameCardProps {
  game: GameDetailResponse
  hoverAction?: ReactNode
  variant?: 'library' | 'play'
}

type OverlayAlignment = 'left' | 'center' | 'right'

interface OverlayLayout {
  width: number
  top: number
  left: number
  trayHeight: number
  alignment: OverlayAlignment
}

const HOVER_CLOSE_DELAY_MS = 120
const HOVER_VIEWPORT_MARGIN_PX = 16
const OVERLAY_EXIT_DURATION_MS = 210

interface IconBadgeProps {
  label: string
  children: ReactNode
}

function IconBadge({ label, children }: IconBadgeProps) {
  return (
    <span
      title={label}
      aria-label={label}
      role="img"
      className="inline-flex h-6 w-6 items-center justify-center rounded-full border border-white/12 bg-black/62 text-white backdrop-blur"
    >
      {children}
    </span>
  )
}

interface CardActionButtonProps {
  label: string
  onClick: (event: MouseEvent<HTMLButtonElement>) => void
  icon: ReactNode
  variant?: 'primary' | 'secondary'
}

function CardActionButton({ label, onClick, icon, variant = 'secondary' }: CardActionButtonProps) {
  return (
    <button
      type="button"
      onClick={onClick}
      className={cn(
        'pointer-events-auto inline-flex h-10 min-w-[9.75rem] flex-1 items-center justify-center gap-2 rounded-full px-4 text-sm font-medium transition-colors sm:flex-none',
        variant === 'primary'
          ? 'bg-white text-black hover:bg-white/90'
          : 'border border-white/14 bg-white/8 text-white hover:bg-white/14',
      )}
      aria-label={label}
      title={label}
    >
      {icon}
      <span className="truncate">{label}</span>
    </button>
  )
}

interface FavoriteToggleButtonProps {
  favorite: boolean
  busy: boolean
  onClick: (event: MouseEvent<HTMLButtonElement>) => void
}

function FavoriteToggleButton({ favorite, busy, onClick }: FavoriteToggleButtonProps) {
  return (
    <button
      type="button"
      onClick={onClick}
      disabled={busy}
      className={cn(
        'pointer-events-auto inline-flex h-10 w-10 items-center justify-center rounded-[14px] border text-lg shadow-[0_8px_24px_rgba(0,0,0,0.28)] backdrop-blur transition-colors disabled:cursor-wait disabled:opacity-70',
        favorite
          ? 'border-rose-300/60 bg-rose-500/18 text-rose-100 hover:bg-rose-500/24'
          : 'border-white/12 bg-black/52 text-white hover:bg-white/12',
      )}
      aria-label={favorite ? 'Remove from favorites' : 'Add to favorites'}
      title={favorite ? 'Remove from favorites' : 'Add to favorites'}
    >
      <span aria-hidden="true" className={cn('leading-none', favorite ? 'text-rose-400' : '')}>
        {favorite ? '♥' : '♡'}
      </span>
    </button>
  )
}

export function GameCard({ game, hoverAction, variant = 'library' }: GameCardProps) {
  const navigate = useNavigate()
  const location = useLocation()
  const { reducedMotion } = useTheme()
  const cardRef = useRef<HTMLElement | null>(null)
  const closeTimerRef = useRef<number | null>(null)
  const unmountTimerRef = useRef<number | null>(null)
  const enterRafRef = useRef<number | null>(null)
  const enterRaf2Ref = useRef<number | null>(null)
  const [contextMenuPoint, setContextMenuPoint] = useState<{ x: number; y: number } | null>(null)
  const [hoverActive, setHoverActive] = useState(false)
  const [overlayVisible, setOverlayVisible] = useState(false)
  const [desktopHoverEnabled, setDesktopHoverEnabled] = useState(false)
  const [overlayLayout, setOverlayLayout] = useState<OverlayLayout | null>(null)
  const { setFavorite, isPendingFor } = useGameFavoriteAction()
  const coverUrl = selectCoverUrl(game.media, game.cover_override)
  const previewUrl = selectPreviewImageUrl(game.media, game.cover_override, game.hover_override)
  const overlayMediaUrl = previewUrl ?? coverUrl
  const previewUsesLandscapeMedia = previewUrl !== null && previewUrl !== coverUrl
  const playable = isPlayable(game)
  const sourceIntegrations = selectSourceIntegrations(game)
  const secondaryText = preferredSecondaryText(game) ?? 'Unknown source'
  const canOpenStream = !playable && typeof game.xcloud_url === 'string' && game.xcloud_url.length > 0
  const primaryActionLabel = playable ? 'Play' : canOpenStream ? 'Open' : 'Details'
  const isPlayVariant = variant === 'play'
  const favoriteBusy = isPendingFor(game.id)

  const routeState = useMemo(
    () => buildGameRouteState(location.pathname, location.search),
    [location.pathname, location.search],
  )

  useEffect(() => {
    if (typeof window === 'undefined') return undefined
    const mediaQuery = window.matchMedia('(min-width: 640px)')
    const update = () => setDesktopHoverEnabled(mediaQuery.matches)

    update()
    if (typeof mediaQuery.addEventListener === 'function') {
      mediaQuery.addEventListener('change', update)
      return () => mediaQuery.removeEventListener('change', update)
    }

    mediaQuery.addListener(update)
    return () => mediaQuery.removeListener(update)
  }, [])

  useEffect(() => {
    return () => {
      if (closeTimerRef.current !== null) {
        window.clearTimeout(closeTimerRef.current)
      }
      if (unmountTimerRef.current !== null) {
        window.clearTimeout(unmountTimerRef.current)
      }
      if (enterRafRef.current !== null) {
        window.cancelAnimationFrame(enterRafRef.current)
      }
      if (enterRaf2Ref.current !== null) {
        window.cancelAnimationFrame(enterRaf2Ref.current)
      }
    }
  }, [])

  const clearCloseTimer = () => {
    if (closeTimerRef.current !== null) {
      window.clearTimeout(closeTimerRef.current)
      closeTimerRef.current = null
    }
  }

  const clearUnmountTimer = () => {
    if (unmountTimerRef.current !== null) {
      window.clearTimeout(unmountTimerRef.current)
      unmountTimerRef.current = null
    }
  }

  const clearEnterRafs = () => {
    if (enterRafRef.current !== null) {
      window.cancelAnimationFrame(enterRafRef.current)
      enterRafRef.current = null
    }
    if (enterRaf2Ref.current !== null) {
      window.cancelAnimationFrame(enterRaf2Ref.current)
      enterRaf2Ref.current = null
    }
  }

  const updateOverlayLayout = () => {
    if (!desktopHoverEnabled || typeof window === 'undefined') return
    const rect = cardRef.current?.getBoundingClientRect()
    if (!rect) return

    const overlayWidth = Math.max(336, Math.min(440, rect.width * (isPlayVariant ? 2.04 : 1.92)))
    const trayHeight = isPlayVariant ? 136 : 128
    const mediaHeight = overlayWidth * 9 / 16
    const totalHeight = mediaHeight + trayHeight
    const centeredLeft = rect.left + rect.width / 2 - overlayWidth / 2
    const minLeft = HOVER_VIEWPORT_MARGIN_PX
    const maxLeft = Math.max(
      HOVER_VIEWPORT_MARGIN_PX,
      window.innerWidth - overlayWidth - HOVER_VIEWPORT_MARGIN_PX,
    )
    const left = Math.min(maxLeft, Math.max(minLeft, centeredLeft))
    const top = Math.max(HOVER_VIEWPORT_MARGIN_PX, rect.bottom - totalHeight)
    const horizontalShift = centeredLeft - left
    let alignment: OverlayAlignment = 'center'
    if (horizontalShift > 24) alignment = 'left'
    if (horizontalShift < -24) alignment = 'right'

    setOverlayLayout({
      width: overlayWidth,
      top,
      left,
      trayHeight,
      alignment,
    })
  }

  const openHover = () => {
    if (!desktopHoverEnabled) return
    clearCloseTimer()
    clearUnmountTimer()
    updateOverlayLayout()
    setHoverActive(true)
  }

  const scheduleHoverClose = () => {
    clearCloseTimer()
    closeTimerRef.current = window.setTimeout(() => {
      if (reducedMotion) {
        setOverlayVisible(false)
        setHoverActive(false)
        return
      }
      setOverlayVisible(false)
      clearUnmountTimer()
      unmountTimerRef.current = window.setTimeout(() => {
        setHoverActive(false)
      }, OVERLAY_EXIT_DURATION_MS)
    }, HOVER_CLOSE_DELAY_MS)
  }

  useEffect(() => {
    if (!hoverActive || !desktopHoverEnabled || typeof window === 'undefined') return undefined

    const handleViewportChange = () => updateOverlayLayout()
    window.addEventListener('resize', handleViewportChange)
    window.addEventListener('scroll', handleViewportChange, true)
    return () => {
      window.removeEventListener('resize', handleViewportChange)
      window.removeEventListener('scroll', handleViewportChange, true)
    }
  }, [hoverActive, desktopHoverEnabled])

  useEffect(() => {
    if (!hoverActive) return undefined
    clearEnterRafs()
    clearUnmountTimer()

    if (reducedMotion) {
      setOverlayVisible(true)
      return undefined
    }

    setOverlayVisible(false)
    enterRafRef.current = window.requestAnimationFrame(() => {
      enterRaf2Ref.current = window.requestAnimationFrame(() => {
        setOverlayVisible(true)
      })
    })

    return () => clearEnterRafs()
  }, [hoverActive, reducedMotion])

  const openGame = () => {
    navigate(`/game/${encodeURIComponent(game.id)}`, { state: routeState })
  }

  const launchPrimaryAction = () => {
    if (playable) {
      navigate(`/game/${encodeURIComponent(game.id)}/play`, { state: routeState })
      return
    }
    if (canOpenStream && game.xcloud_url) {
      window.open(game.xcloud_url, '_blank', 'noopener,noreferrer')
      return
    }
    openGame()
  }

  const stopCardClick = (event: MouseEvent<HTMLElement>) => {
    event.preventDefault()
    event.stopPropagation()
  }

  const renderContextMenu = (event: MouseEvent<HTMLElement>) => {
    event.preventDefault()
    event.stopPropagation()
    setContextMenuPoint({
      x: event.clientX,
      y: event.clientY,
    })
  }

  const toggleFavorite = async (event: MouseEvent<HTMLButtonElement>) => {
    stopCardClick(event)
    await setFavorite({
      gameId: game.id,
      favorite: !game.favorite,
    })
  }

  const badgeRow = (
    <>
      {game.xcloud_available && <StatusBadge kind="xcloud" />}
      {game.is_game_pass && <StatusBadge kind="gamepass" />}
      {playable && <StatusBadge kind="playable" />}
      <IconBadge label={game.platform}>
        <PlatformIcon platform={game.platform} showLabel={false} className="text-white" />
      </IconBadge>
      {sourceIntegrations.map((source) => (
        <IconBadge key={source.key} label={source.label}>
          <BrandIcon brand={source.pluginId} className="h-3.5 w-3.5" />
        </IconBadge>
      ))}
    </>
  )

  const overlay = hoverActive && overlayLayout && typeof document !== 'undefined'
    ? createPortal(
        <div
          className="pointer-events-none fixed inset-0 z-[90] hidden sm:block"
          aria-hidden="true"
        >
          <div
            className={cn(
              'absolute overflow-hidden rounded-[18px] border border-white/[0.05] bg-[#101216] text-white shadow-[0_42px_110px_rgba(0,0,0,0.68)] will-change-transform transition-[transform,opacity,filter] duration-[210ms] ease-[cubic-bezier(0.2,0.8,0.2,1)]',
              isPlayVariant && 'border-white/[0.04] bg-[#09070d] shadow-[0_46px_120px_rgba(0,0,0,0.74)]',
              overlayVisible
                ? 'pointer-events-auto opacity-100 translate-y-0 scale-100'
                : 'pointer-events-none opacity-0 translate-y-5 scale-[0.9]',
            )}
            style={{
              width: `${overlayLayout.width}px`,
              top: `${overlayLayout.top}px`,
              left: `${overlayLayout.left}px`,
            }}
            onMouseEnter={openHover}
            onMouseLeave={scheduleHoverClose}
            onFocusCapture={openHover}
            onBlurCapture={(event) => {
              const nextTarget = event.relatedTarget as Node | null
              if (nextTarget && event.currentTarget.contains(nextTarget)) return
              scheduleHoverClose()
            }}
            onContextMenu={renderContextMenu}
          >
            <div className="relative aspect-video overflow-hidden bg-black">
              {overlayMediaUrl ? (
                <>
                  <img
                    src={overlayMediaUrl}
                    alt=""
                    aria-hidden="true"
                    loading="lazy"
                    decoding="async"
                    className={cn(
                      'absolute inset-0 h-full w-full object-cover opacity-36 blur-xl transition-transform duration-300 ease-out',
                      overlayVisible ? 'scale-110' : 'scale-[1.04]',
                    )}
                  />
                  <img
                    src={overlayMediaUrl}
                    alt={game.title}
                    loading="lazy"
                    decoding="async"
                    className={cn(
                      'relative z-[1] h-full w-full transition-[transform,opacity] duration-300 ease-out',
                      overlayVisible ? 'scale-100' : 'scale-[0.985]',
                      previewUsesLandscapeMedia ? 'object-cover' : 'object-contain',
                    )}
                  />
                </>
              ) : (
                <div className="flex h-full items-center justify-center text-sm text-white/45">No preview</div>
              )}

              <button
                type="button"
                onClick={(event) => {
                  stopCardClick(event)
                  openGame()
                }}
                className="absolute inset-0 z-[2] cursor-pointer"
                aria-label={`Open details for ${game.title}`}
                title="View details"
              />
              <div className="pointer-events-none absolute inset-0 z-[2] bg-gradient-to-t from-black/86 via-black/12 to-black/8" />
              <div
                className={cn(
                  'absolute inset-x-0 top-0 z-[3] flex items-start justify-between gap-3 p-3 transition-all duration-200 ease-out',
                  overlayVisible ? 'opacity-100 translate-y-0 delay-[90ms]' : 'opacity-0 -translate-y-3',
                )}
              >
                <div className="flex max-w-[calc(100%-3rem)] flex-wrap gap-1.5">{badgeRow}</div>
                <div className="pointer-events-auto flex shrink-0 items-center gap-2">
                  {hoverAction}
                  <FavoriteToggleButton favorite={game.favorite} busy={favoriteBusy} onClick={(event) => void toggleFavorite(event)} />
                </div>
              </div>
            </div>

            <div
              className={cn(
                'space-y-3 px-4 pb-4 pt-3 transition-all duration-200 ease-out',
                overlayVisible ? 'opacity-100 translate-y-0 delay-[110ms]' : 'opacity-0 translate-y-4',
              )}
              style={{ minHeight: `${overlayLayout.trayHeight}px` }}
            >
              <div className="flex items-center gap-2">
                <CardActionButton
                  label={primaryActionLabel}
                  variant="primary"
                  icon={
                    playable || canOpenStream ? (
                      <Play size={16} fill="currentColor" />
                    ) : (
                      <Info size={16} />
                    )
                  }
                  onClick={(event) => {
                    stopCardClick(event)
                    launchPrimaryAction()
                  }}
                />
                {primaryActionLabel !== 'Details' && (
                  <CardActionButton
                    label="Details"
                    icon={<Info size={16} />}
                    onClick={(event) => {
                      stopCardClick(event)
                      openGame()
                    }}
                  />
                )}
              </div>

              <div className="space-y-1">
                <p className="line-clamp-2 text-[15px] font-semibold leading-tight text-white">
                  {game.title || '\u2014'}
                </p>
                <p className="line-clamp-1 text-xs text-white/68">{secondaryText}</p>
              </div>

              {game.achievement_summary && (
                <div className="flex items-center justify-between pt-1">
                  <span className="text-[11px] uppercase tracking-[0.18em] text-white/42">Progress</span>
                  <AchievementProgressRing
                    summary={game.achievement_summary}
                    size={36}
                    strokeWidth={4}
                    showLabel={false}
                    className="text-white"
                  />
                </div>
              )}
            </div>
          </div>
        </div>,
        document.body,
      )
    : null

  return (
    <>
      <article
        ref={cardRef}
        role="button"
        tabIndex={0}
        onClick={openGame}
        onMouseEnter={openHover}
        onMouseLeave={scheduleHoverClose}
        onFocusCapture={openHover}
        onBlurCapture={(event) => {
          const nextTarget = event.relatedTarget as Node | null
          if (nextTarget && event.currentTarget.contains(nextTarget)) return
          scheduleHoverClose()
        }}
        onContextMenu={renderContextMenu}
        onKeyDown={(event) => {
          if (event.key === 'Enter' || event.key === ' ') {
            event.preventDefault()
            openGame()
          }
        }}
        className={cn(
          'relative z-0 flex h-full cursor-pointer flex-col overflow-visible rounded-[16px] bg-transparent transition-transform duration-150 ease-out focus:outline-none focus-visible:ring-2 focus-visible:ring-mga-accent',
          hoverActive && 'z-30',
        )}
      >
        <div
          className={cn(
            'relative overflow-hidden rounded-[18px] border border-white/[0.04] bg-[#0d0f13] shadow-[0_14px_34px_rgba(0,0,0,0.22)] transition-shadow duration-150 ease-out',
            isPlayVariant &&
              'border-white/[0.03] bg-[#09070d] shadow-[0_14px_38px_rgba(0,0,0,0.28)]',
            hoverActive && 'shadow-[0_24px_48px_rgba(0,0,0,0.32)]',
          )}
        >
          <div className="relative">
            <CoverImage
              src={coverUrl}
              alt={game.title}
              fit="contain"
              variant="card"
              className="aspect-[3/4] w-full"
            />
            <div className="pointer-events-none absolute inset-0 z-[1] bg-gradient-to-t from-black/96 via-black/28 to-black/10" />
            <div className="pointer-events-none absolute inset-x-0 bottom-0 z-[3] p-3 text-white">
              <div className="space-y-1.5">
                <p className="line-clamp-2 text-[17px] font-semibold leading-tight text-white drop-shadow-[0_1px_8px_rgba(0,0,0,0.35)]">
                  {game.title || '\u2014'}
                </p>
                <p className="line-clamp-1 text-[13px] text-white/74">{secondaryText}</p>
              </div>
            </div>
          </div>
        </div>
      </article>
      {overlay}
      <GameContextMenu game={game} point={contextMenuPoint} onClose={() => setContextMenuPoint(null)} />
    </>
  )
}
