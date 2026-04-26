import { useEffect, useMemo, useState } from 'react'
import { useQuery, useQueryClient } from '@tanstack/react-query'
import { ArrowLeft, ExternalLink, Image as ImageIcon, Video } from 'lucide-react'
import { useLocation, useNavigate, useParams } from 'react-router-dom'
import {
  ApiError,
  getGame,
  setGameBackgroundOverride,
  setGameCoverOverride,
  setGameHoverOverride,
  type GameMediaDetailDTO,
} from '@/api/client'
import { BrandBadge } from '@/components/ui/brand-icon'
import { Badge } from '@/components/ui/badge'
import { Button } from '@/components/ui/button'
import { Dialog } from '@/components/ui/dialog'
import { mediaUrl, GameMediaCollection, youtubeEmbedUrl } from '@/lib/gameMedia'
import { buildRepresentativeMediaPreview, mergeDisplayMedia, type DisplayMediaItem } from '@/lib/gameMediaDisplay'
import { brandLabel } from '@/lib/brands'

function mediaTypeLabel(type: string): string {
  if (!type) return 'Media'
  return type
    .split(/[_-]/g)
    .filter(Boolean)
    .map((part) => part.charAt(0).toUpperCase() + part.slice(1))
    .join(' ')
}

function MediaViewerDialog({ media, onClose }: { media: GameMediaDetailDTO | null; onClose: () => void }) {
  if (!media) return null
  const url = mediaUrl(media)
  const youtubeUrl = youtubeEmbedUrl(media)
  const mediaCollection = new GameMediaCollection([media])

  return (
    <Dialog open={media !== null} onClose={onClose} title={mediaTypeLabel(media.type)} className="max-w-6xl">
      <div className="space-y-4">
        <div className="overflow-hidden rounded-[24px] border border-white/10 bg-[#0a0e15] p-4">
          {mediaCollection.isImage(media) ? (
            <img src={url} alt={mediaTypeLabel(media.type)} className="max-h-[76vh] w-full object-contain" />
          ) : youtubeUrl ? (
            <iframe
              src={youtubeUrl}
              title={mediaTypeLabel(media.type)}
              allow="accelerometer; autoplay; clipboard-write; encrypted-media; gyroscope; picture-in-picture"
              allowFullScreen
              className="aspect-video w-full rounded-[18px] border border-white/10 bg-black"
            />
          ) : mediaCollection.isInlineVideo(media) ? (
            <video controls preload="metadata" className="max-h-[76vh] w-full rounded-[18px] bg-black object-contain">
              <source src={url} type={media.mime_type} />
            </video>
          ) : (
            <div className="flex h-40 items-center justify-center text-sm text-white/58">
              This media cannot be previewed inline.
            </div>
          )}
        </div>
        <div className="flex flex-wrap items-center gap-2 text-sm text-white/60">
          <Badge>{mediaTypeLabel(media.type)}</Badge>
          {media.source ? <BrandBadge brand={media.source} label={brandLabel(media.source, media.source)} /> : null}
          <a href={url} target="_blank" rel="noreferrer" className="inline-flex items-center gap-1 text-mga-accent hover:underline">
            Open original
            <ExternalLink size={14} />
          </a>
        </div>
      </div>
    </Dialog>
  )
}

function SourcePill({ source }: { source: string }) {
  return <BrandBadge brand={source} label={brandLabel(source, source)} className="bg-white/5 text-white" />
}

