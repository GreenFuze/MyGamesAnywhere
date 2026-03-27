import { useEffect, useMemo, useRef, useState, type ReactNode } from 'react'
import { useQuery, useQueryClient } from '@tanstack/react-query'
import {
  ArrowLeft,
  Database,
  ExternalLink,
  FileText,
  FolderOpen,
  HardDrive,
  Image as ImageIcon,
  PlayCircle,
  Trophy,
  Video,
} from 'lucide-react'
import { useLocation, useNavigate, useParams } from 'react-router-dom'
import {
  ApiError,
  getGame,
  getGameAchievements,
  type AchievementDTO,
  type AchievementSetDTO,
  type ExternalIDDTO,
  type GameFileDTO,
  type GameMediaDetailDTO,
  type ResolverMatchDTO,
  type SourceGameDetailDTO,
} from '@/api/client'
import { useRecentPlayed } from '@/hooks/useRecentPlayed'
import { BrandBadge, BrandIcon } from '@/components/ui/brand-icon'
import { Badge } from '@/components/ui/badge'
import { Button } from '@/components/ui/button'
import { CoverImage } from '@/components/ui/cover-image'
import { Dialog } from '@/components/ui/dialog'
import { PlatformIcon } from '@/components/ui/platform-icon'
import { ProgressBar } from '@/components/ui/progress-bar'
import {
  brandLabel,
  resolveBrandDefinition,
  resolveBrandDefinitionFromUrl,
} from '@/lib/brands'
import { inferOriginLabel, readGameRouteState } from '@/lib/gameNavigation'
import {
  formatHLTB,
  isPlayable,
  pluginLabel,
  resolverMatchCount,
  selectSourcePlugins,
} from '@/lib/gameUtils'
import { cn } from '@/lib/utils'

const HERO_MEDIA_TYPES = ['screenshot', 'background', 'banner', 'artwork', 'hero', 'cover']

type MetadataField =
  | 'title'
  | 'description'
  | 'release_date'
  | 'developer'
  | 'publisher'
  | 'genres'
  | 'rating'
  | 'max_players'

type ExternalLinkItem = {
  id: string
  label: string
  url: string
  subtitle: string
  source: string
  host: string
  actionLabel: string
  brandId?: string
}

function hasTextValue(value: string | undefined): boolean {
  return typeof value === 'string' && value.trim().length > 0
}

function hasNumberValue(value: number | undefined): boolean {
  return typeof value === 'number' && Number.isFinite(value) && value > 0
}

function formatDateValue(value: string | undefined): string {
  if (!value) return 'Unknown'
  const date = new Date(value)
  if (Number.isNaN(date.getTime())) return value
  return new Intl.DateTimeFormat(undefined, { dateStyle: 'medium' }).format(date)
}

function formatDateTimeValue(value: string | undefined): string {
  if (!value) return 'Unknown'
  const date = new Date(value)
  if (Number.isNaN(date.getTime())) return value
  return new Intl.DateTimeFormat(undefined, { dateStyle: 'medium', timeStyle: 'short' }).format(date)
}

function formatHours(value: number | undefined): string {
  if (value === undefined || value <= 0) return 'Unknown'
  return `${Math.round(value)}h`
}

function formatBytes(bytes: number): string {
  if (bytes <= 0) return '0 B'
  const units = ['B', 'KB', 'MB', 'GB', 'TB']
  const exponent = Math.min(Math.floor(Math.log(bytes) / Math.log(1024)), units.length - 1)
  const amount = bytes / 1024 ** exponent
  const digits = amount >= 10 || exponent === 0 ? 0 : 1
  return `${amount.toFixed(digits)} ${units[exponent]}`
}

function formatHostname(url: string): string {
  try {
    return new URL(url).hostname.toLowerCase().replace(/^www\./, '')
  } catch {
    return 'external link'
  }
}

function mediaTypeLabel(type: string): string {
  if (!type) return 'Media'
  return type
    .split(/[_-]/g)
    .filter(Boolean)
    .map((part) => part.charAt(0).toUpperCase() + part.slice(1))
    .join(' ')
}

function urlHasExtension(url: string, extensions: string[]): boolean {
  return extensions.some((extension) => url.toLowerCase().includes(extension))
}

function mediaUrl(media: GameMediaDetailDTO): string {
  if (media.local_path) return `/api/media/${media.asset_id}`
  return media.url
}

function isImageMedia(media: GameMediaDetailDTO): boolean {
  if (media.mime_type?.startsWith('image/')) return true
  if (media.mime_type?.startsWith('video/') || media.mime_type?.startsWith('audio/')) return false
  return !['video', 'trailer', 'manual', 'document', 'audio', 'soundtrack'].includes(media.type)
}

function isInlineVideoMedia(media: GameMediaDetailDTO): boolean {
  if (media.mime_type?.startsWith('video/')) return true
  return urlHasExtension(mediaUrl(media), ['.mp4', '.webm', '.ogg', '.mov'])
}

