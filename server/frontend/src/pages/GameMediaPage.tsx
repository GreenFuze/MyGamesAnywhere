import { useCallback, useEffect, useMemo, useState } from 'react'
import { useQuery, useQueryClient } from '@tanstack/react-query'
import { AlertTriangle, ArrowLeft, ExternalLink, Image as ImageIcon, Video } from 'lucide-react'
import { useLocation, useNavigate, useParams } from 'react-router-dom'
import {
  ApiError,
  getGame,
  setGameBackgroundOverride,
  setGameCoverOverride,
  setGameHoverOverride,
  updateMediaAssetMetadata,
  type GameMediaDetailDTO,
  type GameDetailResponse,
} from '@/api/client'
import { BrandBadge } from '@/components/ui/brand-icon'
import { Badge } from '@/components/ui/badge'
import { Button } from '@/components/ui/button'
import { Dialog } from '@/components/ui/dialog'
import { evaluateBackgroundSuitability, formatBackgroundSuitabilityMessage, type BackgroundSuitability } from '@/lib/backgroundSuitability'
import { mediaUrl, GameMediaCollection, youtubeEmbedUrl } from '@/lib/gameMedia'
import { buildRepresentativeMediaPreview, mergeDisplayMedia, type DisplayMediaItem } from '@/lib/gameMediaDisplay'
import { brandLabel } from '@/lib/brands'

function patchGameMediaDimensions(data: GameDetailResponse, assetId: number, width: number, height: number): GameDetailResponse {
  return {
    ...data,
    media: (data.media ?? []).map((media) =>
      media.asset_id === assetId
        ? { ...media, width, height, mime_type: media.mime_type || 'image/*' }
        : media,
    ),
    cover_override:
      data.cover_override?.asset_id === assetId ? { ...data.cover_override, width, height, mime_type: data.cover_override.mime_type || 'image/*' } : data.cover_override,
    hover_override:
      data.hover_override?.asset_id === assetId ? { ...data.hover_override, width, height, mime_type: data.hover_override.mime_type || 'image/*' } : data.hover_override,
    background_override:
      data.background_override?.asset_id === assetId
        ? { ...data.background_override, width, height, mime_type: data.background_override.mime_type || 'image/*' }
        : data.background_override,
  }
}

function probeImageDimensions(url: string): Promise<{ width: number; height: number }> {
  return new Promise((resolve, reject) => {
    const img = new Image()
    img.onload = () => resolve({ width: img.naturalWidth, height: img.naturalHeight })
    img.onerror = () => reject(new Error('Image probe failed'))
    img.src = url
  })
}

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

function BackgroundWarningIndicator({ suitability }: { suitability: BackgroundSuitability }) {
  const message = suitability.reasons.length > 0
    ? suitability.reasons.join(' ')
    : 'This image may not fit the hero background well.'

  return (
    <span className="group/warn relative inline-flex items-center">
      <span
        tabIndex={0}
        role="img"
        aria-label={message}
        className="inline-flex items-center text-amber-300 outline-none"
      >
        <AlertTriangle size={14} />
      </span>
      <span className="pointer-events-none absolute bottom-[calc(100%+8px)] left-1/2 z-20 hidden w-64 -translate-x-1/2 rounded-[12px] border border-amber-500/30 bg-[#18121f] px-3 py-2 text-xs leading-5 text-white shadow-[0_18px_34px_rgba(0,0,0,0.34)] group-hover/warn:block group-focus-within/warn:block">
        {message}
      </span>
    </span>
  )
}

