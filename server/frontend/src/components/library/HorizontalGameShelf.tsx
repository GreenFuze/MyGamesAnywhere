import { useEffect, useMemo, useRef, useState, type ReactNode } from 'react'
import { ChevronLeft, ChevronRight, Info, Play } from 'lucide-react'
import type { GameDetailResponse } from '@/api/client'
import {
  GameCard,
  type GameCardPlayRoute,
  type GameCardPrimaryAction,
} from '@/components/library/GameCard'
import { animateHorizontalScrollTo } from '@/lib/motion'
import { cn } from '@/lib/utils'
import { useTheme } from '@/theme/ThemeProvider'

const GAP_PX = 16
const MIN_CARD_WIDTH = 190
const MAX_CARD_WIDTH = 268
const INITIAL_VISIBLE_GAMES = 16
const VISIBLE_GAME_STEP = 12

interface HorizontalGameShelfProps {
  games: GameDetailResponse[]
  label: string
  renderHoverAction?: (game: GameDetailResponse) => ReactNode
  renderPrimaryAction?: (game: GameDetailResponse) => GameCardPrimaryAction | undefined
  preferredPlayRoute?: GameCardPlayRoute
  cardVariant?: 'library' | 'play'
  hasMoreGames?: boolean
  isLoadingMore?: boolean
  onLoadMore?: () => void
}

function computeCardWidth(width: number): number {
  if (width <= 0) return MIN_CARD_WIDTH
  let columns = Math.max(1, Math.floor((width + GAP_PX) / (MIN_CARD_WIDTH + GAP_PX)))
  let cardWidth = Math.floor((width - GAP_PX * (columns - 1)) / columns)
  while (columns < 12 && cardWidth > MAX_CARD_WIDTH) {
    columns++
    cardWidth = Math.floor((width - GAP_PX * (columns - 1)) / columns)
  }
  return Math.max(MIN_CARD_WIDTH, Math.min(MAX_CARD_WIDTH, cardWidth))
}

