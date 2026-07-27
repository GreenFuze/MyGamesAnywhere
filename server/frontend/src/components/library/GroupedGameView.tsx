import { useMemo } from 'react'
import type { GameDetailResponse, LibraryPrefs } from '@/api/client'
import { GameGrid } from '@/components/library/GameGrid'
import { GameListView } from '@/components/library/GameListView'
import { HorizontalGameShelf } from '@/components/library/HorizontalGameShelf'
import { GameGroupingResolver } from '@/lib/gameGrouping'

interface GroupedGameViewProps {
  games: GameDetailResponse[]
  groupBy: LibraryPrefs['groupBy']
  viewMode: LibraryPrefs['viewMode']
  scope: 'library' | 'play'
  selectedIds?: Set<string>
  onToggleSelected?: (game: GameDetailResponse) => void
}

export function GroupedGameView({
  games,
  groupBy,
  viewMode,
  scope,
  selectedIds = new Set<string>(),
  onToggleSelected = () => undefined,
}: GroupedGameViewProps) {
  const groups = useMemo(() => new GameGroupingResolver(groupBy).build(games), [games, groupBy])

  return (
    <div className="space-y-8">
      {groups.map((group) => (
        <section key={group.key} className="space-y-3">
          <div className="flex items-baseline gap-3 border-b border-mga-border pb-2">
            <h2 className="text-xl font-semibold tracking-tight text-mga-text">{group.label}</h2>
            <span className="text-sm text-mga-muted">{group.games.length}</span>
          </div>
          {viewMode === 'shelf' ? (
            <HorizontalGameShelf
              games={group.games}
              label={group.label}
              cardVariant={scope === 'play' ? 'play' : 'library'}
            />
          ) : viewMode === 'list' && scope === 'library' ? (
            <GameListView
              games={group.games}
              selectedIds={selectedIds}
              onToggleSelected={onToggleSelected}
            />
          ) : (
            <GameGrid
              games={group.games}
              isLoading={false}
              cardVariant={scope === 'play' ? 'play' : 'library'}
            />
          )}
        </section>
      ))}
    </div>
  )
}