function MediaGalleryCard({
  item,
  currentCoverAssetId,
  currentHoverAssetId,
  currentBackgroundAssetId,
  backgroundProbePending,
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
  backgroundProbePending: boolean
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
  const backgroundSuitability = evaluateBackgroundSuitability(media)
  const backgroundWarning = !backgroundProbePending && backgroundSuitability.level !== 'good'

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
            >
              <span className="inline-flex items-center gap-2">
                <span>{isCurrentBackground ? 'Current background' : busyBackground ? 'Setting background...' : 'Set as background'}</span>
                {backgroundWarning && !isCurrentBackground ? <BackgroundWarningIndicator suitability={backgroundSuitability} /> : null}
              </span>
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
  const [backgroundWarningState, setBackgroundWarningState] = useState<{ media: GameMediaDetailDTO; suitability: BackgroundSuitability } | null>(null)
  const [backgroundWarningProbeBusy, setBackgroundWarningProbeBusy] = useState(false)
  const [probedAssetIds, setProbedAssetIds] = useState<Set<number>>(new Set())
  const [probingAssetIds, setProbingAssetIds] = useState<Set<number>>(new Set())

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
  const representativePreview = useMemo(() => buildRepresentativeMediaPreview(mergedMedia), [mergedMedia])

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

  const ensureMediaDimensions = useCallback(async (media: GameMediaDetailDTO): Promise<GameMediaDetailDTO> => {
    if (!game.data) return media
    if ((media.width ?? 0) > 0 && (media.height ?? 0) > 0) {
      return media
    }

    // Probe the original media asset URL, not the rendered thumbnail surface.
    const dims = await probeImageDimensions(mediaUrl(media))
    if (dims.width <= 0 || dims.height <= 0) {
      return media
    }

    const patched = {
      ...media,
      width: dims.width,
      height: dims.height,
      mime_type: media.mime_type || 'image/*',
    }

    queryClient.setQueryData<GameDetailResponse>(['game', game.data.id], (current) =>
      current ? patchGameMediaDimensions(current, media.asset_id, dims.width, dims.height) : current,
    )

    try {
      await updateMediaAssetMetadata(media.asset_id, {
        width: dims.width,
        height: dims.height,
        mime_type: media.mime_type,
      })
    } catch {
      // Keep local dimensions even if persistence fails.
    }

    return patched
  }, [game.data, queryClient])

  useEffect(() => {
    if (!game.data) return

    const candidates = mergedMedia
      .map((item) => item.media)
      .filter((media) => {
        if (probedAssetIds.has(media.asset_id) || probingAssetIds.has(media.asset_id)) return false
        if ((media.width ?? 0) > 0 && (media.height ?? 0) > 0) return false
        return new GameMediaCollection([media]).isImage(media)
      })

    if (candidates.length === 0) return
    let cancelled = false

    void (async () => {
      for (const media of candidates) {
        if (cancelled) return
        setProbingAssetIds((current) => new Set(current).add(media.asset_id))
        try {
          await ensureMediaDimensions(media)
        } catch {
          // Keep unknown dimensions; click-time probe remains the fallback.
        } finally {
          if (!cancelled) {
            setProbingAssetIds((current) => {
              const next = new Set(current)
              next.delete(media.asset_id)
              return next
            })
            setProbedAssetIds((current) => new Set(current).add(media.asset_id))
          }
        }
      }
    })()

    return () => {
      cancelled = true
    }
  }, [ensureMediaDimensions, game.data, mergedMedia, probedAssetIds, probingAssetIds])

  useEffect(() => {
    if (!backgroundWarningState) {
      setBackgroundWarningProbeBusy(false)
      return
    }
    if ((backgroundWarningState.suitability.actual.width ?? 0) > 0 && (backgroundWarningState.suitability.actual.height ?? 0) > 0) {
      setBackgroundWarningProbeBusy(false)
      return
    }

    let cancelled = false
    setBackgroundWarningProbeBusy(true)

    void (async () => {
      try {
        const resolvedMedia = await ensureMediaDimensions(backgroundWarningState.media)
        if (cancelled) return
        setBackgroundWarningState({
          media: resolvedMedia,
          suitability: evaluateBackgroundSuitability(resolvedMedia),
        })
      } catch {
        if (!cancelled) {
          setBackgroundWarningProbeBusy(false)
        }
        return
      }
      if (!cancelled) {
        setBackgroundWarningProbeBusy(false)
      }
    })()

    return () => {
      cancelled = true
    }
  }, [backgroundWarningState, ensureMediaDimensions])

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

  const applyBackgroundOverride = async (media: GameMediaDetailDTO) => {
    if (!game.data || backgroundBusy) return
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

  const handleSetBackground = async (media: GameMediaDetailDTO) => {
    const suitability = evaluateBackgroundSuitability(media)
    const resolvedMedia = (suitability.actual.width > 0 && suitability.actual.height > 0) ? media : await ensureMediaDimensions(media)
    const resolvedSuitability = evaluateBackgroundSuitability(resolvedMedia)
    if (resolvedSuitability.level !== 'good') {
      setBackgroundWarningState({ media: resolvedMedia, suitability: resolvedSuitability })
      return
    }
    await applyBackgroundOverride(resolvedMedia)
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
                  backgroundProbePending={probingAssetIds.has(item.media.asset_id)}
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
      <Dialog
        open={backgroundWarningState !== null}
        onClose={() => setBackgroundWarningState(null)}
        title="Background warning"
        className="max-w-xl"
      >
        {backgroundWarningState ? (
          <div className="space-y-5">
            {backgroundWarningProbeBusy ? (
              <p className="text-xs text-white/54">Checking original image dimensions...</p>
            ) : null}
            <p className="text-sm leading-7 text-white/74 whitespace-pre-line">
              {formatBackgroundSuitabilityMessage(backgroundWarningState.suitability)}
            </p>
            <div className="flex justify-end gap-3">
              <Button type="button" variant="outline" onClick={() => setBackgroundWarningState(null)}>
                Cancel
              </Button>
              <Button
                type="button"
                className="bg-amber-500/20 text-amber-200 hover:bg-amber-500/30"
                onClick={async () => {
                  const pending = backgroundWarningState
                  setBackgroundWarningState(null)
                  await applyBackgroundOverride(pending.media)
                }}
              >
                Use Anyway
              </Button>
            </div>
          </div>
        ) : null}
      </Dialog>
    </div>
  )
}