export function HorizontalGameShelf({
  games,
  label,
  renderHoverAction,
  renderPrimaryAction,
  preferredPlayRoute,
  cardVariant = 'library',
  hasMoreGames = false,
  isLoadingMore = false,
  onLoadMore,
}: HorizontalGameShelfProps) {
  const viewportRef = useRef<HTMLDivElement>(null)
  const { reducedMotion } = useTheme()
  const [canScrollLeft, setCanScrollLeft] = useState(false)
  const [canScrollRight, setCanScrollRight] = useState(false)
  const [cardWidth, setCardWidth] = useState(MIN_CARD_WIDTH)
  const [visibleGameCount, setVisibleGameCount] = useState(() => Math.min(games.length, INITIAL_VISIBLE_GAMES))

  const firstGameID = games[0]?.id
  useEffect(() => {
    setVisibleGameCount(Math.min(games.length, INITIAL_VISIBLE_GAMES))
  }, [firstGameID])

  useEffect(() => {
    setVisibleGameCount((current) => Math.min(games.length, Math.max(current, INITIAL_VISIBLE_GAMES)))
  }, [games.length])

  const updateScrollState = () => {
    const el = viewportRef.current
    if (!el) return
    setCanScrollLeft(el.scrollLeft > 4)
    setCanScrollRight(el.scrollLeft + el.clientWidth < el.scrollWidth - 4)
    setCardWidth(computeCardWidth(el.clientWidth))
    if (el.scrollWidth - el.scrollLeft - el.clientWidth < 800 && visibleGameCount < games.length) {
      setVisibleGameCount((current) => Math.min(games.length, current + VISIBLE_GAME_STEP))
    }
  }

  useEffect(() => {
    updateScrollState()
    const el = viewportRef.current
    if (!el) return

    const observer = new ResizeObserver(() => updateScrollState())
    observer.observe(el)
    el.addEventListener('scroll', updateScrollState, { passive: true })
    window.addEventListener('resize', updateScrollState)
    return () => {
      observer.disconnect()
      el.removeEventListener('scroll', updateScrollState)
      window.removeEventListener('resize', updateScrollState)
    }
  }, [games.length, visibleGameCount])

  const pageStep = useMemo(() => {
    const el = viewportRef.current
    if (!el) return 0
    return Math.max(cardWidth + GAP_PX, el.clientWidth - cardWidth / 2)
  }, [cardWidth])

  const page = (dir: 1 | -1) => {
    const el = viewportRef.current
    if (!el) return
    const maxScrollLeft = Math.max(0, el.scrollWidth - el.clientWidth)
    const targetLeft = Math.max(0, Math.min(maxScrollLeft, el.scrollLeft + dir * pageStep))
    if (reducedMotion) {
      el.scrollLeft = targetLeft
      return
    }
    animateHorizontalScrollTo(el, targetLeft)
  }

  return (
    <div className={cn('group/shelf relative', cardVariant === 'play' && 'rounded-[18px] bg-[#09070d] px-1 py-2')}>
      <div
        ref={viewportRef}
        className={cn(
          'mga-hidden-scrollbar flex snap-x snap-mandatory gap-4 overflow-x-auto pr-14',
          cardVariant === 'play' && 'pr-16',
        )}
      >
        {games.slice(0, visibleGameCount).map((game) => {
          const primaryAction = renderPrimaryAction?.(game)
          return (
            <div key={game.id} className="shrink-0 snap-start" style={{ width: `${cardWidth}px` }}>
              <GameCard
                game={game}
                hoverAction={renderHoverAction?.(game)}
                primaryAction={primaryAction}
                preferredPlayRoute={preferredPlayRoute}
                variant={cardVariant}
              />
              {primaryAction ? (
                <button
                  type="button"
                  disabled={primaryAction.disabled}
                  onClick={primaryAction.onSelect}
                  title={primaryAction.title ?? primaryAction.label}
                  aria-label={`${primaryAction.label} ${game.title}`}
                  className="mt-2 inline-flex h-9 w-full items-center justify-center gap-2 rounded-full border border-white/10 bg-white/7 px-3 text-sm font-medium text-white transition-colors hover:bg-white/12 disabled:cursor-not-allowed disabled:opacity-55"
                >
                  {primaryAction.kind === 'play' ? <Play size={14} fill="currentColor" /> : <Info size={14} />}
                  <span className="truncate">{primaryAction.label}</span>
                </button>
              ) : null}
            </div>
          )
        })}
        {visibleGameCount < games.length ? (
          <button
            type="button"
            onClick={() => setVisibleGameCount((current) => Math.min(games.length, current + VISIBLE_GAME_STEP))}
            className="min-h-40 w-24 shrink-0 snap-start rounded-mga border border-mga-border bg-mga-surface/70 px-3 text-sm font-medium text-mga-muted transition-colors hover:border-mga-accent hover:text-mga-text"
            aria-label={`Show more games in ${label}`}
          >
            Show more
          </button>
        ) : hasMoreGames ? (
          <button
            type="button"
            disabled={isLoadingMore}
            onClick={onLoadMore}
            className="min-h-40 w-24 shrink-0 snap-start rounded-mga border border-mga-border bg-mga-surface/70 px-3 text-sm font-medium text-mga-muted transition-colors hover:border-mga-accent hover:text-mga-text disabled:cursor-wait disabled:opacity-60"
            aria-label={`Load more games for ${label}`}
          >
            {isLoadingMore ? 'Loading…' : 'Load more'}
          </button>
        ) : null}
      </div>
      {canScrollLeft && (
        <button
          type="button"
          onClick={() => page(-1)}
          className={cn(
            'absolute left-0 top-1/2 hidden h-12 w-10 -translate-y-1/2 items-center justify-center rounded-mga border border-mga-border bg-mga-bg/90 text-mga-text shadow-lg backdrop-blur transition-colors hover:border-mga-accent sm:flex',
            cardVariant === 'play' && 'left-1 border-white/8 bg-black/72 text-white hover:border-white/16',
          )}
          aria-label={`Previous page in ${label}`}
        >
          <ChevronLeft size={22} />
        </button>
      )}
      {canScrollRight && (
        <button
          type="button"
          onClick={() => page(1)}
          className={cn(
            'absolute right-0 top-1/2 flex h-12 w-10 -translate-y-1/2 items-center justify-center rounded-mga border border-mga-border bg-mga-bg/90 text-mga-text shadow-lg backdrop-blur transition-colors hover:border-mga-accent',
            cardVariant === 'play' && 'right-1 border-white/8 bg-black/72 text-white hover:border-white/16',
          )}
          aria-label={`Next page in ${label}`}
        >
          <ChevronRight size={22} />
        </button>
      )}
    </div>
  )
}