function isInlineAudioMedia(media: GameMediaDetailDTO): boolean {
  if (media.mime_type?.startsWith('audio/')) return true
  return urlHasExtension(mediaUrl(media), ['.mp3', '.wav', '.ogg', '.m4a', '.flac'])
}

function isPdfMedia(media: GameMediaDetailDTO): boolean {
  const source = media.local_path ?? media.url
  return media.mime_type === 'application/pdf' || source.toLowerCase().endsWith('.pdf')
}

function selectCoverMedia(media: GameMediaDetailDTO[]): GameMediaDetailDTO | null {
  return media.find((item) => item.type === 'cover') ?? media[0] ?? null
}

function selectHeroMedia(media: GameMediaDetailDTO[]): GameMediaDetailDTO | null {
  for (const type of HERO_MEDIA_TYPES) {
    const match = media.find((item) => item.type === type)
    if (match) return match
  }
  return media[0] ?? null
}

function buildExternalLinks(externalIds: ExternalIDDTO[] | undefined): ExternalLinkItem[] {
  if (!externalIds) return []

  const links = externalIds
    .filter((externalId) => typeof externalId.url === 'string' && externalId.url.length > 0)
    .map((externalId) => {
      const brand =
        resolveBrandDefinition(externalId.source) ?? resolveBrandDefinitionFromUrl(externalId.url!)
      const label = brand?.label ?? brandLabel(externalId.source, formatHostname(externalId.url!))
      return {
        id: `${externalId.source}:${externalId.external_id}`,
        label,
        url: externalId.url!,
        subtitle: externalId.external_id,
        source: externalId.source,
        host: formatHostname(externalId.url!),
        actionLabel: brand ? `Open in ${brand.label}` : 'Open external link',
        brandId: brand?.id,
      }
    })
    .sort((a, b) => a.label.localeCompare(b.label) || a.host.localeCompare(b.host))

  return Array.from(new Map(links.map((link) => [link.id, link])).values())
}

function achievementProgress(set: AchievementSetDTO): number {
  if (set.total_count <= 0) return 0
  return (set.unlocked_count / set.total_count) * 100
}

function summarizeAchievements(sets: AchievementSetDTO[]) {
  return sets.reduce(
    (summary, set) => ({
      totalCount: summary.totalCount + set.total_count,
      unlockedCount: summary.unlockedCount + set.unlocked_count,
      totalPoints: summary.totalPoints + (set.total_points ?? 0),
      earnedPoints: summary.earnedPoints + (set.earned_points ?? 0),
    }),
    { totalCount: 0, unlockedCount: 0, totalPoints: 0, earnedPoints: 0 },
  )
}

function detailValue(value: string | number | undefined | null): string {
  if (value === null || value === undefined || value === '') return 'Unknown'
  return String(value)
}

function isMetadataPlugin(pluginId: string): boolean {
  return pluginId.startsWith('metadata-')
}

function resolverHasField(match: ResolverMatchDTO, field: MetadataField): boolean {
  switch (field) {
    case 'title':
      return hasTextValue(match.title)
    case 'description':
      return hasTextValue(match.description)
    case 'release_date':
      return hasTextValue(match.release_date)
    case 'developer':
      return hasTextValue(match.developer)
    case 'publisher':
      return hasTextValue(match.publisher)
    case 'genres':
      return Array.isArray(match.genres) && match.genres.length > 0
    case 'rating':
      return hasNumberValue(match.rating)
    case 'max_players':
      return hasNumberValue(match.max_players)
  }
}

function collectMetadataAttributions(
  sourceGames: SourceGameDetailDTO[],
  field: MetadataField,
): string[] {
  const matches = sourceGames
    .flatMap((sourceGame) => sourceGame.resolver_matches)
    .filter((match) => !match.outvoted && resolverHasField(match, field))

  const metadataMatches = matches.filter(
    (match) => isMetadataPlugin(match.plugin_id) && resolverHasField(match, field),
  )
  const relevant = metadataMatches.length > 0 ? metadataMatches : matches

  return Array.from(new Set(relevant.map((match) => match.plugin_id)))
}

function sortAchievementSet(set: AchievementSetDTO): AchievementSetDTO {
  return {
    ...set,
    achievements: [...set.achievements].sort((a, b) => {
      if (a.unlocked !== b.unlocked) return a.unlocked ? -1 : 1
      if (a.unlocked_at && b.unlocked_at) {
        return new Date(b.unlocked_at).getTime() - new Date(a.unlocked_at).getTime()
      }
      return a.title.localeCompare(b.title)
    }),
  }
}

function SectionCard({ id, title, icon, children }: { id?: string; title: string; icon?: ReactNode; children: ReactNode }) {
  return (
    <section id={id} className="scroll-mt-28 rounded-mga border border-mga-border bg-mga-surface shadow-sm shadow-black/10">
      <div className="flex items-center gap-2 border-b border-mga-border px-4 py-3">
        {icon}
        <h2 className="text-base font-semibold text-mga-text">{title}</h2>
      </div>
      <div className="p-4 md:p-5">{children}</div>
    </section>
  )
}

