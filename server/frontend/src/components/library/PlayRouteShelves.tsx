import { useMemo } from 'react'
import type { GameDetailResponse } from '@/api/client'
import { HorizontalGameShelf } from '@/components/library/HorizontalGameShelf'
import { isPlayable } from '@/lib/gameUtils'

interface PlayRouteShelvesProps {
  games: GameDetailResponse[]
}

/** Action-oriented, intentionally overlapping shelves for current play routes. */
export function PlayRouteShelves({ games }: PlayRouteShelvesProps) {
  const shelves = useMemo(() => {
    const favorites = games.filter((game) => game.favorite)
    const browser = games.filter((game) => isPlayable(game))
    const cloud = games.filter((game) => game.xcloud_available)

    return [
      { key: 'favorites', label: 'Favorites', games: favorites },
      { key: 'browser', label: 'Play in browser', games: browser },
      { key: 'cloud', label: 'Cloud play', games: cloud },
    ].filter((shelf) => shelf.games.length > 0)
  }, [games])

  return (
    <div className="space-y-8">
      {shelves.map((shelf) => (
        <section key={shelf.key} className="space-y-3">
          <div className="flex items-baseline gap-3">
            <h2 className="text-2xl font-semibold tracking-tight text-mga-text">{shelf.label}</h2>
            <span className="text-sm text-mga-muted">{shelf.games.length}</span>
          </div>
          <HorizontalGameShelf games={shelf.games} label={shelf.label} cardVariant="play" />
        </section>
      ))}
    </div>
  )
}
