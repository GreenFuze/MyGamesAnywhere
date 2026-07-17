export type GameCardPlayRoute = 'browser' | 'cloud' | 'emulator' | 'local' | 'storefront'

export interface GameCardPrimaryAction {
  id?: string
  label: string
  onSelect: () => void
  disabled?: boolean
  kind?: 'play' | 'details'
  route?: GameCardPlayRoute
  title?: string
}

export interface GameCardActionResolution {
  primaryAction: GameCardPrimaryAction
  alternateActions: GameCardPrimaryAction[]
}

interface GameCardActionResolverInput {
  primaryAction?: GameCardPrimaryAction
  alternateActions: GameCardPrimaryAction[]
  derivedActions: GameCardPrimaryAction[]
  preferredPlayRoute?: GameCardPlayRoute
  fallbackAction: GameCardPrimaryAction
}

/** Resolves a contextual default without discarding any distinct playable route. */
export class GameCardActionResolver {
  readonly #input: GameCardActionResolverInput

  constructor(input: GameCardActionResolverInput) {
    this.#input = input
  }

  resolve(): GameCardActionResolution {
    const explicitActions = [this.#input.primaryAction, ...this.#input.alternateActions].filter(
      (action): action is GameCardPrimaryAction => action !== undefined,
    )
    const explicitDerivedRoutes = new Set(
      explicitActions
        .map((action) => action.route)
        .filter((route): route is 'browser' | 'cloud' => route === 'browser' || route === 'cloud'),
    )
    const availableActions = [
      ...explicitActions,
      ...this.#input.derivedActions.filter((action) => {
        if (action.route !== 'browser' && action.route !== 'cloud') return true
        return !explicitDerivedRoutes.has(action.route)
      }),
    ]
    const preferredAction = this.#input.preferredPlayRoute
      ? availableActions.find((action) => action.route === this.#input.preferredPlayRoute)
      : undefined
    const primaryAction = preferredAction
      ?? this.#input.primaryAction
      ?? availableActions[0]
      ?? this.#input.fallbackAction

    return {
      primaryAction,
      alternateActions: availableActions.filter((action) => action !== primaryAction),
    }
  }
}