function SourceBadge({ source, className }: { source: string; className?: string }) {
  return <BrandBadge brand={source} label={brandLabel(source, pluginLabel(source))} className={className} />
}

function AttributionNote({ sources, prefix = 'Source' }: { sources?: string[] | null; prefix?: string }) {
  if (!sources || sources.length === 0) return null
  return (
    <div className="mt-2 flex flex-wrap items-center gap-2 text-xs text-mga-muted">
      <span>{prefix}</span>
      {sources.map((source) => (
        <SourceBadge key={source} source={source} className="bg-mga-bg/80" />
      ))}
    </div>
  )
}

function MetaItem({ label, value, attributionSources, attributionPrefix }: { label: string; value: ReactNode; attributionSources?: string[] | null; attributionPrefix?: string }) {
  return (
    <div className="rounded-mga border border-mga-border bg-mga-bg/70 p-3">
      <p className="text-xs font-medium uppercase tracking-wide text-mga-muted">{label}</p>
      <div className="mt-1 text-sm text-mga-text">{value}</div>
      <AttributionNote sources={attributionSources} prefix={attributionPrefix} />
    </div>
  )
}

function FileRow({ file }: { file: GameFileDTO }) {
  return (
    <div className="rounded-mga border border-mga-border bg-mga-bg/60 p-3 text-sm">
      <div className="flex flex-wrap items-center gap-2">
        <Badge variant="muted">{file.role}</Badge>
        {file.file_kind && <Badge>{file.file_kind}</Badge>}
        <span className="text-xs text-mga-muted">{formatBytes(file.size)}</span>
      </div>
      <p className="mt-2 break-all font-mono text-xs text-mga-text">{file.path}</p>
    </div>
  )
}

function AchievementRow({ achievement }: { achievement: AchievementDTO }) {
  const iconUrl = achievement.unlocked ? achievement.unlocked_icon : achievement.locked_icon

  return (
    <div className="flex items-start gap-3 rounded-mga border border-mga-border bg-mga-bg/60 p-3">
      <div className="h-12 w-12 shrink-0 overflow-hidden rounded-mga border border-mga-border bg-mga-surface">
        {iconUrl ? (
          <img
            src={iconUrl}
            alt=""
            loading="lazy"
            decoding="async"
            className={cn('h-full w-full object-cover', achievement.unlocked ? '' : 'opacity-70 grayscale')}
          />
        ) : (
          <div className="flex h-full w-full items-center justify-center text-mga-muted">
            <Trophy size={18} />
          </div>
        )}
      </div>
      <div className="min-w-0 flex-1 space-y-1">
        <div className="flex flex-wrap items-center gap-2">
          <p className="font-medium text-mga-text">{achievement.title}</p>
          {achievement.unlocked ? <Badge variant="accent">Unlocked</Badge> : <Badge variant="muted">Locked</Badge>}
          {achievement.points !== undefined && achievement.points > 0 && <Badge>{achievement.points} pts</Badge>}
          {achievement.rarity !== undefined && achievement.rarity > 0 && <Badge>{achievement.rarity.toFixed(1)}%</Badge>}
        </div>
        {achievement.description && <p className="text-sm leading-6 text-mga-muted">{achievement.description}</p>}
        {achievement.unlocked_at && <p className="text-xs text-mga-muted">Unlocked {formatDateTimeValue(achievement.unlocked_at)}</p>}
      </div>
    </div>
  )
}

function summarizeResolverMatch(match: ResolverMatchDTO): string[] {
  const facts: string[] = []
  if (match.release_date) facts.push(formatDateValue(match.release_date))
  if (match.developer) facts.push(match.developer)
  if (match.publisher) facts.push(match.publisher)
  if (match.genres && match.genres.length > 0) facts.push(match.genres.join(', '))
  if (match.rating && match.rating > 0) facts.push(`Rating ${match.rating.toFixed(1)}`)
  if (match.max_players && match.max_players > 0) facts.push(`${match.max_players} players`)
  if (match.xcloud_available) facts.push('xCloud ready')
  if (match.is_game_pass) facts.push('Game Pass')
  return facts
}

function ResolverMatchRow({ match }: { match: ResolverMatchDTO }) {
  const facts = summarizeResolverMatch(match)

  return (
    <div className="rounded-mga border border-mga-border bg-mga-bg/60 p-3">
      <div className="flex flex-wrap items-center gap-2">
        <SourceBadge source={match.plugin_id} />
        {match.outvoted && <Badge variant="muted">Outvoted</Badge>}
        {match.xcloud_available && <BrandBadge brand="xcloud" label="xCloud" />}
        {match.is_game_pass && <Badge variant="gamepass">Game Pass</Badge>}
        {match.url && (
          <a href={match.url} target="_blank" rel="noreferrer" className="inline-flex items-center gap-1 text-xs font-medium text-mga-accent hover:underline">
            Open match
            <ExternalLink size={12} />
          </a>
        )}
      </div>
      <div className="mt-2 grid gap-2 text-sm md:grid-cols-2">
        <MetaItem label="Title" value={match.title ?? 'Unknown'} />
        <MetaItem label="External ID" value={match.external_id} />
      </div>
      {facts.length > 0 && <p className="mt-3 text-xs leading-5 text-mga-muted">{facts.join(' • ')}</p>}
    </div>
  )
}

