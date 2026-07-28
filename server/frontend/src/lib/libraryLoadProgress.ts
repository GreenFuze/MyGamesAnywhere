export type LibraryLoadMode = 'browse' | 'search' | 'filter' | 'group'

export interface LibraryLoadProgressInput {
  target: 'Library' | 'Play'
  mode: LibraryLoadMode
  loadedCount: number
  totalCount: number
  isInitialLoading: boolean
  hasMore: boolean
  errorMessage?: string
}

/**
 * Player-facing state for progressive Library and Play loading.
 *
 * Keeping this calculation separate from the React view makes completion,
 * partial progress, and errors use the same rules on both pages.
 */
export class LibraryLoadProgressModel {
  readonly target: LibraryLoadProgressInput['target']
  readonly mode: LibraryLoadMode
  readonly loadedCount: number
  readonly totalCount: number
  readonly isInitialLoading: boolean
  readonly hasMore: boolean
  readonly errorMessage?: string

  constructor(input: LibraryLoadProgressInput) {
    this.target = input.target
    this.mode = input.mode
    this.loadedCount = Math.max(0, input.loadedCount)
    this.totalCount = Math.max(0, input.totalCount)
    this.isInitialLoading = input.isInitialLoading
    this.hasMore = input.hasMore
    this.errorMessage = input.errorMessage?.trim() || undefined
  }

  get isError(): boolean {
    return this.errorMessage !== undefined
  }

  get isLoading(): boolean {
    return !this.isError && (this.isInitialLoading || this.hasMore)
  }

  get isVisible(): boolean {
    return this.isError || this.isLoading
  }

  get percentage(): number | undefined {
    if (this.totalCount <= 0) return undefined
    return Math.min(100, (this.loadedCount / this.totalCount) * 100)
  }

  get title(): string {
    if (this.isError) {
      return this.loadedCount > 0
        ? `Couldn't finish loading ${this.target}`
        : `Couldn't load ${this.target}`
    }
    switch (this.mode) {
      case 'search':
        return 'Searching your games'
      case 'filter':
        return 'Applying your filters'
      case 'group':
        return 'Building your groups'
      default:
        return `Loading ${this.target}`
    }
  }

  get detail(): string {
    if (this.isError) {
      if (this.target === 'Play' && this.loadedCount > 0) {
        return `Checked ${this.loadedCount} of ${this.totalCount || '?'} library games. Try again to check the rest.`
      }
      return this.loadedCount > 0
        ? `${this.loadedCount} of ${this.totalCount || '?'} games are ready. Try again to load the rest.`
        : 'Your games could not be loaded. Try again.'
    }
    if (this.totalCount <= 0) return 'Getting the first games ready…'
    if (this.target === 'Play') {
      return `Checked ${this.loadedCount} of ${this.totalCount} library games`
    }
    return `${this.loadedCount} of ${this.totalCount} games ready`
  }

  get toolbarLabel(): string {
    if (this.isError) {
      return this.loadedCount > 0 ? `${this.loadedCount} games ready` : 'Games unavailable'
    }
    if (this.isLoading) {
      if (this.target === 'Play' && this.totalCount > 0) {
        return `Checking ${this.loadedCount} of ${this.totalCount}`
      }
      return this.totalCount > 0
        ? `${this.loadedCount} of ${this.totalCount} ready`
        : 'Loading games…'
    }
    return `${this.loadedCount} games`
  }
}
