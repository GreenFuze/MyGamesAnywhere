import type { GameMediaDetailDTO } from '@/api/client'

const HERO_MEDIA_TYPES = ['screenshot', 'background', 'banner', 'artwork', 'hero', 'cover']
const NON_IMAGE_MEDIA_TYPES = new Set(['video', 'trailer', 'manual', 'document', 'audio', 'soundtrack'])

function urlHasExtension(url: string, extensions: string[]): boolean {
  return extensions.some((extension) => url.toLowerCase().includes(extension))
}

export function mediaUrl(media: GameMediaDetailDTO): string {
  if (media.local_path) return `/api/media/${media.asset_id}`
  return media.url
}

export function youtubeEmbedUrl(media: GameMediaDetailDTO): string | null {
  try {
    const parsed = new URL(mediaUrl(media))
    const host = parsed.hostname.toLowerCase().replace(/^www\./, '')
    if (host === 'youtu.be') {
      const id = parsed.pathname.split('/').filter(Boolean)[0]
      return id ? `https://www.youtube.com/embed/${id}` : null
    }
    if (host === 'youtube.com' || host === 'm.youtube.com') {
      if (parsed.pathname === '/watch') {
        const id = parsed.searchParams.get('v')
        return id ? `https://www.youtube.com/embed/${id}` : null
      }
      if (parsed.pathname.startsWith('/embed/')) {
        return parsed.toString()
      }
    }
  } catch {
    return null
  }
  return null
}

export class GameMediaCollection {
  private readonly media: GameMediaDetailDTO[]

  constructor(media: GameMediaDetailDTO[] | undefined) {
    this.media = Array.isArray(media) ? media.filter(Boolean) : []
  }

  all(): GameMediaDetailDTO[] {
    return this.media
  }

  imageMedia(): GameMediaDetailDTO[] {
    return this.media.filter((media) => this.isImage(media))
  }

  nonImageMedia(): GameMediaDetailDTO[] {
    return this.media.filter((media) => !this.isImage(media))
  }

  cover(): GameMediaDetailDTO | null {
    const imageMedia = this.imageMedia()
    return imageMedia.find((item) => item.type === 'cover') ?? imageMedia[0] ?? null
  }

  coverUrl(): string | null {
    return this.urlFor(this.cover())
  }

  hero(): GameMediaDetailDTO | null {
    const imageMedia = this.imageMedia()
    for (const type of HERO_MEDIA_TYPES) {
      const match = imageMedia.find((item) => item.type === type)
      if (match) return match
    }
    return imageMedia[0] ?? null
  }

  heroUrl(): string | null {
    return this.urlFor(this.hero())
  }

  isImage(media: GameMediaDetailDTO): boolean {
    if (media.mime_type?.startsWith('image/')) return true
    if (media.mime_type?.startsWith('video/') || media.mime_type?.startsWith('audio/')) return false
    return !NON_IMAGE_MEDIA_TYPES.has(media.type)
  }

  isInlineVideo(media: GameMediaDetailDTO): boolean {
    if (media.mime_type?.startsWith('video/')) return true
    return urlHasExtension(mediaUrl(media), ['.mp4', '.webm', '.ogg', '.mov'])
  }

  isInlineAudio(media: GameMediaDetailDTO): boolean {
    if (media.mime_type?.startsWith('audio/')) return true
    return urlHasExtension(mediaUrl(media), ['.mp3', '.wav', '.ogg', '.m4a', '.flac'])
  }

  isPdf(media: GameMediaDetailDTO): boolean {
    const source = media.local_path ?? media.url
    return media.mime_type === 'application/pdf' || source.toLowerCase().endsWith('.pdf')
  }

  isInlineDocument(media: GameMediaDetailDTO): boolean {
    const source = (media.local_path ?? media.url).toLowerCase()
    const mime = media.mime_type?.toLowerCase() ?? ''
    if (mime.startsWith('text/')) return true
    if (mime === 'application/json' || mime === 'application/xml') return true
    return urlHasExtension(source, ['.txt', '.md', '.markdown', '.html', '.htm', '.json', '.xml', '.csv'])
  }

  private urlFor(media: GameMediaDetailDTO | null): string | null {
    if (!media) return null
    return mediaUrl(media)
  }
}
