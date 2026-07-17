import { useMemo } from 'react'
import type { GameDetailResponse } from '@/api/client'
import type { GameCardPlayRoute } from '@/components/library/GameCard'
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
		const emulator = games.filter((game) => game.devices?.some((device) =>
			device.connected && device.can_play && device.emulator_routes?.some((route) => route.state === 'ready'),
		))

    const routeShelves: Array<{
      key: string
      label: string
      games: GameDetailResponse[]
      preferredPlayRoute?: GameCardPlayRoute
    }> = [
      { key: 'favorites', label: 'Favorites', games: favorites },
      { key: 'browser', label: 'Play in browser', games: browser, preferredPlayRoute: 'browser' },
      { key: 'cloud', label: 'Cloud play', games: cloud, preferredPlayRoute: 'cloud' },
			{ key: 'emulator', label: 'Play with an emulator', games: emulator, preferredPlayRoute: 'emulator' },
    ]
    return routeShelves.filter((shelf) => shelf.games.length > 0)
  }, [games])

  return (
    <div className="space-y-8">
      {shelves.map((shelf) => (
        <section key={shelf.key} className="space-y-3">
          <div className="flex items-baseline gap-3">
            <h2 className="text-2xl font-semibold tracking-tight text-mga-text">{shelf.label}</h2>
            <span className="text-sm text-mga-muted">{shelf.games.length}</span>
          </div>
          <HorizontalGameShelf
            games={shelf.games}
            label={shelf.label}
            cardVariant="play"
            preferredPlayRoute={shelf.preferredPlayRoute}
          />
        </section>
      ))}
    </div>
  )
}