function SourceRecordCard({ source }: { source: SourceGameDetailDTO }) {
  return (
    <article className="space-y-4 rounded-mga border border-mga-border bg-mga-bg/60 p-4">
      <div className="flex flex-wrap items-start justify-between gap-3">
        <div className="space-y-2">
          <div className="flex flex-wrap items-center gap-2">
            <SourceBadge source={source.plugin_id} />
            <Badge variant="source">{source.status}</Badge>
            <Badge variant="platform"><PlatformIcon platform={source.platform} showLabel /></Badge>
          </div>
          <p className="text-sm text-mga-muted">{source.raw_title || source.external_id}</p>
        </div>

        {source.url && (
          <a href={source.url} target="_blank" rel="noreferrer" className="inline-flex items-center gap-2 rounded-mga border border-mga-border bg-mga-surface px-3 py-1.5 text-xs font-medium text-mga-text transition-colors hover:bg-mga-elevated">
            <BrandIcon brand={source.plugin_id} />
            Open Source
            <ExternalLink size={14} />
          </a>
        )}
      </div>

      <div className="grid gap-3 text-sm md:grid-cols-2 xl:grid-cols-3">
        <MetaItem label="Integration" value={source.integration_id} />
        <MetaItem label="External ID" value={source.external_id} />
        <MetaItem label="Kind" value={source.kind} />
        <MetaItem label="Created" value={formatDateTimeValue(source.created_at)} />
        <MetaItem label="Last Seen" value={formatDateTimeValue(source.last_seen_at)} />
        <MetaItem label="Root Path" value={source.root_path ?? 'Unknown'} />
      </div>

      <details className="rounded-mga border border-mga-border bg-mga-surface px-3 py-2">
        <summary className="cursor-pointer list-none text-sm font-medium text-mga-text">
          Resolver Matches ({source.resolver_matches.length})
        </summary>
        <div className="mt-3 space-y-2">
          {source.resolver_matches.length === 0 ? (
            <p className="text-sm text-mga-muted">No resolver matches stored for this source game.</p>
          ) : (
            source.resolver_matches.map((match) => (
              <ResolverMatchRow key={`${match.plugin_id}:${match.external_id}:${match.title ?? ''}`} match={match} />
            ))
          )}
        </div>
      </details>

      <details className="rounded-mga border border-mga-border bg-mga-surface px-3 py-2">
        <summary className="cursor-pointer list-none text-sm font-medium text-mga-text">
          Files ({source.files.length})
        </summary>
        <div className="mt-3 space-y-2">
          {source.files.length === 0 ? (
            <p className="text-sm text-mga-muted">No files associated with this source game.</p>
          ) : (
            source.files.map((file) => <FileRow key={`${file.path}:${file.role}`} file={file} />)
          )}
        </div>
      </details>
    </article>
  )
}

function MediaPreview({ media }: { media: GameMediaDetailDTO }) {
  const url = mediaUrl(media)

  if (isInlineVideoMedia(media)) {
    return (
      <video controls preload="metadata" className="max-h-[360px] w-full rounded-mga border border-mga-border bg-black">
        <source src={url} type={media.mime_type} />
      </video>
    )
  }
  if (isInlineAudioMedia(media)) {
    return (
      <div className="rounded-mga border border-mga-border bg-mga-surface p-4">
        <audio controls preload="metadata" className="w-full">
          <source src={url} type={media.mime_type} />
        </audio>
      </div>
    )
  }
  if (isPdfMedia(media)) {
    return <iframe src={url} title={`${mediaTypeLabel(media.type)} preview`} className="h-[360px] w-full rounded-mga border border-mga-border bg-white" />
  }
  return (
    <div className="flex items-center gap-2 rounded-mga border border-dashed border-mga-border bg-mga-surface px-3 py-4 text-sm text-mga-muted">
      {media.mime_type?.startsWith('video/') ? <Video size={16} /> : <FileText size={16} />}
      {`${mediaTypeLabel(media.type)} cannot be previewed inline in the browser. Use the external link above.`}
    </div>
  )
}

function OtherMediaCard({ media }: { media: GameMediaDetailDTO }) {
  const url = mediaUrl(media)
  return (
    <article className="space-y-3 rounded-mga border border-mga-border bg-mga-bg/60 p-4">
      <div className="flex flex-wrap items-center justify-between gap-3">
        <div className="space-y-1">
          <div className="flex flex-wrap items-center gap-2">
            <Badge>{mediaTypeLabel(media.type)}</Badge>
            {media.source && <SourceBadge source={media.source} />}
            {media.local_path && <Badge variant="muted">Local</Badge>}
          </div>
          <p className="text-sm text-mga-muted">
            {media.mime_type || 'Remote media asset'}
            {media.width && media.height ? ` • ${media.width} × ${media.height}` : ''}
          </p>
        </div>
        <a href={url} target="_blank" rel="noreferrer" className="inline-flex items-center gap-1 text-sm font-medium text-mga-accent hover:underline">
          Open media
          <ExternalLink size={14} />
        </a>
      </div>
      <MediaPreview media={media} />
    </article>
  )
}

