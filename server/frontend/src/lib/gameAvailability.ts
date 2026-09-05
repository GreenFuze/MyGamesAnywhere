import type { CatalogOffer } from '@/api/client'

/**
 * Saying what a person can actually do with a game right now.
 *
 * MGA-118 recorded entitlement and availability as independent fields with a
 * deliberate `unknown` in each, and the whole point of keeping them apart was
 * this question: can I play it, do I have to buy it, or is it gone? Until now
 * nothing in the console read them.
 *
 * The rule that matters most here is that `unknown` is never rounded up.
 * Unknown entitlement is not ownership, and unknown availability is not
 * removal — a provider going quiet must not make MGA announce that a game has
 * been taken away.
 */

export type PlayAnswer = {
  /** What the reader is told, in one short phrase. */
  headline: string
  /** Why, when the short phrase would otherwise be a bare assertion. */
  detail: string
  tone: 'good' | 'attention' | 'danger' | 'neutral'
}

const ENTITLEMENT_WORDS: Record<CatalogOffer['entitlement'], string> = {
  owned: 'You own this',
  subscription: 'Included in your subscription',
  shared: 'Shared with you',
  trial: 'Trial',
  none: 'Not yours to play',
  unknown: 'Unknown',
}

const AVAILABILITY_WORDS: Record<CatalogOffer['availability'], string> = {
  available: 'Available now',
  leaving_soon: 'Leaving soon',
  unavailable: 'No longer available',
  unknown: 'Unknown',
}

export function entitlementLabel(value: CatalogOffer['entitlement']): string {
  return ENTITLEMENT_WORDS[value] ?? ENTITLEMENT_WORDS.unknown
}

export function availabilityLabel(value: CatalogOffer['availability']): string {
  return AVAILABILITY_WORDS[value] ?? AVAILABILITY_WORDS.unknown
}

/** The one sentence someone wants when they open a game. */
export function describePlayability(offer: CatalogOffer): PlayAnswer {
  const { entitlement, availability } = offer

  if (availability === 'unavailable') {
    // Gone from the provider. Whether they still have a claim on it changes
    // what they can do about it, so the two cases are not merged.
    return entitlement === 'owned'
      ? {
          headline: 'You own this, but it is gone from the store',
          detail: 'It cannot be downloaded again from here. Anything already downloaded still works.',
          tone: 'attention',
        }
      : {
          headline: 'No longer available',
          detail: 'The provider has stopped offering this, so it cannot be installed from here.',
          tone: 'danger',
        }
  }

  if (availability === 'leaving_soon') {
    return {
      headline: 'Leaving soon',
      detail: entitlement === 'subscription'
        ? 'It is still in your subscription for now. After it leaves you would have to buy it.'
        : 'Still available for now.',
      tone: 'attention',
    }
  }

  switch (entitlement) {
    case 'owned':
      return { headline: 'You own this', detail: 'Available to install.', tone: 'good' }
    case 'subscription':
      return { headline: 'Included in your subscription', detail: 'Available while it stays in the catalogue.', tone: 'good' }
    case 'shared':
      return { headline: 'Shared with you', detail: 'Playable while the owner keeps sharing it.', tone: 'good' }
    case 'trial':
      return { headline: 'Trial', detail: 'A limited version. Buying it unlocks the rest.', tone: 'attention' }
    case 'none':
      return { headline: 'You would need to buy this', detail: 'It is available, but not on your account.', tone: 'attention' }
    default:
      // Deliberately not "you own this" and deliberately not "unavailable".
      // MGA read this from a provider that does not report entitlements, so
      // the only honest answer is that it does not know.
      return {
        headline: 'MGA cannot tell how you have access',
        detail: 'This provider does not say whether a game is owned, subscribed or something else. That is a gap in what it reports, not a problem with the game.',
        tone: 'neutral',
      }
  }
}

/** Offers for one game, newest observation first, so a stale duplicate cannot
 *  outrank the current answer. */
export function offersForGame(offers: CatalogOffer[] | undefined, canonicalGameID: string): CatalogOffer[] {
  return (offers ?? [])
    .filter((offer) => offer.canonical_game_id === canonicalGameID)
    .sort((left, right) => (right.observed_at ?? '').localeCompare(left.observed_at ?? ''))
}

/** True when the observation is old enough that MGA no longer stands behind it. */
export function isStale(offer: CatalogOffer): boolean {
  return Boolean(offer.stale_at)
}
