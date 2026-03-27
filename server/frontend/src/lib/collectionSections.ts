import type {
  CollectionSectionConfig,
  CollectionSectionField,
  GameDetailResponse,
} from '@/api/client'
import { pluginLabel } from '@/lib/gameUtils'

export type SectionOption = {
  value: string
  label: string
  count: number
}

const FIELD_LABELS: Record<CollectionSectionField, string> = {
  platform: 'Platform',
  genre: 'Genre',
  developer: 'Developer',
  publisher: 'Publisher',
  source: 'Source',
  year: 'Year',
}

export function createAllGamesSection(): CollectionSectionConfig {
  return { id: 'all-games', kind: 'all', label: 'All Games' }
}

export function defaultSections(): CollectionSectionConfig[] {
  return [createAllGamesSection()]
}

function parseYear(dateStr: string | undefined): string | null {
  if (!dateStr) return null
  const year = dateStr.substring(0, 4)
  return year && /^\d{4}$/.test(year) ? year : null
}

function stableGroupId(field: CollectionSectionField, value: string): string {
  return `${field}:${encodeURIComponent(value)}`
}

function displayValue(field: CollectionSectionField, value: string): string {
  if (field === 'source') return pluginLabel(value)
  return value
}

export function makeGroupSection(
  field: CollectionSectionField,
  value: string,
): CollectionSectionConfig | null {
  const trimmed = value.trim()
  if (!trimmed) return null
  return {
    id: stableGroupId(field, trimmed),
    kind: 'group',
    field,
    value: trimmed,
    label: `${FIELD_LABELS[field]}: ${displayValue(field, trimmed)}`,
  }
}

export function sanitizeSections(raw: unknown): CollectionSectionConfig[] {
  if (!Array.isArray(raw)) return defaultSections()

  const seen = new Set<string>()
  const next: CollectionSectionConfig[] = []

  for (const item of raw) {
    if (!item || typeof item !== 'object') continue
    const section = item as Partial<CollectionSectionConfig>

    if (section.kind === 'all') {
      const normalized = createAllGamesSection()
      if (!seen.has(normalized.id)) {
        seen.add(normalized.id)
        next.push(normalized)
      }
      continue
    }

    if (section.kind !== 'group') continue
    if (!section.field || typeof section.field !== 'string') continue
    if (!section.value || typeof section.value !== 'string') continue
    const normalized = makeGroupSection(section.field as CollectionSectionField, section.value)
    if (!normalized || seen.has(normalized.id)) continue
    seen.add(normalized.id)
    next.push(normalized)
  }

  return next.length > 0 ? next : defaultSections()
}

export function filterGamesBySection(
  games: GameDetailResponse[],
  section: CollectionSectionConfig,
): GameDetailResponse[] {
  if (section.kind === 'all') return games

  switch (section.field) {
    case 'platform':
      return games.filter((game) => game.platform === section.value)
    case 'genre':
      return games.filter((game) => game.genres?.includes(section.value))
    case 'developer':
      return games.filter((game) => game.developer === section.value)
    case 'publisher':
      return games.filter((game) => game.publisher === section.value)
    case 'source':
      return games.filter((game) =>
        game.source_games.some((sourceGame) => sourceGame.plugin_id === section.value),
      )
    case 'year':
      return games.filter((game) => parseYear(game.release_date) === section.value)
  }
}

export function getSectionOptions(
  games: GameDetailResponse[],
  field: CollectionSectionField,
): SectionOption[] {
  const counts = new Map<string, number>()

  const add = (value: string | null | undefined) => {
    if (!value) return
    const trimmed = value.trim()
    if (!trimmed) return
    counts.set(trimmed, (counts.get(trimmed) ?? 0) + 1)
  }

  for (const game of games) {
    switch (field) {
      case 'platform':
        add(game.platform)
        break
      case 'genre':
        for (const genre of game.genres ?? []) add(genre)
        break
      case 'developer':
        add(game.developer)
        break
      case 'publisher':
        add(game.publisher)
        break
      case 'source': {
        const uniqueSources = new Set(game.source_games.map((sourceGame) => sourceGame.plugin_id))
        for (const source of uniqueSources) add(source)
        break
      }
      case 'year':
        add(parseYear(game.release_date))
        break
    }
  }

  const options = Array.from(counts.entries()).map(([value, count]) => ({
    value,
    count,
    label: displayValue(field, value),
  }))

  if (field === 'year') {
    return options.sort((a, b) => Number(b.value) - Number(a.value))
  }

  return options.sort((a, b) => a.label.localeCompare(b.label))
}
