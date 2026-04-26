import { useEffect, useMemo, useRef, useState } from 'react'
import type { GameDetailResponse } from '@/api/client'
import { Skeleton } from '@/components/ui/skeleton'
import { GameCard } from '@/components/library/GameCard'

interface GameGridProps {
  games: GameDetailResponse[]
  isLoading: boolean
  progressive?: boolean
  initialRows?: number
  loadMoreRows?: number
  cardVariant?: 'library' | 'play'
}

const GRID_GAP_PX = 16
const GRID_MIN_CARD_WIDTH = 190

function computeColumns(width: number): number {
  if (width <= 0) return 1
  return Math.max(1, Math.floor((width + GRID_GAP_PX) / (GRID_MIN_CARD_WIDTH + GRID_GAP_PX)))
}

export function GameGrid({
  games,
  isLoading,
  progressive = false,
  initialRows = 4,
  loadMoreRows = 3,
  cardVariant = 'library',
}: GameGridProps) {
  const containerRef = useRef<HTMLDivElement | null>(null)
  const sentinelRef = useRef<HTMLDivElement | null>(null)
  const [columns, setColumns] = useState(1)
  const [loadedRows, setLoadedRows] = useState(initialRows)

  useEffect(() => {
    if (!progressive) return
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
  }, [progressive])

  useEffect(() => {
    setLoadedRows(initialRows)
  }, [games, initialRows, progressive])

  const visibleInitialCount = Math.max(1, columns) * initialRows
  const progressiveEnabled = progressive && games.length > visibleInitialCount
  const visibleCount = progressiveEnabled
    ? Math.min(games.length, loadedRows * Math.max(1, columns))
    : games.length
  const visibleGames = useMemo(() => games.slice(0, visibleCount), [games, visibleCount])
  const hasMore = progressiveEnabled && visibleCount < games.length

  useEffect(() => {
    if (!hasMore) return
    const sentinel = sentinelRef.current
    if (!sentinel) return

    const observer = new IntersectionObserver(
      (entries) => {
        for (const entry of entries) {
          if (!entry.isIntersecting) continue
          setLoadedRows((current) => current + loadMoreRows)
        }
      },
      {
        root: null,
        rootMargin: '320px 0px',
        threshold: 0,
      },
    )

    observer.observe(sentinel)
    return () => observer.disconnect()
  }, [hasMore, loadMoreRows, visibleCount])

  if (isLoading) {
    return (
      <div className="grid grid-cols-[repeat(auto-fill,minmax(190px,1fr))] gap-4">
        {Array.from({ length: 12 }, (_, i) => (
          <div key={i} className="overflow-hidden rounded-mga border border-mga-border bg-mga-surface">
            <Skeleton className="aspect-[2/3] w-full rounded-none" />
            <div className="space-y-2 p-2">
              <Skeleton className="h-4 w-3/4" />
              <Skeleton className="h-3 w-1/2" />
            </div>
          </div>
        ))}
      </div>
    )
  }

  return (
    <div ref={containerRef} className="space-y-4">
      <div className="mga-grid-fade grid grid-cols-[repeat(auto-fill,minmax(190px,1fr))] gap-4">
        {visibleGames.map((game, index) => (
          <div
            key={game.id}
            className="mga-stagger-item"
            style={{ animationDelay: `${Math.min(index, 10) * 40}ms` }}
          >
            <GameCard game={game} variant={cardVariant} />
          </div>
        ))}
      </div>
      {hasMore && <div ref={sentinelRef} aria-hidden="true" className="h-1 w-full" />}
    </div>
  )
}
