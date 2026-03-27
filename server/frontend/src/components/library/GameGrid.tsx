import type { GameDetailResponse } from '@/api/client'
import { Skeleton } from '@/components/ui/skeleton'
import { GameCard } from '@/components/library/GameCard'

interface GameGridProps {
  games: GameDetailResponse[]
  isLoading: boolean
}

export function GameGrid({ games, isLoading }: GameGridProps) {
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
    <div className="grid grid-cols-[repeat(auto-fill,minmax(190px,1fr))] gap-4">
      {games.map((game) => (
        <GameCard key={game.id} game={game} />
      ))}
    </div>
  )
}
