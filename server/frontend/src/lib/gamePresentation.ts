import type { GameDetailResponse } from '../api/client.ts'
import { platformLabel, sourceLabel } from './displayText.ts'
import { GameContentPresentation } from './gameContent.ts'
import { collectSaveDomains, saveDomainSummary } from './saveDomains.ts'

export type GameAvailabilityKind =
  | 'installed'
  | 'playable'
  | 'xcloud'
  | 'emulator'
  | 'gamepass'
  | 'shared'

export interface GameSourcePresentation {
  key: string
  label: string
  pluginId: string
}

/**
 * A player-facing projection shared by every library density.
 *
 * It deliberately keeps copies, play routes, achievements, and saves separate:
 * those facts may overlap and must not be collapsed into a single "source".
 */
export class GamePresentation {
  readonly game: GameDetailResponse
  readonly content: GameContentPresentation
  readonly platform: string
  readonly sources: GameSourcePresentation[]
  readonly availability: GameAvailabilityKind | null
  readonly achievementLabel: string | null
  readonly saveLabel: string | null

  constructor(game: GameDetailResponse) {
    this.game = game
    this.content = new GameContentPresentation(game.kind)
    this.platform = platformLabel(game.platform)
    this.sources = this.#sourcePresentations()
    this.availability = this.#primaryAvailability()
    this.achievementLabel = this.#achievementLabel()
    this.saveLabel = saveDomainSummary(collectSaveDomains(game))
  }

  get copyCount(): number {
    return this.game.source_games.length
  }

  get foundInLabel(): string {
    if (this.sources.length === 0) return 'No connected library'
    const visible = this.sources.slice(0, 2).map((source) => source.label)
    const remainder = this.sources.length - visible.length
    return `Found in ${visible.join(' and ')}${remainder > 0 ? ` +${remainder}` : ''}`
  }

  get copyCountLabel(): string {
    return `${this.copyCount} ${this.copyCount === 1 ? 'copy' : 'copies'}`
  }

  get compactBadgeCount(): number {
    return [
      this.content.badgeLabel,
      this.availability,
      this.platform,
    ].filter(Boolean).length
  }

  #sourcePresentations(): GameSourcePresentation[] {
    const seen = new Set<string>()
    const sources: GameSourcePresentation[] = []

    for (const source of this.game.source_games) {
      const label = source.integration_label?.trim() || sourceLabel(source.plugin_id)
      const key = source.integration_id?.trim() || `${source.plugin_id}:${label}`
      if (seen.has(key)) continue
      seen.add(key)
      sources.push({ key, label, pluginId: source.plugin_id })
    }

    return sources.sort((left, right) => {
      const labelOrder = left.label.localeCompare(right.label)
      return labelOrder !== 0 ? labelOrder : left.key.localeCompare(right.key)
    })
  }

  #primaryAvailability(): GameAvailabilityKind | null {
    if (this.game.play?.available) return 'playable'
    if (
      this.game.xcloud_available
      || this.game.play?.options?.some(
        (option) => option.kind === 'xcloud' && option.launchable,
      )
    ) return 'xcloud'
    if (this.game.is_game_pass) return 'gamepass'
    if (this.game.shared) return 'shared'
    return null
  }

  #achievementLabel(): string | null {
    const summary = this.game.achievement_summary
    if (!summary || summary.total_count <= 0) return null
    return `${summary.unlocked_count} of ${summary.total_count} achievements`
  }
}