function MediaViewerDialog({ media, onClose }: { media: GameMediaDetailDTO | null; onClose: () => void }) {
  return (
    <Dialog open={media !== null} onClose={onClose} title={media ? mediaTypeLabel(media.type) : 'Media'} className="max-w-5xl">
      {media && (
        <div className="space-y-4">
          <div className="overflow-hidden rounded-mga border border-mga-border bg-mga-bg">
            <img src={mediaUrl(media)} alt={mediaTypeLabel(media.type)} className="max-h-[75vh] w-full object-contain" />
          </div>
          <div className="flex flex-wrap items-center gap-2 text-sm text-mga-muted">
            <Badge>{mediaTypeLabel(media.type)}</Badge>
            {media.source && <SourceBadge source={media.source} />}
            {media.width && media.height && <span>{media.width} × {media.height}</span>}
            <a href={mediaUrl(media)} target="_blank" rel="noreferrer" className="inline-flex items-center gap-1 font-medium text-mga-accent hover:underline">
              Open image
              <ExternalLink size={14} />
            </a>
          </div>
        </div>
      )}
    </Dialog>
  )
}

function ExternalLinkCard({ link }: { link: ExternalLinkItem }) {
  return (
    <a href={link.url} target="_blank" rel="noreferrer" className="flex items-start gap-3 rounded-mga border border-mga-border bg-mga-bg/60 p-3 transition-colors hover:border-mga-accent">
      <div className="flex h-11 w-11 shrink-0 items-center justify-center rounded-mga border border-mga-border bg-mga-surface">
        {link.brandId ? <BrandIcon brand={link.brandId} className="h-6 w-6" /> : <ExternalLink size={16} className="text-mga-accent" />}
      </div>
      <div className="min-w-0 flex-1">
        <div className="flex flex-wrap items-center gap-2">
          <p className="font-medium text-mga-text">{link.label}</p>
          <Badge variant="muted">{link.host}</Badge>
        </div>
        <p className="mt-1 truncate text-xs text-mga-muted">{link.subtitle}</p>
        <div className="mt-2 flex flex-wrap items-center gap-2">
          <SourceBadge source={link.source} className="bg-mga-surface" />
          <span className="text-xs font-medium text-mga-accent">{link.actionLabel}</span>
        </div>
      </div>
      <ExternalLink size={16} className="mt-0.5 shrink-0 text-mga-accent" />
    </a>
  )
}

