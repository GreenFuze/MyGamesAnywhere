import { useEffect, useMemo, useRef, useState, type ReactNode } from 'react'
import { ChevronLeft, ChevronRight } from 'lucide-react'
import type { GameDetailResponse } from '@/api/client'
import { GameCard } from '@/components/library/GameCard'
import { animateHorizontalScrollTo } from '@/lib/motion'
import { useTheme } from '@/theme/ThemeProvider'

const GAP_PX = 16
const MIN_CARD_WIDTH = 190
const MAX_CARD_WIDTH = 268

interface HorizontalGameShelfProps {
  games: GameDetailResponse[]
  label: string
  renderHoverAction?: (game: GameDetailResponse) => ReactNode
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

export function HorizontalGameShelf({ games, label, renderHoverAction }: HorizontalGameShelfProps) {
  const viewportRef = useRef<HTMLDivElement>(null)
  const { reducedMotion } = useTheme()
  const [canScrollLeft, setCanScrollLeft] = useState(false)
  const [canScrollRight, setCanScrollRight] = useState(false)
  const [cardWidth, setCardWidth] = useState(MIN_CARD_WIDTH)

  const updateScrollState = () => {
    const el = viewportRef.current
    if (!el) return
    setCanScrollLeft(el.scrollLeft > 4)
    setCanScrollRight(el.scrollLeft + el.clientWidth < el.scrollWidth - 4)
    setCardWidth(computeCardWidth(el.clientWidth))
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
  }, [games.length])

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
    <div className="group/shelf relative">
      <div
        ref={viewportRef}
        className="mga-hidden-scrollbar flex snap-x snap-mandatory gap-4 overflow-x-auto pr-14"
      >
        {games.map((game) => (
          <div key={game.id} className="shrink-0 snap-start" style={{ width: `${cardWidth}px` }}>
            <GameCard game={game} hoverAction={renderHoverAction?.(game)} />
          </div>
        ))}
      </div>
      {canScrollLeft && (
        <button
          type="button"
          onClick={() => page(-1)}
          className="absolute left-0 top-1/2 hidden h-12 w-10 -translate-y-1/2 items-center justify-center rounded-mga border border-mga-border bg-mga-bg/90 text-mga-text shadow-lg backdrop-blur transition-colors hover:border-mga-accent sm:flex"
          aria-label={`Previous page in ${label}`}
        >
          <ChevronLeft size={22} />
        </button>
      )}
      {canScrollRight && (
        <button
          type="button"
          onClick={() => page(1)}
          className="absolute right-0 top-1/2 flex h-12 w-10 -translate-y-1/2 items-center justify-center rounded-mga border border-mga-border bg-mga-bg/90 text-mga-text shadow-lg backdrop-blur transition-colors hover:border-mga-accent"
          aria-label={`Next page in ${label}`}
        >
          <ChevronRight size={22} />
        </button>
      )}
    </div>
  )
}
