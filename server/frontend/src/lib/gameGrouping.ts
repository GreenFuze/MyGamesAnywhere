import type { GameDetailResponse, LibraryPrefs } from '../api/client.ts'
import { platformLabel, sourceLabel } from './displayText.ts'

export interface GameGroupMembership {
  key: string
  label: string
}

export interface ResolvedGameGroup {
  key: string
  label: string
  games: GameDetailResponse[]
}

function releaseYear(game: GameDetailResponse): string {
  const value = game.release_date?.substring(0, 4)
  return value && /^\d{4}$/.test(value) ? value : 'Unknown year'
}

export class GameGroupingResolver {
  private readonly groupBy: LibraryPrefs['groupBy']

  constructor(groupBy: LibraryPrefs['groupBy']) {
    this.groupBy = groupBy
  }

  memberships(game: GameDetailResponse): GameGroupMembership[] {
    switch (this.groupBy) {
      case 'platform': {
        const labels = new Set<string>()
        if (game.platform) labels.add(platformLabel(game.platform))
        for (const source of game.source_games ?? []) {
          if (source.platform) labels.add(platformLabel(source.platform))
        }
        return Array.from(labels.size > 0 ? labels : ['Unknown platform'])
          .map((label) => ({ key: `platform:${label}`, label }))
      }
      case 'integration': {
        const integrations = new Map<string, string>()
        for (const source of game.source_games ?? []) {
          const key = source.integration_id?.trim() || `${source.plugin_id}:${source.integration_label ?? ''}`
          if (!integrations.has(key)) {
            integrations.set(key, source.integration_label?.trim() || sourceLabel(source.plugin_id))
          }
        }
        return integrations.size > 0
          ? Array.from(integrations, ([key, label]) => ({
              key: `integration:${key}`,
              label,
            }))
          : [{ key: 'integration:unknown', label: 'Unknown connection' }]
      }
      case 'play_method': {
        const memberships: GameGroupMembership[] = []
        if (game.play?.available) memberships.push({ key: 'play:browser', label: 'Play in browser' })
        if (game.xcloud_available || game.play?.options?.some((option) => option.kind === 'xcloud' && option.launchable)) {
          memberships.push({ key: 'play:cloud', label: 'Cloud play' })
        }
        return memberships.length > 0
          ? memberships
          : [{ key: 'play:unavailable', label: 'Not ready to play' }]
      }
      case 'achievements':
        return game.achievement_summary && game.achievement_summary.total_count > 0
          ? [{ key: 'achievements:yes', label: 'Has achievements' }]
          : [{ key: 'achievements:no', label: 'No achievements' }]
      case 'year': {
        const label = releaseYear(game)
        return [{ key: `year:${label}`, label }]
      }
      case 'none':
      default:
        return []
    }
  }

  build(games: GameDetailResponse[]): ResolvedGameGroup[] {
    const groups = new Map<string, { label: string; games: Map<string, GameDetailResponse> }>()
    for (const game of games) {
      for (const membership of this.memberships(game)) {
        const group = groups.get(membership.key) ?? {
          label: membership.label,
          games: new Map<string, GameDetailResponse>(),
        }
        group.games.set(game.id, game)
        groups.set(membership.key, group)
      }
    }

    return Array.from(groups.entries())
      .map(([key, group]) => ({ key, label: group.label, games: Array.from(group.games.values()) }))
      .sort((left, right) => {
        const leftUnknown = left.label.startsWith('Unknown') || left.label === 'Not ready to play'
        const rightUnknown = right.label.startsWith('Unknown') || right.label === 'Not ready to play'
        if (leftUnknown !== rightUnknown) return leftUnknown ? 1 : -1
        if (this.groupBy === 'year' && left.label !== 'Unknown year' && right.label !== 'Unknown year') {
          return Number(right.label) - Number(left.label)
        }
        const labelOrder = left.label.localeCompare(right.label)
        return labelOrder !== 0 ? labelOrder : left.key.localeCompare(right.key)
      })
  }
}