export function GameDetailPage() {
  const navigate = useNavigate()
  const location = useLocation()
  const { id = '' } = useParams()
  const queryClient = useQueryClient()
  const { recordLaunch } = useRecentPlayed()
  const [selectedMedia, setSelectedMedia] = useState<GameMediaDetailDTO | null>(null)
  const hasRetried404Ref = useRef(false)

  const routeState = readGameRouteState(location.state)
  const from = routeState?.from ?? '/library'
  const originLabel = routeState?.originLabel ?? inferOriginLabel(from)

  const game = useQuery({
    queryKey: ['game', id],
    queryFn: async () => {
      try {
        return await getGame(id)
      } catch (error) {
        if (
          error instanceof ApiError &&
          error.status === 404 &&
          !hasRetried404Ref.current
        ) {
          hasRetried404Ref.current = true
          await queryClient.invalidateQueries({ queryKey: ['games'] })
          return getGame(id)
        }
        throw error
      }
    },
    enabled: id.length > 0,
  })

  const achievements = useQuery({
    queryKey: ['game', id, 'achievements'],
    queryFn: () => getGameAchievements(id),
    enabled: id.length > 0,
  })

  const gameData = game.data ?? null
  const imageMedia = useMemo(() => (gameData?.media ?? []).filter(isImageMedia), [gameData?.media])
  const nonImageMedia = useMemo(() => (gameData?.media ?? []).filter((media) => !isImageMedia(media)), [gameData?.media])
  const coverMedia = useMemo(() => selectCoverMedia(imageMedia), [imageMedia])
  const heroMedia = useMemo(() => selectHeroMedia(imageMedia), [imageMedia])
  const heroUrl = heroMedia ? mediaUrl(heroMedia) : null
  const coverUrl = coverMedia ? mediaUrl(coverMedia) : null
  const playable = gameData ? isPlayable(gameData.platform) : false
  const sources = gameData ? selectSourcePlugins(gameData) : []
  const hltb = gameData ? formatHLTB(gameData.completion_time) : null
  const matchCount = gameData ? resolverMatchCount(gameData) : 0
  const externalLinks = useMemo(() => buildExternalLinks(gameData?.external_ids), [gameData?.external_ids])
  const metadataAttribution = useMemo(() => {
    const sourceGames = gameData?.source_games ?? []
    return {
      title: collectMetadataAttributions(sourceGames, 'title'),
      description: collectMetadataAttributions(sourceGames, 'description'),
      release_date: collectMetadataAttributions(sourceGames, 'release_date'),
      developer: collectMetadataAttributions(sourceGames, 'developer'),
      publisher: collectMetadataAttributions(sourceGames, 'publisher'),
      genres: collectMetadataAttributions(sourceGames, 'genres'),
      rating: collectMetadataAttributions(sourceGames, 'rating'),
      max_players: collectMetadataAttributions(sourceGames, 'max_players'),
    }
  }, [gameData?.source_games])
  const achievementSets = useMemo(() => (achievements.data ?? []).map(sortAchievementSet), [achievements.data])
  const achievementSummary = useMemo(() => summarizeAchievements(achievementSets), [achievementSets])

  useEffect(() => {
    hasRetried404Ref.current = false
  }, [id])

  useEffect(() => {
    if (!game.data || id.length === 0 || game.data.id === id) return
    navigate(`/game/${encodeURIComponent(game.data.id)}`, {
      replace: true,
      state: location.state,
    })
  }, [game.data, id, location.state, navigate])

  const handleLaunchXcloud = () => {
    if (!game.data?.xcloud_url) return
    recordLaunch({
      gameId: game.data.id,
      title: game.data.title,
      platform: game.data.platform,
      coverUrl,
      launchKind: 'xcloud',
      launchUrl: game.data.xcloud_url,
    })
  }

  const handleBack = () => {
    const shouldRestoreScroll = from.startsWith('/play') || from.startsWith('/library')
    navigate(from, shouldRestoreScroll ? { state: { restoreScroll: true } } : undefined)
  }

  if (game.isPending) {
    return (
      <div className="mx-auto max-w-5xl space-y-4 p-4 md:p-6">
        <Button variant="outline" size="sm" onClick={handleBack}>
          <ArrowLeft size={14} />
          {originLabel}
        </Button>
        <div className="rounded-mga border border-mga-border bg-mga-surface p-6">
          <p className="text-sm text-mga-muted">Loading game details...</p>
        </div>
      </div>
    )
  }

  if (game.isError || !game.data) {
    return (
      <div className="mx-auto max-w-5xl space-y-4 p-4 md:p-6">
        <Button variant="outline" size="sm" onClick={handleBack}>
          <ArrowLeft size={14} />
          {originLabel}
        </Button>
        <div className="rounded-mga border border-red-500/30 bg-red-500/10 p-6">
          <p className="text-sm text-red-400">{game.isError ? game.error.message : 'Game not found.'}</p>
        </div>
      </div>
    )
  }

  const data = game.data
  const achievementPercent = achievementSummary.totalCount > 0 ? (achievementSummary.unlockedCount / achievementSummary.totalCount) * 100 : 0

  return (
    <div className="mx-auto max-w-7xl space-y-6 p-4 md:p-6">
      <Button variant="outline" size="sm" onClick={handleBack}>
        <ArrowLeft size={14} />
        {originLabel}
      </Button>

      <section className="relative overflow-hidden rounded-mga border border-mga-border bg-mga-surface shadow-lg shadow-black/20">
        {heroUrl && (
          <div className="absolute inset-0">
            <img src={heroUrl} alt="" className="h-full w-full scale-105 object-cover opacity-30 blur-2xl" aria-hidden="true" />
            <div className="absolute inset-0 bg-gradient-to-br from-mga-bg/90 via-mga-surface/95 to-mga-bg/90" />
          </div>
        )}

        <div className="relative grid gap-6 p-5 md:grid-cols-[240px,1fr] md:p-8">
          <div className="overflow-hidden rounded-mga border border-mga-border bg-mga-bg shadow-md shadow-black/20">
            <CoverImage src={coverUrl} alt={data.title} className="aspect-[2/3] h-full w-full" />
          </div>

          <div className="space-y-5">
            <div className="space-y-3">
              <div className="flex flex-wrap items-center gap-2">
                <Badge variant="platform"><PlatformIcon platform={data.platform} showLabel /></Badge>
                {data.xcloud_available && <BrandBadge brand="xcloud" label="xCloud" />}
                {data.is_game_pass && <Badge variant="gamepass">Game Pass</Badge>}
                {playable && <Badge variant="playable">Playable Platform</Badge>}
                {hltb && <Badge>{hltb}</Badge>}
                {matchCount > 0 && <Badge>{matchCount} matches</Badge>}
              </div>

              <div>
                <h1 className="text-3xl font-semibold tracking-tight text-mga-text md:text-4xl">{data.title}</h1>
                <AttributionNote sources={metadataAttribution.title} prefix="Title aligned to" />
                <p className="mt-3 max-w-3xl text-sm leading-7 text-mga-muted">
                  {data.description || 'No description is available for this game yet.'}
                </p>
                {data.description && <AttributionNote sources={metadataAttribution.description} prefix="Description attributed to" />}
              </div>
            </div>

            <div className="space-y-3">
              <div className="flex flex-wrap gap-3">
                {data.xcloud_url && (
                  <a href={data.xcloud_url} target="_blank" rel="noreferrer" onClick={handleLaunchXcloud} className="inline-flex h-10 items-center justify-center gap-2 rounded-mga bg-mga-accent px-4 py-2 text-sm font-medium text-white transition-colors hover:opacity-90">
                    <BrandIcon brand="xcloud" className="h-4 w-4 invert" />
                    Play on xCloud
                  </a>
                )}
                {playable && (
                  <div className="inline-flex h-10 items-center justify-center gap-2 rounded-mga border border-mga-border bg-mga-bg px-4 py-2 text-sm font-medium text-mga-muted">
                    <PlayCircle size={16} />
                    Browser Play in Phase 6
                  </div>
                )}
                {externalLinks.length > 0 && (
                  <a href="#external-links" className="inline-flex h-10 items-center justify-center gap-2 rounded-mga border border-mga-border bg-mga-bg px-4 py-2 text-sm font-medium text-mga-text transition-colors hover:bg-mga-elevated">
                    <ExternalLink size={16} />
                    External Links
                  </a>
                )}
                {data.source_games.length > 0 && (
                  <a href="#source-records" className="inline-flex h-10 items-center justify-center gap-2 rounded-mga border border-mga-border bg-mga-bg px-4 py-2 text-sm font-medium text-mga-text transition-colors hover:bg-mga-elevated">
                    <Database size={16} />
                    Source Records
                  </a>
                )}
              </div>
              {data.xcloud_url && <AttributionNote sources={['xcloud']} prefix="Streaming target" />}
            </div>

            <div className="grid gap-3 md:grid-cols-2 xl:grid-cols-4">
              <MetaItem label="Release Date" value={formatDateValue(data.release_date)} attributionSources={metadataAttribution.release_date} />
              <MetaItem label="Developer" value={detailValue(data.developer)} attributionSources={metadataAttribution.developer} />
              <MetaItem label="Publisher" value={detailValue(data.publisher)} attributionSources={metadataAttribution.publisher} />
              <MetaItem label="Genres" value={data.genres && data.genres.length > 0 ? data.genres.join(', ') : 'Unknown'} attributionSources={metadataAttribution.genres} />
              <MetaItem label="Rating" value={data.rating ? data.rating.toFixed(1) : 'Unknown'} attributionSources={metadataAttribution.rating} />
              <MetaItem label="Players" value={data.max_players ? `${data.max_players}` : 'Unknown'} attributionSources={metadataAttribution.max_players} />
              <MetaItem label="Main Story" value={formatHours(data.completion_time?.main_story)} attributionSources={data.completion_time?.source ? [data.completion_time.source] : null} attributionPrefix="Estimate from" />
              <MetaItem label="Completionist" value={formatHours(data.completion_time?.completionist)} attributionSources={data.completion_time?.source ? [data.completion_time.source] : null} attributionPrefix="Estimate from" />
            </div>

            {sources.length > 0 && (
              <div className="space-y-2">
                <p className="text-sm font-medium text-mga-text">Sources</p>
                <div className="flex flex-wrap gap-2">
                  {sources.map((source) => <SourceBadge key={source} source={source} />)}
                </div>
              </div>
            )}
          </div>
        </div>
      </section>

      <div className="grid gap-6 xl:grid-cols-[minmax(0,1.55fr),minmax(320px,0.95fr)]">
        <div className="space-y-6">
          <SectionCard id="media-gallery" title="Media Gallery" icon={<ImageIcon size={18} className="text-mga-accent" />}>
            {imageMedia.length === 0 ? (
              <p className="text-sm text-mga-muted">No image media is available for this game yet.</p>
            ) : (
              <div className="grid grid-cols-2 gap-3 md:grid-cols-3">
                {imageMedia.map((media) => (
                  <button key={`${media.asset_id}:${media.type}`} type="button" onClick={() => setSelectedMedia(media)} className="group overflow-hidden rounded-mga border border-mga-border bg-mga-bg text-left transition-colors hover:border-mga-accent">
                    <img src={mediaUrl(media)} alt={mediaTypeLabel(media.type)} loading="lazy" decoding="async" className="aspect-video w-full object-cover transition-transform duration-200 group-hover:scale-[1.02]" />
                    <div className="space-y-2 border-t border-mga-border px-3 py-2">
                      <span className="block truncate text-xs font-medium text-mga-text">{mediaTypeLabel(media.type)}</span>
                      <div className="flex flex-wrap items-center gap-2">
                        {media.source && <SourceBadge source={media.source} className="bg-mga-surface" />}
                        {media.local_path && <Badge variant="muted">Local</Badge>}
                      </div>
                    </div>
                  </button>
                ))}
              </div>
            )}
          </SectionCard>

          {nonImageMedia.length > 0 && (
            <SectionCard title="Other Media" icon={<Video size={18} className="text-mga-accent" />}>
              <div className="space-y-4">
                {nonImageMedia.map((media) => <OtherMediaCard key={`${media.asset_id}:${media.type}`} media={media} />)}
              </div>
            </SectionCard>
          )}

          <SectionCard id="achievements" title="Achievements" icon={<Trophy size={18} className="text-mga-accent" />}>
            {achievements.isPending ? (
              <p className="text-sm text-mga-muted">Loading achievements...</p>
            ) : achievements.isError ? (
              <div className="rounded-mga border border-red-500/30 bg-red-500/10 p-4">
                <p className="text-sm text-red-400">{achievements.error.message}</p>
              </div>
            ) : achievementSets.length > 0 ? (
              <div className="space-y-4">
                <div className="grid gap-3 md:grid-cols-3">
                  <MetaItem label="Sources" value={achievementSets.length} />
                  <MetaItem label="Unlocked" value={`${achievementSummary.unlockedCount}/${achievementSummary.totalCount}`} />
                  <MetaItem label="Points" value={achievementSummary.totalPoints > 0 ? `${achievementSummary.earnedPoints}/${achievementSummary.totalPoints}` : 'Unknown'} />
                </div>
                <ProgressBar value={achievementPercent} label={`${achievementSummary.unlockedCount}/${achievementSummary.totalCount}`} />
                {achievementSets.map((set) => (
                  <div key={`${set.source}:${set.external_game_id}`} className="space-y-4 rounded-mga border border-mga-border bg-mga-bg/60 p-4">
                    <div className="flex flex-wrap items-center justify-between gap-3">
                      <div className="space-y-1">
                        <div className="flex flex-wrap items-center gap-2">
                          <SourceBadge source={set.source} />
                          <span className="text-sm font-medium text-mga-text">{set.unlocked_count}/{set.total_count} unlocked</span>
                          <Badge variant="muted">{Math.round(achievementProgress(set))}% complete</Badge>
                        </div>
                        <p className="text-xs text-mga-muted">External game ID: {set.external_game_id}</p>
                      </div>
                      <div className="text-right text-sm text-mga-muted">
                        {set.total_points !== undefined && set.total_points > 0 && <p>{set.earned_points ?? 0}/{set.total_points} points</p>}
                      </div>
                    </div>
                    <ProgressBar value={achievementProgress(set)} label={`${set.unlocked_count}/${set.total_count}`} />
                    <div className="space-y-3">
                      {set.achievements.map((achievement) => <AchievementRow key={`${set.source}:${achievement.external_id}`} achievement={achievement} />)}
                    </div>
                  </div>
                ))}
              </div>
            ) : (
              <p className="text-sm text-mga-muted">No achievements are available for this game.</p>
            )}
          </SectionCard>

          <SectionCard id="source-records" title="Source Records" icon={<Database size={18} className="text-mga-accent" />}>
            <div className="space-y-4">
              {data.source_games.length === 0 ? (
                <p className="text-sm text-mga-muted">No source records are stored for this game.</p>
              ) : (
                data.source_games.map((source) => <SourceRecordCard key={source.id} source={source} />)
              )}
            </div>
          </SectionCard>
        </div>

        <div className="space-y-6">
          <SectionCard id="quick-facts" title="Quick Facts" icon={<FolderOpen size={18} className="text-mga-accent" />}>
            <div className="grid gap-3 sm:grid-cols-2 xl:grid-cols-1">
              <MetaItem label="Canonical ID" value={data.id} />
              <MetaItem label="Source Records" value={data.source_games.length} />
              <MetaItem label="Media Items" value={data.media?.length ?? 0} />
              <MetaItem label="Files" value={data.files?.length ?? 0} />
              <MetaItem label="Resolver Matches" value={matchCount} />
              <MetaItem label="Root Path" value={data.root_path ?? 'Unknown'} />
              <MetaItem label="HLTB Main + Extra" value={formatHours(data.completion_time?.main_extra)} attributionSources={data.completion_time?.source ? [data.completion_time.source] : null} attributionPrefix="Estimate from" />
              <MetaItem label="HLTB Source" value={data.completion_time?.source ? <SourceBadge source={data.completion_time.source} /> : 'Unknown'} />
            </div>
          </SectionCard>

          {externalLinks.length > 0 && (
            <SectionCard id="external-links" title="External Links" icon={<ExternalLink size={18} className="text-mga-accent" />}>
              <div className="space-y-3">
                {externalLinks.map((link) => <ExternalLinkCard key={link.id} link={link} />)}
              </div>
            </SectionCard>
          )}

          <SectionCard id="merged-files" title="Merged Files" icon={<HardDrive size={18} className="text-mga-accent" />}>
            {data.files && data.files.length > 0 ? (
              <div className="space-y-2">
                {data.files.map((file) => <FileRow key={`${file.path}:${file.role}`} file={file} />)}
              </div>
            ) : (
              <p className="text-sm text-mga-muted">No merged files are available for this game.</p>
            )}
          </SectionCard>
        </div>
      </div>

      <MediaViewerDialog media={selectedMedia} onClose={() => setSelectedMedia(null)} />
    </div>
  )
}
