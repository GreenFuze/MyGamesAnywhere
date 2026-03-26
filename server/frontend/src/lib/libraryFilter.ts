import type { GameDetailResponse, LibraryPrefs } from '@/api/client'
import { isActionable, isPlayable } from '@/lib/gameUtils'

// ---------------------------------------------------------------------------
// Types
// ---------------------------------------------------------------------------

export type CollectionScope = 'library' | 'play'

export type FilterState = {
  search: string
  platforms: string[]
  genres: string[]
  yearMin: number | null
  yearMax: number | null
  developer: string
  publisher: string
  source: string
  playableOnly: boolean
  xcloudOnly: boolean
  gamePassOnly: boolean
  sortBy: LibraryPrefs['sortBy']
  sortDir: LibraryPrefs['sortDir']
}

export const DEFAULT_FILTER_STATE: FilterState = {
  search: '',
  platforms: [],
  genres: [],
  yearMin: null,
  yearMax: null,
  developer: '',
  publisher: '',
  source: '',
  playableOnly: false,
  xcloudOnly: false,
  gamePassOnly: false,
  sortBy: 'title',
  sortDir: 'asc',
}

// ---------------------------------------------------------------------------
// Section pre-filter
// ---------------------------------------------------------------------------

export function applyScopeFilter(
  games: GameDetailResponse[],
  scope: CollectionScope,
): GameDetailResponse[] {
  if (scope === 'play') return games.filter((g) => isActionable(g))
  return games
}

// ---------------------------------------------------------------------------
// Helpers
// ---------------------------------------------------------------------------

function parseYear(dateStr: string | undefined): number | null {
  if (!dateStr) return null
  const y = parseInt(dateStr.substring(0, 4), 10)
  return Number.isNaN(y) ? null : y
}

function uniqueSorted(items: string[]): string[] {
  return Array.from(new Set(items)).sort((a, b) => a.localeCompare(b))
}

// ---------------------------------------------------------------------------
// LibraryFilter — encapsulates all client-side filter + sort logic
// ---------------------------------------------------------------------------

export class LibraryFilter {
  constructor(private readonly games: GameDetailResponse[]) {}

  /** Apply all filters and sort, returning the visible subset. */
  apply(state: FilterState): GameDetailResponse[] {
    const filtered = this.games.filter((g) => {
      // Search: substring match on title
      if (state.search) {
        const q = state.search.toLowerCase()
        if (!g.title.toLowerCase().includes(q)) return false
      }

      // Platform
      if (state.platforms.length > 0 && !state.platforms.includes(g.platform)) {
        return false
      }

      // Genre
      if (state.genres.length > 0) {
        if (!g.genres || !g.genres.some((genre) => state.genres.includes(genre))) {
          return false
        }
      }

      // Year range
      const year = parseYear(g.release_date)
      if (state.yearMin !== null && (year === null || year < state.yearMin)) return false
      if (state.yearMax !== null && (year === null || year > state.yearMax)) return false

      // Developer substring
      if (state.developer) {
        if (!g.developer?.toLowerCase().includes(state.developer.toLowerCase())) return false
      }

      // Publisher substring
      if (state.publisher) {
        if (!g.publisher?.toLowerCase().includes(state.publisher.toLowerCase())) return false
      }

      // Source plugin
      if (state.source) {
        if (!g.source_games?.some((sg) => sg.plugin_id === state.source)) return false
      }

      // Flag toggles
      if (state.playableOnly && !isPlayable(g.platform)) return false
      if (state.xcloudOnly && !g.xcloud_available) return false
      if (state.gamePassOnly && !g.is_game_pass) return false

      return true
    })

    return this.sort(filtered, state.sortBy, state.sortDir)
  }

  // -- Facet extraction (from full unfiltered list) -------------------------

  allPlatforms(): string[] {
    return uniqueSorted(this.games.map((g) => g.platform))
  }

  allGenres(): string[] {
    const genres: string[] = []
    for (const g of this.games) {
      if (g.genres) genres.push(...g.genres)
    }
    return uniqueSorted(genres)
  }

  allDevelopers(): string[] {
    return uniqueSorted(
      this.games.map((g) => g.developer).filter((d): d is string => !!d),
    )
  }

  allPublishers(): string[] {
    return uniqueSorted(
      this.games.map((g) => g.publisher).filter((p): p is string => !!p),
    )
  }

  allSources(): string[] {
    const ids: string[] = []
    for (const g of this.games) {
      if (g.source_games) {
        for (const sg of g.source_games) ids.push(sg.plugin_id)
      }
    }
    return uniqueSorted(ids)
  }

  yearRange(): [number, number] | null {
    let min = Infinity
    let max = -Infinity
    for (const g of this.games) {
      const y = parseYear(g.release_date)
      if (y !== null) {
        if (y < min) min = y
        if (y > max) max = y
      }
    }
    return min <= max ? [min, max] : null
  }

  // -- Sorting --------------------------------------------------------------

  private sort(
    games: GameDetailResponse[],
    sortBy: LibraryPrefs['sortBy'],
    sortDir: LibraryPrefs['sortDir'],
  ): GameDetailResponse[] {
    const dir = sortDir === 'asc' ? 1 : -1

    return [...games].sort((a, b) => {
      let cmp = 0

      switch (sortBy) {
        case 'title':
          cmp = (a.title || '').localeCompare(b.title || '')
          break
        case 'release_date':
          cmp = (a.release_date || '').localeCompare(b.release_date || '')
          break
        case 'platform':
          cmp = (a.platform || '').localeCompare(b.platform || '')
          break
        case 'rating':
          cmp = (a.rating ?? 0) - (b.rating ?? 0)
          break
      }

      // Secondary sort by id for stability
      if (cmp === 0) cmp = a.id.localeCompare(b.id)

      return cmp * dir
    })
  }
}