function MediaGalleryCard({
  item,
  currentCoverAssetId,
  currentHoverAssetId,
  currentBackgroundAssetId,
  busyCover,
  busyHover,
  busyBackground,
  onOpen,
  onSetCover,
  onSetHover,
  onSetBackground,
}: {
  item: DisplayMediaItem
  currentCoverAssetId?: number
  currentHoverAssetId?: number
  currentBackgroundAssetId?: number
  busyCover: boolean
  busyHover: boolean
  busyBackground: boolean
  onOpen: (media: GameMediaDetailDTO) => void
  onSetCover: (media: GameMediaDetailDTO) => void
  onSetHover: (media: GameMediaDetailDTO) => void
  onSetBackground: (media: GameMediaDetailDTO) => void
}) {
  const media = item.media
  const mediaCollection = new GameMediaCollection([media])
  const isImage = mediaCollection.isImage(media)
  const isVideo = !isImage && (Boolean(youtubeEmbedUrl(media)) || mediaCollection.isInlineVideo(media))
  const isCurrentCover = currentCoverAssetId === media.asset_id
  const isCurrentHover = currentHoverAssetId === media.asset_id
  const isCurrentBackground = currentBackgroundAssetId === media.asset_id
  const lowResolutionBackground = Boolean(media.width && media.height && (media.width < 1600 || media.height < 900))

  return (
    <article className="overflow-hidden rounded-[22px] border border-white/8 bg-[#0f141f] shadow-[0_16px_34px_rgba(0,0,0,0.18)]">
      <button
        type="button"
        onClick={() => onOpen(media)}
        className="group block w-full text-left"
      >
        {isImage ? (
          <div className="aspect-video w-full bg-[#090d14] p-2">
            <img
              src={mediaUrl(media)}
              alt={item.types.map(mediaTypeLabel).join(', ')}
              loading="lazy"
              decoding="async"
              className="h-full w-full object-contain transition-transform duration-200 group-hover:scale-[1.02]"
            />
          </div>
        ) : (
          <div className="flex aspect-video w-full items-center justify-center bg-[radial-gradient(circle_at_top,rgba(255,255,255,0.14),transparent_55%),linear-gradient(180deg,rgba(14,18,28,0.98),rgba(8,10,16,0.98))]">
            <div className="rounded-full bg-black/45 p-3 text-white/82">
              <Video size={18} />
            </div>
          </div>
        )}
      </button>
      <div className="space-y-3 border-t border-white/8 px-4 py-4">
        <div className="space-y-2">
          <p className="text-sm font-medium text-white">{item.types.map(mediaTypeLabel).join(' • ')}</p>
          <div className="flex flex-wrap gap-2">
            {item.sources.map((source) => (
              <SourcePill key={`${item.key}:${source}`} source={source} />
            ))}
          </div>
        </div>
        {isImage ? (
          <div className="flex flex-wrap gap-2">
            <Button
              type="button"
              size="sm"
              variant="outline"
              disabled={busyCover || isCurrentCover}
              onClick={() => onSetCover(media)}
            >
              {isCurrentCover ? 'Current cover' : busyCover ? 'Setting cover...' : 'Set as cover'}
            </Button>
            <Button
              type="button"
              size="sm"
              variant="outline"
              disabled={busyHover || isCurrentHover}
              onClick={() => onSetHover(media)}
            >
              {isCurrentHover ? 'Current hover' : busyHover ? 'Setting hover...' : 'Set as hover'}
            </Button>
            <Button
              type="button"
              size="sm"
              variant="outline"
              disabled={busyBackground || isCurrentBackground}
              onClick={() => onSetBackground(media)}
              className={lowResolutionBackground && !isCurrentBackground ? 'border-amber-500/50 text-amber-300 hover:bg-amber-500/10' : undefined}
            >
              {isCurrentBackground ? 'Current background' : busyBackground ? 'Setting background...' : 'Set as background'}
            </Button>
          </div>
        ) : (
          <div className="text-xs text-white/52">{isVideo ? 'Video preview' : 'Media preview'}</div>
        )}
      </div>
    </article>
  )
}

