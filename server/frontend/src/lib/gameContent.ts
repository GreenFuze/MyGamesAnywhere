export type ContentRelationshipState = 'known' | 'unlinked' | 'ambiguous' | string

type ContentKindDetails = {
  badge: string | null
  noun: string
  plural: string
}

const CONTENT_KINDS: Record<string, ContentKindDetails> = {
  base_game: { badge: null, noun: 'game', plural: 'games' },
  dlc: { badge: 'DLC', noun: 'DLC', plural: 'DLC' },
  expansion: { badge: 'Expansion', noun: 'expansion', plural: 'expansions' },
  addon: { badge: 'Add-on', noun: 'add-on', plural: 'add-ons' },
  patch: { badge: 'Update', noun: 'update', plural: 'updates' },
  extras: { badge: 'Bonus content', noun: 'bonus content', plural: 'bonus content' },
  unknown: { badge: 'Needs review', noun: 'item', plural: 'items' },
}

export class GameContentPresentation {
  readonly kind: string
  readonly badgeLabel: string | null
  readonly noun: string
  readonly pluralNoun: string

  constructor(kind: string | undefined) {
    this.kind = (kind ?? '').trim().toLowerCase()
    const details = CONTENT_KINDS[this.kind] ?? CONTENT_KINDS.unknown
    this.badgeLabel = details.badge
    this.noun = details.noun
    this.pluralNoun = details.plural
  }

  relationshipMessage(state: ContentRelationshipState | undefined): string | null {
    if (!this.badgeLabel || state === 'known') return null
    if (state === 'ambiguous') {
      return `MGA found more than one possible game for this ${this.noun}. Choose the right relationship in Library Review.`
    }
    if (state === 'unlinked') {
      return `MGA knows this is ${articleFor(this.noun)} ${this.noun}, but its game is not linked yet.`
    }
    return null
  }
}

function articleFor(noun: string): 'a' | 'an' {
  return /^[aeiou]/i.test(noun) ? 'an' : 'a'
}
