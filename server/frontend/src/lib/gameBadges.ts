import type { CatalogOffer, GameDetailResponse } from '@/api/client'
// Relative rather than aliased: these are value imports, and the Node runner
// the unit tests use does not resolve the @/ alias for anything but types.
import { humanizeIdentifier } from './displayText.ts'
import { offersForGame } from './gameAvailability.ts'

/**
 * The short labels on a game, in the order someone needs them.
 *
 * Two sources feed these, and keeping them apart matters. The per-game flags
 * (`is_game_pass`, `xcloud_available`, achievements) come straight from the
 * provider on every scan and are available now. Entitlement and availability
 * come from the catalog observations MGA-118 records, which only exist once a
 * store or subscription source has been scanned — so those badges appear when
 * there is evidence and stay absent when there is not.
 *
 * The rule carried over from MGA-118: absence is never evidence. A game
 * without the Game Pass flag is not therefore owned — it is equally a demo, a
 * trial, an expired subscription or a family share — so no badge claims
 * ownership that MGA did not observe.
 */

export type BadgeTone = 'good' | 'attention' | 'danger' | 'neutral'

export type GameBadge = {
  id: string
  label: string
  tone: BadgeTone
  /** Shown on hover. Present when the label alone would be a bare assertion. */
  title?: string
}

export function gameBadges(game: GameDetailResponse, offers?: CatalogOffer[]): GameBadge[] {
  const badges: GameBadge[] = []
  const sources = game.source_games ?? []

  // Most actionable first. Something the reader has lost outranks something
  // they merely have.
  const everySourceMissing = sources.length > 0 && sources.every((source) => source.status !== 'found')
  if (everySourceMissing) {
    badges.push({
      id: 'missing',
      label: 'Missing',
      tone: 'attention',
      title: 'Found by an earlier scan and not there at the last one. The source may be offline.',
    })
  }

  // Recorded availability, when a source has actually reported some.
  const offer = offersForGame(offers, game.id)[0]
  if (offer) {
    if (offer.availability === 'unavailable') {
      badges.push({
        id: 'unavailable',
        label: 'No longer available',
        tone: 'danger',
        title: 'The provider has stopped offering this, so it cannot be installed from here.',
      })
    } else if (offer.availability === 'leaving_soon') {
      badges.push({
        id: 'leaving',
        label: 'Leaving soon',
        tone: 'attention',
        title: 'Still available for now. After it leaves you would have to buy it.',
      })
    }

    if (offer.entitlement === 'owned') {
      badges.push({ id: 'owned', label: 'Owned', tone: 'good' })
    } else if (offer.entitlement === 'none') {
      badges.push({
        id: 'purchase',
        label: 'Must be bought',
        tone: 'attention',
        title: 'Available, but not on your account.',
      })
    } else if (offer.entitlement === 'trial') {
      badges.push({ id: 'trial', label: 'Trial', tone: 'attention' })
    } else if (offer.entitlement === 'shared') {
      badges.push({ id: 'shared', label: 'Shared with you', tone: 'neutral' })
    }
  }

  // Subscription membership, straight from the provider. Only ever shown when
  // the flag is true: its absence says nothing at all.
  if (game.is_game_pass) {
    badges.push({
      id: 'game-pass',
      label: 'Game Pass',
      tone: 'good',
      title: 'Included in your subscription while it stays in the catalogue.',
    })
  }

  if (game.xcloud_available) {
    badges.push({
      id: 'cloud',
      label: 'Cloud',
      tone: 'neutral',
      title: 'Can be streamed rather than installed.',
    })
  }

  const achievements = game.achievement_summary
  if (achievements && achievements.total_count > 0) {
    const complete = achievements.unlocked_count >= achievements.total_count
    badges.push({
      id: 'achievements',
      label: `${achievements.unlocked_count}/${achievements.total_count}`,
      tone: complete ? 'good' : 'neutral',
      title: complete
        ? 'Every achievement unlocked.'
        : `${achievements.unlocked_count} of ${achievements.total_count} achievements unlocked.`,
    })
  }

  // Not a base game — a DLC or expansion listed in its own right.
  if (game.kind && game.kind !== 'base_game') {
    badges.push({ id: 'kind', label: humanizeIdentifier(game.kind), tone: 'neutral' })
  }

  if ((game.media?.length ?? 0) === 0) {
    badges.push({
      id: 'no-artwork',
      label: 'No artwork',
      tone: 'attention',
      title: 'No cover was found. A metadata source may still fill this in.',
    })
  }

  if (game.favorite) {
    badges.push({ id: 'favorite', label: 'Favourite', tone: 'good' })
  }

  return badges
}

/** The providers a game came from, named the way the user knows them, with
 *  duplicates collapsed: two entries for one game is our bookkeeping. */
export function gameSourceNames(game: GameDetailResponse, label: (pluginId: string) => string): string[] {
  const names = new Set<string>()
  for (const source of game.source_games ?? []) {
    const name = source.integration_label?.trim() || label(source.plugin_id)
    if (name) names.add(name)
  }
  return [...names].sort()
}
