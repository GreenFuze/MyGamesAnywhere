import { useMemo } from 'react'
import type { GameDetailResponse, LibraryPrefs } from '@/api/client'
import { GameGrid } from '@/components/library/GameGrid'
import { GameListView } from '@/components/library/GameListView'
import { HorizontalGameShelf } from '@/components/library/HorizontalGameShelf'
import { isPlayable, platformLabel } from '@/lib/gameUtils'

interface GameGroup {
  key: string
  label: string
  games: GameDetailResponse[]
}

interface GroupedGameViewProps {
  games: GameDetailResponse[]
  groupBy: LibraryPrefs['groupBy']
  viewMode: LibraryPrefs['viewMode']
  scope: 'library' | 'play'
  selectedIds?: Set<string>
  onToggleSelected?: (game: GameDetailResponse) => void
}

function releaseYear(game: GameDetailResponse): string {
  const value = game.release_date?.substring(0, 4)
  return value && /^\d{4}$/.test(value) ? value : 'Unknown year'
}

function groupLabels(game: GameDetailResponse, groupBy: LibraryPrefs['groupBy']): string[] {
  switch (groupBy) {
    case 'platform': {
      const labels = new Set<string>()
      if (game.platform) labels.add(platformLabel(game.platform))
      for (const source of game.source_games ?? []) {
        if (source.platform) labels.add(platformLabel(source.platform))
      }
      return labels.size > 0 ? Array.from(labels) : ['Unknown platform']
    }
    case 'integration': {
      const labels = new Set(
        (game.source_games ?? []).map((source) => source.integration_label || source.integration_id),
      )
      return labels.size > 0 ? Array.from(labels) : ['Unknown connection']
    }
    case 'play_method': {
      const labels: string[] = []
      if (isPlayable(game)) labels.push('Play in browser')
      if (game.xcloud_available) labels.push('Cloud play')
      return labels.length > 0 ? labels : ['Not ready to play']
    }
    case 'achievements':
      return game.achievement_summary && game.achievement_summary.total_count > 0
        ? ['Has achievements']
        : ['No achievements']
    case 'year':
      return [releaseYear(game)]
    case 'none':
    default:
      return []
  }
}

function buildGroups(games: GameDetailResponse[], groupBy: LibraryPrefs['groupBy']): GameGroup[] {
  const byLabel = new Map<string, Map<string, GameDetailResponse>>()
  for (const game of games) {
    for (const label of groupLabels(game, groupBy)) {
      const members = byLabel.get(label) ?? new Map<string, GameDetailResponse>()
      members.set(game.id, game)
      byLabel.set(label, members)
    }
  }

  return Array.from(byLabel.entries())
    .map(([label, members]) => ({ key: label.toLowerCase(), label, games: Array.from(members.values()) }))
    .sort((left, right) => {
      const leftUnknown = left.label.startsWith('Unknown') || left.label === 'Not ready to play'
      const rightUnknown = right.label.startsWith('Unknown') || right.label === 'Not ready to play'
      if (leftUnknown !== rightUnknown) return leftUnknown ? 1 : -1
      if (groupBy === 'year' && left.label !== 'Unknown year' && right.label !== 'Unknown year') {
        return Number(right.label) - Number(left.label)
      }
      return left.label.localeCompare(right.label)
    })
}

export function GroupedGameView({
  games,
  groupBy,
  viewMode,
  scope,
  selectedIds = new Set<string>(),
  onToggleSelected = () => undefined,
}: GroupedGameViewProps) {
  const groups = useMemo(() => buildGroups(games, groupBy), [games, groupBy])

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
