import type { GameMediaDetailDTO } from '@/api/client'
import { GameMediaCollection, youtubeEmbedUrl } from '@/lib/gameMedia'

export interface DisplayMediaItem {
  key: string
  media: GameMediaDetailDTO
  sources: string[]
  types: string[]
}

export type FeaturedMediaType = 'video' | 'cover' | 'background' | 'screenshot' | 'artwork' | 'logo' | 'other'

const FEATURED_TYPE_ORDER: FeaturedMediaType[] = ['video', 'cover', 'background', 'screenshot', 'artwork', 'logo', 'other']

function normalizeMediaUrl(url: string): string {
  try {
    const parsed = new URL(url)
    parsed.hash = ''
    return parsed.toString()
  } catch {
    return url.trim().toLowerCase()
  }
}

export function mediaIdentityKey(media: Pick<GameMediaDetailDTO, 'asset_id' | 'url'>): string {
  if (typeof media.asset_id === 'number' && media.asset_id > 0) {
    return `asset:${media.asset_id}`
  }
  return `url:${normalizeMediaUrl(media.url)}`
}

export function normalizeFeaturedMediaType(type: string): FeaturedMediaType {
  switch (type) {
    case 'video':
    case 'trailer':
      return 'video'
    case 'cover':
      return 'cover'
    case 'background':
    case 'backdrop':
    case 'hero':
    case 'fanart':
      return 'background'
    case 'screenshot':
      return 'screenshot'
    case 'artwork':
    case 'banner':
      return 'artwork'
    case 'logo':
    case 'icon':
      return 'logo'
    default:
      return 'other'
  }
}

function mediaTypeRank(type: string): number {
  const idx = FEATURED_TYPE_ORDER.indexOf(normalizeFeaturedMediaType(type))
  return idx === -1 ? FEATURED_TYPE_ORDER.length : idx
}

function uniqueStrings(values: Array<string | null | undefined>): string[] {
  return Array.from(new Set(values.map((value) => value?.trim()).filter((value): value is string => Boolean(value))))
}

function chooseRepresentativeMedia(current: GameMediaDetailDTO, candidate: GameMediaDetailDTO): GameMediaDetailDTO {
  const currentRank = mediaTypeRank(current.type)
  const candidateRank = mediaTypeRank(candidate.type)
  if (candidateRank < currentRank) return candidate
  if (candidateRank > currentRank) return current

  const currentArea = (current.width || 0) * (current.height || 0)
  const candidateArea = (candidate.width || 0) * (candidate.height || 0)
  if (candidateArea > currentArea) return candidate
  return current
}

function compareDisplayMedia(a: DisplayMediaItem, b: DisplayMediaItem): number {
  const rankDelta = mediaTypeRank(a.media.type) - mediaTypeRank(b.media.type)
  if (rankDelta !== 0) {
    return rankDelta
  }

  const areaA = (a.media.width || 0) * (a.media.height || 0)
  const areaB = (b.media.width || 0) * (b.media.height || 0)
  if (areaA !== areaB) {
    return areaB - areaA
  }

  return a.key.localeCompare(b.key)
}

export function mergeDisplayMedia(media: GameMediaDetailDTO[] | undefined): DisplayMediaItem[] {
  const merged = new Map<string, DisplayMediaItem>()

  for (const item of media ?? []) {
    const key = mediaIdentityKey(item)
    const existing = merged.get(key)
    if (!existing) {
      merged.set(key, {
        key,
        media: item,
        sources: uniqueStrings([item.source]),
        types: uniqueStrings([item.type]),
      })
      continue
    }

    existing.media = chooseRepresentativeMedia(existing.media, item)
    existing.sources = uniqueStrings([...existing.sources, item.source])
    existing.types = uniqueStrings([...existing.types, item.type])
  }

  return Array.from(merged.values())
}

export function isPreviewableDisplayMedia(item: DisplayMediaItem): boolean {
  const collection = new GameMediaCollection([item.media])
  return collection.isImage(item.media) || collection.isInlineVideo(item.media) || Boolean(youtubeEmbedUrl(item.media))
}

export function buildRepresentativeMediaPreview(items: DisplayMediaItem[]): DisplayMediaItem[] {
  const results: DisplayMediaItem[] = []
  const seen = new Set<string>()

  const add = (item: DisplayMediaItem | undefined) => {
    if (!item || seen.has(item.key) || !isPreviewableDisplayMedia(item)) return
    seen.add(item.key)
    results.push(item)
  }

  for (const type of FEATURED_TYPE_ORDER) {
    const match = items
      .filter((item) => normalizeFeaturedMediaType(item.media.type) === type)
      .sort(compareDisplayMedia)[0]
    add(match)
  }

  return results
}

export function buildFeaturedMediaRail(items: DisplayMediaItem[], maxItems: number): DisplayMediaItem[] {
  const results: DisplayMediaItem[] = []
  const seen = new Set<string>()

  const add = (item: DisplayMediaItem | undefined) => {
    if (!item || seen.has(item.key) || !isPreviewableDisplayMedia(item)) return
    seen.add(item.key)
    results.push(item)
  }

  for (const item of buildRepresentativeMediaPreview(items)) {
    add(item)
  }

  const remainder = items
    .filter((item) => isPreviewableDisplayMedia(item) && !seen.has(item.key))
    .sort(compareDisplayMedia)

  for (const item of remainder) {
    if (results.length >= maxItems) {
      break
    }
    add(item)
  }

  return results.slice(0, maxItems)
}
