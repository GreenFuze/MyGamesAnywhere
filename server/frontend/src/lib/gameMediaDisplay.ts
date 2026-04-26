import type { GameMediaDetailDTO } from '@/api/client'
import { GameMediaCollection, youtubeEmbedUrl } from '@/lib/gameMedia'

export interface DisplayMediaItem {
  key: string
  media: GameMediaDetailDTO
  sources: string[]
  types: string[]
}

const DISPLAY_TYPE_PRIORITY = [
  'cover',
  'hero',
  'background',
  'backdrop',
  'screenshot',
  'artwork',
  'banner',
  'logo',
  'video',
  'trailer',
] as const

const REPRESENTATIVE_GROUPS = [
  ['cover'],
  ['screenshot'],
  ['background', 'backdrop'],
  ['artwork', 'banner', 'logo', 'hero'],
  ['video', 'trailer'],
] as const

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

function mediaTypeRank(type: string): number {
  const idx = DISPLAY_TYPE_PRIORITY.indexOf(type as (typeof DISPLAY_TYPE_PRIORITY)[number])
  return idx === -1 ? DISPLAY_TYPE_PRIORITY.length : idx
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

export function buildRepresentativeMediaPreview(
  items: DisplayMediaItem[],
  coverOverride?: GameMediaDetailDTO,
  hoverOverride?: GameMediaDetailDTO,
): DisplayMediaItem[] {
  const results: DisplayMediaItem[] = []
  const seen = new Set<string>()

  const add = (item: DisplayMediaItem | undefined) => {
    if (!item || seen.has(item.key) || !isPreviewableDisplayMedia(item)) return
    seen.add(item.key)
    results.push(item)
  }

  const byIdentity = new Map(items.map((item) => [item.key, item]))
  if (coverOverride) add(byIdentity.get(mediaIdentityKey(coverOverride)))
  if (hoverOverride) add(byIdentity.get(mediaIdentityKey(hoverOverride)))

  for (const group of REPRESENTATIVE_GROUPS) {
    const match = items.find((item) => item.types.some((type) => group.includes(type as never)))
    add(match)
  }

  return results
}
