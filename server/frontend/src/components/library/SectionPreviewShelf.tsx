import { useEffect, useMemo, useRef, useState } from 'react'
import { ChevronLeft, ChevronRight } from 'lucide-react'
import type { GameDetailResponse } from '@/api/client'
import { GameCard } from '@/components/library/GameCard'
import { Button } from '@/components/ui/button'

const GAP_PX = 16
const MIN_CARD_WIDTH = 190
const MAX_CARD_WIDTH = 268
const PREVIEW_PAGE_LIMIT = 3

interface SectionPreviewShelfProps {
  games: GameDetailResponse[]
  label: string
  onOpenShelf: () => void
}

function computeColumns(width: number): number {
  if (width <= 0) return 1

  let columns = Math.max(1, Math.floor((width + GAP_PX) / (MIN_CARD_WIDTH + GAP_PX)))
  let cardWidth = Math.floor((width - GAP_PX * (columns - 1)) / columns)

  while (columns < 12 && cardWidth > MAX_CARD_WIDTH) {
    columns++
    cardWidth = Math.floor((width - GAP_PX * (columns - 1)) / columns)
  }

  return Math.max(1, columns)
}

export function SectionPreviewShelf({ games, label, onOpenShelf }: SectionPreviewShelfProps) {
  const containerRef = useRef<HTMLDivElement>(null)
  const [columns, setColumns] = useState(1)
  const [pageIndex, setPageIndex] = useState(0)

  useEffect(() => {
    const el = containerRef.current
    if (!el) return

    const updateColumns = () => {
      const nextColumns = computeColumns(el.clientWidth)
      setColumns((prev) => (prev === nextColumns ? prev : nextColumns))
    }

    updateColumns()
    const observer = new ResizeObserver(updateColumns)
    observer.observe(el)
    return () => observer.disconnect()
  }, [])

  const pageSize = Math.max(1, columns)
  const actualPageCount = Math.max(1, Math.ceil(games.length / pageSize))
  const previewPageCount = Math.min(actualPageCount, PREVIEW_PAGE_LIMIT)
  const canOpenShelf = actualPageCount > PREVIEW_PAGE_LIMIT

  useEffect(() => {
    setPageIndex((prev) => Math.min(prev, previewPageCount - 1))
  }, [previewPageCount])

  const previewGames = useMemo(() => {
    const start = pageIndex * pageSize
    return games.slice(start, start + pageSize)
  }, [games, pageIndex, pageSize])
  const visibleColumns = Math.max(1, Math.min(columns, previewGames.length))

  const showNextPage = pageIndex < previewPageCount - 1
  const showOpenShelf = !showNextPage && canOpenShelf

  return (
    <div className="relative" ref={containerRef}>
      <div
        className="grid gap-4 pr-24"
        style={{ gridTemplateColumns: `repeat(${visibleColumns}, minmax(${MIN_CARD_WIDTH}px, ${MAX_CARD_WIDTH}px))` }}
      >
        {previewGames.map((game) => (
          <div key={game.id} className="min-w-0">
            <GameCard game={game} />
          </div>
        ))}
      </div>

      {pageIndex > 0 && (
        <button
          type="button"
          onClick={() => setPageIndex((current) => Math.max(0, current - 1))}
          className="absolute left-0 top-1/2 hidden h-12 w-10 -translate-y-1/2 items-center justify-center rounded-mga border border-mga-border bg-mga-bg/90 text-mga-text shadow-lg backdrop-blur transition-colors hover:border-mga-accent sm:flex"
          aria-label={`Previous page in ${label}`}
        >
          <ChevronLeft size={22} />
        </button>
      )}

      {showNextPage && (
        <button
          type="button"
          onClick={() => setPageIndex((current) => Math.min(previewPageCount - 1, current + 1))}
          className="absolute right-0 top-1/2 flex h-12 w-10 -translate-y-1/2 items-center justify-center rounded-mga border border-mga-border bg-mga-bg/90 text-mga-text shadow-lg backdrop-blur transition-colors hover:border-mga-accent"
          aria-label={`Next page in ${label}`}
        >
          <ChevronRight size={22} />
        </button>
      )}

      {showOpenShelf && (
        <div className="absolute right-0 top-1/2 -translate-y-1/2">
          <Button type="button" variant="outline" size="sm" onClick={onOpenShelf}>
            Open Shelf
          </Button>
        </div>
      )}
    </div>
  )
}