export function GameMediaPage() {
  const navigate = useNavigate()
  const location = useLocation()
  const { id = '' } = useParams()
  const queryClient = useQueryClient()
  const [selectedMedia, setSelectedMedia] = useState<GameMediaDetailDTO | null>(null)
  const [selectedSources, setSelectedSources] = useState<string[]>([])
  const [selectedTypes, setSelectedTypes] = useState<string[]>([])
  const [coverBusy, setCoverBusy] = useState(false)
  const [hoverBusy, setHoverBusy] = useState(false)
  const [backgroundBusy, setBackgroundBusy] = useState(false)
  const [errorMessage, setErrorMessage] = useState('')

  const game = useQuery({
    queryKey: ['game', id],
    queryFn: () => getGame(id),
    enabled: id.length > 0,
  })

  const mergedMedia = useMemo(() => mergeDisplayMedia(game.data?.media), [game.data?.media])
  const availableSources = useMemo(
    () => Array.from(new Set(mergedMedia.flatMap((item) => item.sources))).sort((a, b) => brandLabel(a, a).localeCompare(brandLabel(b, b))),
    [mergedMedia],
  )
  const availableTypes = useMemo(
    () => Array.from(new Set(mergedMedia.flatMap((item) => item.types))).sort((a, b) => mediaTypeLabel(a).localeCompare(mediaTypeLabel(b))),
    [mergedMedia],
  )
  const representativePreview = useMemo(
    () => buildRepresentativeMediaPreview(mergedMedia, game.data?.cover_override, game.data?.hover_override),
    [game.data?.cover_override, game.data?.hover_override, mergedMedia],
  )

  useEffect(() => {
    setSelectedSources((current) => (current.length > 0 ? current.filter((item) => availableSources.includes(item)) : availableSources))
  }, [availableSources])

  useEffect(() => {
    setSelectedTypes((current) => (current.length > 0 ? current.filter((item) => availableTypes.includes(item)) : availableTypes))
  }, [availableTypes])

  const filteredMedia = useMemo(
    () =>
      mergedMedia.filter(
        (item) =>
          item.sources.some((source) => selectedSources.includes(source)) &&
          item.types.some((type) => selectedTypes.includes(type)),
      ),
    [mergedMedia, selectedSources, selectedTypes],
  )

  const handleSetCover = async (media: GameMediaDetailDTO) => {
    if (!game.data || coverBusy) return
    setErrorMessage('')
    setCoverBusy(true)
    try {
      const updated = await setGameCoverOverride(game.data.id, media.asset_id)
      queryClient.setQueryData(['game', updated.id], updated)
      await queryClient.invalidateQueries({ queryKey: ['games'] })
    } catch (error) {
      setErrorMessage(
        error instanceof ApiError ? error.responseText?.trim() || error.message : error instanceof Error ? error.message : 'Set cover failed.',
      )
    } finally {
      setCoverBusy(false)
    }
  }

  const handleSetHover = async (media: GameMediaDetailDTO) => {
    if (!game.data || hoverBusy) return
    setErrorMessage('')
    setHoverBusy(true)
    try {
      const updated = await setGameHoverOverride(game.data.id, media.asset_id)
      queryClient.setQueryData(['game', updated.id], updated)
      await queryClient.invalidateQueries({ queryKey: ['games'] })
    } catch (error) {
      setErrorMessage(
        error instanceof ApiError ? error.responseText?.trim() || error.message : error instanceof Error ? error.message : 'Set hover failed.',
      )
    } finally {
      setHoverBusy(false)
    }
  }

  const handleSetBackground = async (media: GameMediaDetailDTO) => {
    if (!game.data || backgroundBusy) return
    const lowResolution = Boolean(media.width && media.height && (media.width < 1600 || media.height < 900))
    if (lowResolution) {
      const proceed = window.confirm(
        `This image is ${media.width}x${media.height}, which is likely too low-resolution for a hero background. Continue anyway?`,
      )
      if (!proceed) return
    }
    setErrorMessage('')
    setBackgroundBusy(true)
    try {
      const updated = await setGameBackgroundOverride(game.data.id, media.asset_id)
      queryClient.setQueryData(['game', updated.id], updated)
      await queryClient.invalidateQueries({ queryKey: ['games'] })
    } catch (error) {
      setErrorMessage(
        error instanceof ApiError ? error.responseText?.trim() || error.message : error instanceof Error ? error.message : 'Set background failed.',
      )
    } finally {
      setBackgroundBusy(false)
    }
  }

  if (game.isPending) {
    return <div className="mx-auto max-w-[1540px] p-6 text-sm text-white/60">Loading media gallery...</div>
  }

  if (game.isError || !game.data) {
    return <div className="mx-auto max-w-[1540px] p-6 text-sm text-red-400">{game.isError ? game.error.message : 'Game not found.'}</div>
  }

  const data = game.data
  return (
    <div className="mx-auto max-w-[1540px] space-y-6 p-4 md:p-6 lg:p-8">
      <div className="flex flex-wrap items-center justify-between gap-3">
        <Button type="button" variant="outline" size="sm" onClick={() => navigate(`/game/${encodeURIComponent(data.id)}`, { state: location.state })}>
          <ArrowLeft size={14} />
          Back to game
        </Button>
        <div className="flex flex-wrap gap-2 text-xs text-white/56">
          <span className="inline-flex items-center rounded-full border border-white/8 bg-white/5 px-3 py-1.5">
            {filteredMedia.length} filtered item{filteredMedia.length === 1 ? '' : 's'}
          </span>
          <span className="inline-flex items-center rounded-full border border-white/8 bg-white/5 px-3 py-1.5">
            {representativePreview.length} preview type{representativePreview.length === 1 ? '' : 's'}
          </span>
        </div>
      </div>

      <section className="rounded-[30px] border border-white/8 bg-[#0b111b] p-6 shadow-[0_28px_72px_rgba(0,0,0,0.28)]">
        <div className="space-y-3">
          <div className="flex items-center gap-2">
            <ImageIcon size={18} className="text-mga-accent" />
            <h1 className="text-3xl font-semibold text-white">{data.title} Media Gallery</h1>
          </div>
          <p className="text-sm leading-7 text-white/66">
            Filter by source and media type, then choose which image should be used as the cover, hover preview, or hero background.
          </p>
        </div>
      </section>

      <div className="grid gap-6 xl:grid-cols-[280px_minmax(0,1fr)]">
        <aside className="space-y-6 rounded-[24px] border border-white/8 bg-[#101622] p-5 shadow-[0_18px_40px_rgba(0,0,0,0.18)]">
          <div className="space-y-3">
            <h2 className="text-sm font-semibold uppercase tracking-[0.18em] text-white/54">Sources</h2>
            <div className="space-y-2">
              {availableSources.map((source) => (
                <label key={source} className="flex items-center gap-3 text-sm text-white/82">
                  <input
                    type="checkbox"
                    className="h-4 w-4 rounded border-white/20 bg-transparent"
                    checked={selectedSources.includes(source)}
                    onChange={() =>
                      setSelectedSources((current) =>
                        current.includes(source) ? current.filter((item) => item !== source) : [...current, source],
                      )
                    }
                  />
                  <SourcePill source={source} />
                </label>
              ))}
            </div>
          </div>
          <div className="space-y-3">
            <h2 className="text-sm font-semibold uppercase tracking-[0.18em] text-white/54">Media Types</h2>
            <div className="space-y-2">
              {availableTypes.map((type) => (
                <label key={type} className="flex items-center gap-3 text-sm text-white/82">
                  <input
                    type="checkbox"
                    className="h-4 w-4 rounded border-white/20 bg-transparent"
                    checked={selectedTypes.includes(type)}
                    onChange={() =>
                      setSelectedTypes((current) =>
                        current.includes(type) ? current.filter((item) => item !== type) : [...current, type],
                      )
                    }
                  />
                  <span>{mediaTypeLabel(type)}</span>
                </label>
              ))}
            </div>
          </div>
        </aside>

        <section className="space-y-4">
          {errorMessage ? <p className="text-sm text-red-400">{errorMessage}</p> : null}
          {filteredMedia.length === 0 ? (
            <div className="rounded-[24px] border border-white/8 bg-[#101622] p-6 text-sm text-white/58">
              No media matches the current filters.
            </div>
          ) : (
            <div className="grid gap-4 md:grid-cols-2 xl:grid-cols-3">
              {filteredMedia.map((item) => (
                <MediaGalleryCard
                  key={item.key}
                  item={item}
                  currentCoverAssetId={data.cover_override?.asset_id}
                  currentHoverAssetId={data.hover_override?.asset_id}
                  currentBackgroundAssetId={data.background_override?.asset_id}
                  busyCover={coverBusy}
                  busyHover={hoverBusy}
                  busyBackground={backgroundBusy}
                  onOpen={setSelectedMedia}
                  onSetCover={handleSetCover}
                  onSetHover={handleSetHover}
                  onSetBackground={handleSetBackground}
                />
              ))}
            </div>
          )}
        </section>
      </div>

      <MediaViewerDialog media={selectedMedia} onClose={() => setSelectedMedia(null)} />
    </div>
  )
}
