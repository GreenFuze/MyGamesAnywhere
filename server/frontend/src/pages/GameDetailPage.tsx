import { useEffect, useMemo, useRef, useState, type ReactNode } from 'react'
import { createPortal } from 'react-dom'
import { useQuery, useQueryClient } from '@tanstack/react-query'
import {
  ArrowLeft,
  ArrowRightLeft,
  Database,
  ExternalLink,
  FileText,
  FolderOpen,
  HardDrive,
  Loader2,
  MoreHorizontal,
  PlayCircle,
  Trophy,
  Video,
} from 'lucide-react'
import { useLocation, useNavigate, useParams } from 'react-router-dom'
import {
  ApiError,
  deleteSourceGame,
  getGame,
  getGameAchievements,
  refreshGameMetadata,
  type AchievementDTO,
  type AchievementSetDTO,
  type ExternalIDDTO,
  type GameFileDTO,
  type GameLaunchOptionDTO,
  type GameMediaDetailDTO,
  type ResolverMatchDTO,
  type SourceGameDetailDTO,
} from '@/api/client'
import { useGameFavoriteAction } from '@/hooks/useGameFavorite'
import { useRecentPlayed } from '@/hooks/useRecentPlayed'
import { AchievementProgressRing } from '@/components/library/AchievementProgressRing'
import { BrandBadge, BrandIcon } from '@/components/ui/brand-icon'
import { Badge } from '@/components/ui/badge'
import { Button } from '@/components/ui/button'
import { Dialog } from '@/components/ui/dialog'
import { PlatformIcon } from '@/components/ui/platform-icon'
import {
  brandLabel,
  resolveBrandDefinition,
  resolveBrandDefinitionFromUrl,
} from '@/lib/brands'
import {
  clearBrowserPlaySourcePreference,
  browserPlaySourceContext,
  browserPlaySourceOptionLabel,
  getBrowserPlayPreferenceRuntime,
  readBrowserPlaySourcePreference,
  resolveBrowserPlaySelection,
  writeBrowserPlaySourcePreference,
} from '@/lib/browserPlay'
import { inferOriginLabel, readGameRouteState } from '@/lib/gameNavigation'
import {
  hasBrowserPlaySupport,
  pluginLabel,
  platformLabel,
  selectSourcePlugins,
} from '@/lib/gameUtils'
import { GameMediaCollection, mediaOriginalUrl, mediaUrl, youtubeEmbedUrl, youtubeThumbnailUrl } from '@/lib/gameMedia'
import { buildFeaturedMediaRail, mergeDisplayMedia } from '@/lib/gameMediaDisplay'
import { evaluateBackgroundSuitability } from '@/lib/backgroundSuitability'
import { cn } from '@/lib/utils'

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

type GameFileDisplayRecord = {
  sourceGameId: string
  sourcePluginId: string
  sourceTitle: string
  isLaunchFile: boolean
  file: GameFileDTO
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

function humanizeValue(value: string): string {
  return value
    .split(/[_-]+/g)
    .filter(Boolean)
    .map((part) => {
      if (part.length <= 3) return part.toUpperCase()
      return part.charAt(0).toUpperCase() + part.slice(1)
    })
    .join(' ')
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

function splitHeroDescription(value: string | undefined): { tagline: string | null; body: string | null } {
  if (!value) return { tagline: null, body: null }
  const normalized = value.replace(/\s+/g, ' ').trim()
  if (!normalized) return { tagline: null, body: null }
  const parts = normalized.split(/(?<=[.!?])\s+/)
  if (parts.length < 2 || parts[0].length > 96) {
    return { tagline: null, body: normalized }
  }
  return {
    tagline: parts[0].trim(),
    body: parts.slice(1).join(' ').trim() || parts[0].trim(),
  }
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

function collectUnifiedMetadataSources(
  sourceGames: SourceGameDetailDTO[],
  media: GameMediaDetailDTO[] | undefined,
): string[] {
  const fieldSources = [
    ...collectMetadataAttributions(sourceGames, 'title'),
    ...collectMetadataAttributions(sourceGames, 'description'),
    ...collectMetadataAttributions(sourceGames, 'release_date'),
    ...collectMetadataAttributions(sourceGames, 'developer'),
    ...collectMetadataAttributions(sourceGames, 'publisher'),
    ...collectMetadataAttributions(sourceGames, 'genres'),
    ...collectMetadataAttributions(sourceGames, 'rating'),
    ...collectMetadataAttributions(sourceGames, 'max_players'),
  ]
  const mediaSources = (media ?? [])
    .map((item) => item.source?.trim())
    .filter((item): item is string => Boolean(item))

  return Array.from(new Set([...fieldSources, ...mediaSources])).sort((a, b) => pluginLabel(a).localeCompare(pluginLabel(b)))
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

function achievementSetTitle(set: AchievementSetDTO): string {
  const source = pluginLabel(set.source)
  const platform = set.platform && set.platform !== 'unknown' ? platformLabel(set.platform) : ''
  return platform ? `${source} • ${platform}` : source
}

function achievementSetContext(set: AchievementSetDTO): string {
  const parts = [
    set.integration_label?.trim(),
    set.source_title?.trim(),
  ].filter((part): part is string => Boolean(part))
  return Array.from(new Set(parts)).join(' · ')
}

function launchOptionContext(option: GameLaunchOptionDTO): string {
  const parts = [
    option.integration_label?.trim(),
    option.platform && option.platform !== 'unknown' ? platformLabel(option.platform) : '',
    option.source_title?.trim(),
  ].filter((part): part is string => Boolean(part))
  return Array.from(new Set(parts)).join(' · ')
}

function launchOptionKey(option: GameLaunchOptionDTO): string {
  return `${option.kind}:${option.source_game_id}:${option.url ?? option.file_id ?? option.root_file_id ?? ''}`
}

function SectionCard({
  id,
  title,
  icon,
  description,
  actions,
  className,
  children,
}: {
  id?: string
  title: string
  icon?: ReactNode
  description?: ReactNode
  actions?: ReactNode
  className?: string
  children: ReactNode
}) {
  return (
    <section
      id={id}
      className={cn(
        'scroll-mt-28 overflow-hidden rounded-[28px] border border-white/[0.05] bg-[linear-gradient(180deg,rgba(14,19,30,0.92),rgba(10,14,23,0.92))] shadow-[0_18px_42px_rgba(0,0,0,0.2)]',
        className,
      )}
    >
      <div className="flex flex-wrap items-start justify-between gap-3 border-b border-white/[0.05] px-5 py-4 md:px-6">
        <div className="space-y-1">
          <div className="flex items-center gap-2">
            {icon}
            <h2 className="text-base font-semibold text-white">{title}</h2>
          </div>
          {description ? <div className="text-sm leading-6 text-white/60">{description}</div> : null}
        </div>
        {actions ? <div className="flex shrink-0 flex-wrap gap-2">{actions}</div> : null}
      </div>
      <div className="p-5 md:p-6">{children}</div>
    </section>
  )
}

function SourceBadge({ source, className }: { source: string; className?: string }) {
  return <BrandBadge brand={source} label={brandLabel(source, pluginLabel(source))} className={className} />
}

function AttributionNote({ sources, prefix = 'Source' }: { sources?: string[] | null; prefix?: string }) {
  if (!sources || sources.length === 0) return null
  return (
    <div className="mt-2 flex flex-wrap items-center gap-2 text-xs text-white/58">
      <span className="text-white/52">{prefix}</span>
      {sources.map((source) => (
        <SourceBadge key={source} source={source} className="border-white/[0.08] bg-white/[0.04] text-white" />
      ))}
    </div>
  )
}

function MetaItem({ label, value, attributionSources, attributionPrefix }: { label: string; value: ReactNode; attributionSources?: string[] | null; attributionPrefix?: string }) {
  return (
    <div className="rounded-[20px] border border-white/[0.05] bg-[#101723] p-4 shadow-[0_10px_22px_rgba(0,0,0,0.14)]">
      <p className="text-[11px] font-medium uppercase tracking-[0.18em] text-white/42">{label}</p>
      <div className="mt-2 text-sm font-medium text-white">{value}</div>
      <AttributionNote sources={attributionSources} prefix={attributionPrefix} />
    </div>
  )
}

function HeroStatCard({ label, value, detail }: { label: string; value: ReactNode; detail?: ReactNode }) {
  return (
    <div className="rounded-[20px] border border-white/[0.05] bg-[#101824] px-4 py-4 shadow-[0_12px_28px_rgba(0,0,0,0.16)]">
      <p className="text-[11px] font-medium uppercase tracking-[0.18em] text-white/42">{label}</p>
      <div className="mt-2 text-lg font-semibold text-white">{value}</div>
      {detail ? <div className="mt-1.5 text-xs leading-5 text-white/58">{detail}</div> : null}
    </div>
  )
}

function HeroFactStripItem({ label, value, detail }: { label: string; value: ReactNode; detail?: ReactNode }) {
  return (
    <div className="min-w-0 px-4 py-4">
      <p className="text-[11px] font-medium uppercase tracking-[0.18em] text-white/38">{label}</p>
      <div className="mt-2 text-lg font-semibold text-white">{value}</div>
      {detail ? <div className="mt-1 text-xs text-white/54">{detail}</div> : null}
    </div>
  )
}

function HeroTabLink({ href, label }: { href: string; label: string }) {
  return (
    <a
      href={href}
      className="inline-flex h-11 items-center justify-center rounded-full border border-white/[0.04] bg-white/[0.03] px-4 text-sm font-medium text-white/72 transition-colors hover:bg-white/[0.08] hover:text-white"
    >
      {label}
    </a>
  )
}

function mediaItemKey(media: GameMediaDetailDTO): string {
  return `${media.asset_id}:${media.type}`
}

function HeroMediaThumb({
  media,
  label,
  onSelect,
}: {
  media: GameMediaDetailDTO
  label: string
  onSelect: (media: GameMediaDetailDTO) => void
}) {
  const mediaCollection = new GameMediaCollection([media])
  const isImage = mediaCollection.isImage(media)
  const isVideo = !isImage && (Boolean(youtubeEmbedUrl(media)) || mediaCollection.isInlineVideo(media))
  const youtubeThumb = youtubeThumbnailUrl(media)

  return (
    <button
      type="button"
      onClick={() => onSelect(media)}
      className="group relative h-20 w-32 shrink-0 overflow-hidden rounded-[16px] bg-black/20 ring-1 ring-white/[0.05] transition-all duration-200 hover:-translate-y-0.5 hover:ring-white/[0.14] focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-white/60 focus-visible:ring-offset-0"
      title={label}
      aria-label={label}
    >
      {isImage ? (
        <img
          src={mediaUrl(media)}
          alt=""
          aria-hidden="true"
          loading="lazy"
          decoding="async"
          className="h-full w-full object-contain transition-transform duration-200 group-hover:scale-[1.03]"
        />
      ) : youtubeThumb ? (
        <div className="relative h-full w-full">
          <img
            src={youtubeThumb}
            alt=""
            aria-hidden="true"
            loading="lazy"
            decoding="async"
            className="h-full w-full object-cover transition-transform duration-200 group-hover:scale-[1.03]"
          />
          <div className="absolute inset-0 bg-black/20" />
        </div>
      ) : (
        <div className="flex h-full w-full items-center justify-center bg-[radial-gradient(circle_at_top,rgba(255,255,255,0.18),transparent_55%),linear-gradient(180deg,rgba(18,17,23,0.92),rgba(10,10,14,0.98))]">
          <div className="rounded-full border-2 border-white/80 bg-black/50 p-1 text-white shadow-[0_10px_24px_rgba(0,0,0,0.34)]">
            <PlayCircle size={34} strokeWidth={1.8} fill="rgba(255,255,255,0.12)" />
          </div>
        </div>
      )}
      <div className="absolute inset-0 bg-gradient-to-t from-black/70 via-transparent to-transparent" />
      {isVideo ? (
        <div className="absolute inset-0 flex items-center justify-center">
          <div className="rounded-full border-2 border-white/90 bg-black/52 p-1 text-white shadow-[0_10px_24px_rgba(0,0,0,0.34)]">
            <PlayCircle size={34} strokeWidth={1.8} fill="rgba(255,255,255,0.12)" />
          </div>
        </div>
      ) : null}
      <div className="absolute inset-x-2 bottom-1.5 truncate text-[11px] font-medium text-white/86">{label}</div>
    </button>
  )
}

function HeroActionButton({
  children,
  primary = false,
  className,
  ...props
}: React.ComponentProps<'button'> & { primary?: boolean }) {
  return (
    <button
      {...props}
      className={cn(
        'inline-flex h-11 items-center justify-center gap-2 rounded-[15px] px-4.5 text-sm font-medium transition-colors disabled:cursor-not-allowed disabled:opacity-60',
        primary
          ? 'bg-[linear-gradient(180deg,#7379ff,#5960ef)] text-white shadow-[0_12px_24px_rgba(90,97,239,0.28)] hover:opacity-95'
          : 'border border-white/[0.08] bg-[#101620] text-white hover:bg-white/[0.06]',
        className,
      )}
    >
      {children}
    </button>
  )
}

function FavoriteActionButton({
  favorite,
  busy,
  onClick,
  className,
}: {
  favorite: boolean
  busy: boolean
  onClick: () => void
  className?: string
}) {
  return (
    <button
      type="button"
      onClick={onClick}
      disabled={busy}
      aria-label={favorite ? 'Remove from favorites' : 'Add to favorites'}
      title={favorite ? 'Remove from favorites' : 'Add to favorites'}
      className={cn(
        'inline-flex h-11 w-11 items-center justify-center rounded-[15px] border text-lg transition-colors disabled:cursor-wait disabled:opacity-60',
        favorite
          ? 'border-rose-300/60 bg-rose-500/18 text-rose-100 hover:bg-rose-500/24'
          : 'border-white/[0.08] bg-[#101620] text-white hover:bg-white/[0.06]',
        className,
      )}
    >
      <span aria-hidden="true" className={cn('leading-none', favorite ? 'text-rose-400' : '')}>
        {favorite ? '♥' : '♡'}
      </span>
    </button>
  )
}

function HeroOverflowMenu({
  onRefresh,
  onReclassify,
  refreshBusy,
  className,
  direction = 'down',
}: {
  onRefresh: () => void
  onReclassify: () => void
  refreshBusy: boolean
  className?: string
  direction?: 'down' | 'up'
}) {
  const [open, setOpen] = useState(false)
  const buttonRef = useRef<HTMLButtonElement | null>(null)
  const menuRef = useRef<HTMLDivElement | null>(null)
  const [menuPosition, setMenuPosition] = useState<{
    top?: number
    bottom?: number
    left: number
    width: number
  } | null>(null)

  useEffect(() => {
    if (!open) return
    const updatePosition = () => {
      const button = buttonRef.current
      if (!button) return
      const rect = button.getBoundingClientRect()
      const width = Math.min(336, window.innerWidth - 32)
      const left = Math.min(Math.max(16, rect.right - width), window.innerWidth - width - 16)
      setMenuPosition(
        direction === 'down'
          ? { top: rect.bottom + 8, left, width }
          : { bottom: window.innerHeight - rect.top + 8, left, width },
      )
    }
    updatePosition()
    window.addEventListener('resize', updatePosition)
    window.addEventListener('scroll', updatePosition, true)
    return () => {
      window.removeEventListener('resize', updatePosition)
      window.removeEventListener('scroll', updatePosition, true)
    }
  }, [direction, open])

  useEffect(() => {
    if (!open) return
    const handlePointerDown = (event: MouseEvent) => {
      const target = event.target as Node
      if (!menuRef.current?.contains(target) && !buttonRef.current?.contains(target)) {
        setOpen(false)
      }
    }
    document.addEventListener('mousedown', handlePointerDown)
    return () => document.removeEventListener('mousedown', handlePointerDown)
  }, [open])

  return (
    <div className={cn('relative', className)}>
      <button
        ref={buttonRef}
        type="button"
        onClick={() => setOpen((current) => !current)}
        aria-haspopup="menu"
        aria-expanded={open}
        className="inline-flex h-11 w-11 items-center justify-center rounded-[15px] border border-white/[0.08] bg-[#101620] text-white transition-colors hover:bg-white/[0.06]"
      >
        <MoreHorizontal size={18} />
      </button>
      {open && menuPosition
        ? createPortal(
        <div
          ref={menuRef}
          className={cn(
            'fixed z-[1000] overflow-hidden rounded-[18px] border border-white/[0.08] bg-[#0f1520] p-2 shadow-[0_18px_44px_rgba(0,0,0,0.34)] backdrop-blur-xl',
          )}
          style={menuPosition}
        >
          <button
            type="button"
            onClick={() => {
              setOpen(false)
              onRefresh()
            }}
            disabled={refreshBusy}
            className="flex w-full items-center gap-2 rounded-[12px] px-3 py-2.5 text-left text-sm text-white transition-colors hover:bg-white/[0.06] disabled:cursor-not-allowed disabled:opacity-60"
          >
            <Database size={16} />
            {refreshBusy ? 'Refreshing...' : 'Refresh Metadata and Achievements'}
          </button>
          <button
            type="button"
            onClick={() => {
              setOpen(false)
              onReclassify()
            }}
            className="flex w-full items-center gap-2 rounded-[12px] px-3 py-2.5 text-left text-sm text-white transition-colors hover:bg-white/[0.06]"
          >
            <ArrowRightLeft size={16} />
            Reclassify
          </button>
        </div>,
        document.body,
      )
        : null}
    </div>
  )
}

function AchievementPreviewCard({ achievement }: { achievement: AchievementDTO }) {
  const iconUrl = achievement.unlocked ? achievement.unlocked_icon : achievement.locked_icon

  return (
    <article className="overflow-hidden rounded-[18px] border border-white/10 bg-[rgba(28,26,34,0.92)] shadow-[0_12px_24px_rgba(0,0,0,0.18)]">
      <div className="relative aspect-[4/3] overflow-hidden bg-[radial-gradient(circle_at_top,rgba(255,255,255,0.16),transparent_52%),linear-gradient(180deg,rgba(34,44,40,0.96),rgba(22,24,30,0.98))]">
        {iconUrl ? (
          <img
            src={iconUrl}
            alt=""
            aria-hidden="true"
            loading="lazy"
            decoding="async"
            className={cn('h-full w-full object-cover', achievement.unlocked ? '' : 'opacity-55 grayscale')}
          />
        ) : (
          <div className="flex h-full w-full items-center justify-center text-white/72">
            <Trophy size={24} />
          </div>
        )}
        {!achievement.unlocked ? (
          <div className="absolute inset-0 flex items-center justify-center">
            <div className="rounded-full bg-black/55 p-2.5 text-white/88">
              <Trophy size={18} />
            </div>
          </div>
        ) : null}
      </div>
      <div className="space-y-2 p-3">
        <div className="flex flex-wrap items-center gap-2">
          {achievement.unlocked ? <Badge variant="accent">Unlocked</Badge> : <Badge variant="muted">Locked</Badge>}
          {achievement.points !== undefined && achievement.points > 0 ? <Badge>{achievement.points} pts</Badge> : null}
          {achievement.rarity !== undefined && achievement.rarity > 0 ? <Badge>{achievement.rarity.toFixed(1)}%</Badge> : null}
        </div>
        <p className="line-clamp-2 text-base font-semibold leading-5 text-mga-text">{achievement.title}</p>
        <p className="line-clamp-2 text-xs leading-5 text-mga-muted">
          {achievement.description || (achievement.unlocked ? 'Unlocked achievement.' : 'Achievement details unavailable.')}
        </p>
      </div>
    </article>
  )
}

function fileRoleLabel(role: string): string {
  if (role === 'root') return 'Launch / Root'
  return humanizeValue(role)
}

function fileKindLabel(kind: string | undefined): string | null {
  if (!kind) return null
  return humanizeValue(kind)
}

function fileGroupKey(entry: GameFileDisplayRecord): 'primary' | 'package' | 'other' {
  if (entry.isLaunchFile || entry.file.role === 'root') return 'primary'
  const fileKind = entry.file.file_kind?.toLowerCase() ?? ''
  if (
    fileKind === 'archive' ||
    fileKind === 'executable' ||
    fileKind === 'dos_executable' ||
    fileKind === 'disc_image' ||
    fileKind === 'disc_meta'
  ) {
    return 'package'
  }
  return 'other'
}

function compareFileEntries(a: GameFileDisplayRecord, b: GameFileDisplayRecord): number {
  if (a.isLaunchFile !== b.isLaunchFile) return a.isLaunchFile ? -1 : 1
  if (a.file.role !== b.file.role) {
    if (a.file.role === 'root') return -1
    if (b.file.role === 'root') return 1
  }
  if (a.file.path !== b.file.path) return a.file.path.localeCompare(b.file.path)
  return a.sourcePluginId.localeCompare(b.sourcePluginId)
}

function buildGameFileGroups(sourceGames: SourceGameDetailDTO[]) {
  const entries = sourceGames
    .flatMap((source) =>
      source.files.map((file) => ({
        sourceGameId: source.id,
        sourcePluginId: source.plugin_id,
        sourceTitle: source.raw_title || source.external_id,
        isLaunchFile: sourcePrimaryRootFileID(source) === file.id || file.role === 'root',
        file,
      })),
    )
    .sort(compareFileEntries)

  return {
    all: entries,
    primary: entries.filter((entry) => fileGroupKey(entry) === 'primary'),
    package: entries.filter((entry) => fileGroupKey(entry) === 'package'),
    other: entries.filter((entry) => fileGroupKey(entry) === 'other'),
  }
}

function sourcePrimaryRootFileID(source: SourceGameDetailDTO): string | null {
  return source.delivery?.profiles?.[0]?.root_file_id ?? source.play?.root_file_id ?? null
}

function sourceHasBrowserPlayDelivery(source: SourceGameDetailDTO): boolean {
  return (
    source.delivery?.profiles?.some((profile) => profile.mode === 'direct' || profile.mode === 'materialized') ??
    false
  )
}

function GameFileRow({ entry }: { entry: GameFileDisplayRecord }) {
  return (
    <div className="rounded-mga border border-mga-border bg-mga-bg/60 p-3 text-sm shadow-sm shadow-black/5">
      <div className="flex flex-wrap items-start justify-between gap-3">
        <div className="space-y-2">
          <div className="flex flex-wrap items-center gap-2">
            <SourceBadge source={entry.sourcePluginId} />
            <Badge variant="muted">{fileRoleLabel(entry.file.role)}</Badge>
            {entry.isLaunchFile ? <Badge variant="accent">Launchable</Badge> : null}
            {fileKindLabel(entry.file.file_kind) ? <Badge>{fileKindLabel(entry.file.file_kind)}</Badge> : null}
          </div>
          <p className="text-xs font-medium uppercase tracking-wide text-mga-muted">{entry.sourceTitle}</p>
        </div>
        <div className="rounded-mga border border-mga-border bg-mga-surface px-2.5 py-1 text-xs font-medium text-mga-text">
          {formatBytes(entry.file.size)}
        </div>
      </div>
      <div className="mt-3 rounded-mga border border-mga-border/70 bg-mga-surface/70 px-3 py-2">
        <p className="break-all font-mono text-xs leading-6 text-mga-text">{entry.file.path}</p>
      </div>
    </div>
  )
}

function GameFileGroup({
  title,
  description,
  entries,
}: {
  title: string
  description: string
  entries: GameFileDisplayRecord[]
}) {
  if (entries.length === 0) return null

  return (
    <div className="space-y-3 rounded-mga border border-mga-border bg-mga-bg/40 p-4">
      <div className="flex flex-wrap items-center justify-between gap-3">
        <div>
          <h3 className="text-sm font-semibold text-mga-text">{title}</h3>
          <p className="mt-1 text-xs text-mga-muted">{description}</p>
        </div>
        <Badge variant="muted">{entries.length}</Badge>
      </div>
      <div className="space-y-2">
        {entries.map((entry) => (
          <GameFileRow key={`${entry.sourceGameId}:${entry.file.path}:${entry.file.role}`} entry={entry} />
        ))}
      </div>
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
    <div className="rounded-mga border border-mga-border bg-mga-bg/60 p-3 shadow-sm shadow-black/5">
      <div className="flex flex-wrap items-start justify-between gap-3">
        <div className="space-y-2">
          <div className="flex flex-wrap items-center gap-2">
            <SourceBadge source={match.plugin_id} />
            {match.outvoted ? <Badge variant="muted">Outvoted</Badge> : <Badge variant="accent">Active</Badge>}
            {match.xcloud_available ? <BrandBadge brand="xcloud" label="xCloud" /> : null}
            {match.is_game_pass ? <Badge variant="gamepass">Game Pass</Badge> : null}
          </div>
          <p className="text-sm font-semibold text-mga-text">{match.title ?? 'Unknown title'}</p>
        </div>
        {match.url ? (
          <a href={match.url} target="_blank" rel="noreferrer" className="inline-flex items-center gap-1 text-xs font-medium text-mga-accent hover:underline">
            Open match
            <ExternalLink size={12} />
          </a>
        ) : null}
      </div>
      <div className="mt-3 grid gap-2 text-sm md:grid-cols-2">
        <MetaItem label="External ID" value={match.external_id} />
        <MetaItem label="Platform" value={match.platform ? humanizeValue(match.platform) : 'Unknown'} />
      </div>
      {facts.length > 0 && <p className="mt-3 text-xs leading-5 text-mga-muted">{facts.join(' • ')}</p>}
    </div>
  )
}

function sourceRecordLabel(source: SourceGameDetailDTO): string {
  return `${source.integration_label || source.integration_id} · ${source.raw_title || source.external_id}`
}

function SourceRecordCard({
  source,
  onHardDelete,
}: {
  source: SourceGameDetailDTO
  onHardDelete: (source: SourceGameDetailDTO) => void
}) {
  const browserPlayable = sourceHasBrowserPlayDelivery(source)
  const hardDeleteEligible = source.hard_delete?.eligible ?? false

  return (
    <article className="space-y-4 rounded-mga border border-mga-border bg-mga-bg/60 p-4 shadow-sm shadow-black/5">
      <div className="flex flex-wrap items-start justify-between gap-3">
        <div className="space-y-2">
          <div className="flex flex-wrap items-center gap-2">
            <SourceBadge source={source.plugin_id} />
            <Badge variant="source">{source.status}</Badge>
            <Badge variant="platform"><PlatformIcon platform={source.platform} showLabel /></Badge>
            {browserPlayable ? <Badge variant="accent">Browser Play</Badge> : null}
          </div>
          <div>
            <p className="text-sm font-semibold text-mga-text">{source.raw_title || source.external_id}</p>
            <p className="mt-1 text-xs text-mga-muted">{source.external_id}</p>
          </div>
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
        <MetaItem label="Integration" value={source.integration_label || source.integration_id} />
        <MetaItem label="Kind" value={source.kind} />
        <MetaItem label="Created" value={formatDateTimeValue(source.created_at)} />
        <MetaItem label="Last Seen" value={formatDateTimeValue(source.last_seen_at)} />
        <MetaItem label="Root Path" value={source.root_path ?? 'Unknown'} />
        <MetaItem label="Files" value={source.files.length} />
        <MetaItem label="Resolver Matches" value={source.resolver_matches.length} />
      </div>

      {hardDeleteEligible ? (
        <div className="flex justify-end">
          <Button
            type="button"
            variant="outline"
            onClick={() => onHardDelete(source)}
            className="border-red-500/30 text-red-200 hover:bg-red-500/10"
          >
            <FileText size={16} />
            Hard Delete Source Record
          </Button>
        </div>
      ) : null}

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
          Source File Inventory ({source.files.length})
        </summary>
        <div className="mt-3 space-y-2">
          {source.files.length === 0 ? (
            <p className="text-sm text-mga-muted">No files associated with this source game.</p>
          ) : (
            source.files.map((file) => (
              <GameFileRow
                key={`${source.id}:${file.path}:${file.role}`}
                entry={{
                  sourceGameId: source.id,
                  sourcePluginId: source.plugin_id,
                  sourceTitle: source.raw_title || source.external_id,
                  isLaunchFile: sourcePrimaryRootFileID(source) === file.id || file.role === 'root',
                  file,
                }}
              />
            ))
          )}
        </div>
      </details>
    </article>
  )
}

function MediaPreview({ media }: { media: GameMediaDetailDTO }) {
  const url = mediaUrl(media)
  const youtubeUrl = youtubeEmbedUrl(media)
  const mediaCollection = new GameMediaCollection([media])

  if (youtubeUrl) {
    return (
      <iframe
        src={youtubeUrl}
        title={`${mediaTypeLabel(media.type)} preview`}
        allow="accelerometer; autoplay; clipboard-write; encrypted-media; gyroscope; picture-in-picture"
        allowFullScreen
        className="h-[360px] w-full rounded-mga border border-mga-border bg-black"
      />
    )
  }
  if (mediaCollection.isInlineVideo(media)) {
    return (
      <video controls preload="metadata" className="max-h-[360px] w-full rounded-mga border border-mga-border bg-black">
        <source src={url} type={media.mime_type} />
      </video>
    )
  }
  if (mediaCollection.isInlineAudio(media)) {
    return (
      <div className="rounded-mga border border-mga-border bg-mga-surface p-4">
        <audio controls preload="metadata" className="w-full">
          <source src={url} type={media.mime_type} />
        </audio>
      </div>
    )
  }
  if (mediaCollection.isPdf(media)) {
    return <iframe src={url} title={`${mediaTypeLabel(media.type)} preview`} className="h-[360px] w-full rounded-mga border border-mga-border bg-white" />
  }
  if (mediaCollection.isInlineDocument(media)) {
    return <iframe src={url} title={`${mediaTypeLabel(media.type)} preview`} className="h-[360px] w-full rounded-mga border border-mga-border bg-mga-surface" />
  }
  return (
    <div className="flex items-center gap-2 rounded-mga border border-dashed border-mga-border bg-mga-surface px-3 py-4 text-sm text-mga-muted">
      {youtubeUrl || media.mime_type?.startsWith('video/') ? <Video size={16} /> : <FileText size={16} />}
      {`${mediaTypeLabel(media.type)} cannot be previewed inline in the browser. Use the external link above.`}
    </div>
  )
}

function MediaViewerDialog({ media, onClose }: { media: GameMediaDetailDTO | null; onClose: () => void }) {
  const mediaCollection = new GameMediaCollection(media ? [media] : [])
  const youtubeUrl = media ? youtubeEmbedUrl(media) : null
  return (
    <Dialog open={media !== null} onClose={onClose} title={media ? mediaTypeLabel(media.type) : 'Media'} className="max-w-5xl">
      {media && (
        <div className="space-y-4">
          <div className="overflow-hidden rounded-mga border border-mga-border bg-mga-bg">
            {mediaCollection.isImage(media) ? (
              <img src={mediaUrl(media)} alt={mediaTypeLabel(media.type)} className="max-h-[75vh] w-full object-contain" />
            ) : youtubeUrl ? (
              <iframe
                src={youtubeUrl}
                title={mediaTypeLabel(media.type)}
                allow="accelerometer; autoplay; clipboard-write; encrypted-media; gyroscope; picture-in-picture"
                allowFullScreen
                className="aspect-video w-full bg-black"
              />
            ) : mediaCollection.isInlineVideo(media) ? (
              <video controls preload="metadata" className="max-h-[75vh] w-full bg-black object-contain">
                <source src={mediaUrl(media)} type={media.mime_type} />
              </video>
            ) : (
              <MediaPreview media={media} />
            )}
          </div>
          <div className="flex flex-wrap items-center gap-2 text-sm text-mga-muted">
            <Badge>{mediaTypeLabel(media.type)}</Badge>
            {media.source && <SourceBadge source={media.source} />}
            {media.width && media.height && <span>{media.width} × {media.height}</span>}
            <a href={mediaOriginalUrl(media)} target="_blank" rel="noreferrer" className="inline-flex items-center gap-1 font-medium text-mga-accent hover:underline">
              Open original
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
  const { setFavorite, isPendingFor } = useGameFavoriteAction()
  const { recordLaunch } = useRecentPlayed()
  const [selectedMedia, setSelectedMedia] = useState<GameMediaDetailDTO | null>(null)
  const [refreshBusy, setRefreshBusy] = useState(false)
  const [refreshNotice, setRefreshNotice] = useState('')
  const [refreshError, setRefreshError] = useState('')
  const [deleteTarget, setDeleteTarget] = useState<SourceGameDetailDTO | null>(null)
  const [deleteBusy, setDeleteBusy] = useState(false)
  const [deleteNotice, setDeleteNotice] = useState('')
  const [showFloatingActions, setShowFloatingActions] = useState(false)
  const [deleteError, setDeleteError] = useState('')
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
  const mediaCollection = useMemo(() => new GameMediaCollection(gameData?.media), [gameData?.media])
  const coverMedia = useMemo(() => gameData?.cover_override ?? mediaCollection.cover(), [gameData?.cover_override, mediaCollection])
  const heroMedia = useMemo(() => mediaCollection.hero(), [mediaCollection])
  const mergedMedia = useMemo(() => mergeDisplayMedia(gameData?.media), [gameData?.media])
  const coverUrl = coverMedia ? mediaUrl(coverMedia) : null
  const featuredMedia = useMemo(() => buildFeaturedMediaRail(mergedMedia, 12).map((item) => item.media), [mergedMedia])
  const heroBackdropMedia = useMemo(
    () => gameData?.background_override ?? heroMedia ?? coverMedia ?? featuredMedia[0] ?? null,
    [coverMedia, featuredMedia, gameData?.background_override, heroMedia],
  )
  const heroBackdropUrl = heroBackdropMedia ? mediaUrl(heroBackdropMedia) : null
  const heroBackgroundSuitability = useMemo(
    () => (heroBackdropMedia ? evaluateBackgroundSuitability(heroBackdropMedia) : null),
    [heroBackdropMedia],
  )
  const [selectedBrowserSourceId, setSelectedBrowserSourceId] = useState('')
  const [selectedXcloudOptionKey, setSelectedXcloudOptionKey] = useState('')
  const browserSupported = gameData ? hasBrowserPlaySupport(gameData) : false
  const browserPlayResolution = useMemo(() => {
    if (!gameData) return null
    const runtime = getBrowserPlayPreferenceRuntime(gameData)
    const rememberedSourceId = selectedBrowserSourceId
      ? null
      : (runtime ? readBrowserPlaySourcePreference(gameData.id, runtime) : null)
    return resolveBrowserPlaySelection(gameData, {
      requestedSourceGameId: selectedBrowserSourceId || null,
      rememberedSourceGameId: rememberedSourceId,
    })
  }, [gameData, selectedBrowserSourceId])
  const browserPlaySelections = browserPlayResolution?.selections ?? []
  const selectedBrowserSelection = browserPlayResolution?.selection ?? null
  const serverLaunchOptions = gameData?.play?.options?.filter((option) => option.launchable) ?? []
  const xcloudLaunchOptions = serverLaunchOptions.filter((option) => option.kind === 'xcloud' && option.url)
  const primaryXcloudOption =
    xcloudLaunchOptions.find((option) => launchOptionKey(option) === selectedXcloudOptionKey) ??
    xcloudLaunchOptions[0] ??
    null
  const browserPlayable = browserPlaySelections.length > 0
  const browserPlayIssue = browserPlayResolution?.issue ?? null
  const browserPlayRuntime = browserPlayResolution?.runtime ?? null
  const sources = gameData ? selectSourcePlugins(gameData) : []
  const resolverCount = gameData
    ? gameData.source_games.reduce(
        (total, sourceGame) => total + sourceGame.resolver_matches.length,
        0,
      )
    : 0
  const externalLinks = useMemo(() => buildExternalLinks(gameData?.external_ids), [gameData?.external_ids])
  const fileGroups = useMemo(() => buildGameFileGroups(gameData?.source_games ?? []), [gameData?.source_games])
  const metadataSources = useMemo(
    () => collectUnifiedMetadataSources(gameData?.source_games ?? [], gameData?.media),
    [gameData?.media, gameData?.source_games],
  )
  const achievementSets = useMemo(() => (achievements.data ?? []).map(sortAchievementSet), [achievements.data])
  const launchableSourceCount = browserPlaySelections.length
  const heroDescription = useMemo(() => splitHeroDescription(gameData?.description), [gameData?.description])
  const playModeLabel =
    gameData?.max_players && gameData.max_players > 1
      ? `Multiplayer (${gameData.max_players})`
      : gameData?.max_players === 1
        ? 'Single Player'
        : 'Unknown'
  const availabilityLabel = browserPlayable
    ? 'Browser Play'
    : gameData?.xcloud_available
      ? 'xCloud'
      : gameData?.is_game_pass
        ? 'Game Pass'
        : platformLabel(gameData?.platform ?? '')
  useEffect(() => {
    hasRetried404Ref.current = false
  }, [id])

  useEffect(() => {
    setSelectedBrowserSourceId('')
    setSelectedXcloudOptionKey('')
  }, [id])

  useEffect(() => {
    if (!achievements.isSuccess) return
    void queryClient.invalidateQueries({ queryKey: ['games'] })
  }, [achievements.isSuccess, queryClient])

  useEffect(() => {
    const updateVisibility = () => {
      const threshold = window.innerHeight * 0.5
      setShowFloatingActions(window.scrollY > threshold)
    }
    updateVisibility()
    window.addEventListener('scroll', updateVisibility, { passive: true })
    window.addEventListener('resize', updateVisibility)
    return () => {
      window.removeEventListener('scroll', updateVisibility)
      window.removeEventListener('resize', updateVisibility)
    }
  }, [])

  useEffect(() => {
    if (!gameData || !browserPlayRuntime || !browserPlayResolution?.invalidRememberedSourceGameId) return
    clearBrowserPlaySourcePreference(gameData.id, browserPlayRuntime)
  }, [browserPlayResolution?.invalidRememberedSourceGameId, browserPlayRuntime, gameData])

  useEffect(() => {
    if (!game.data || id.length === 0 || game.data.id === id) return
    navigate(`/game/${encodeURIComponent(game.data.id)}`, {
      replace: true,
      state: location.state,
    })
  }, [game.data, id, location.state, navigate])

  const handleLaunchXcloud = (option?: GameLaunchOptionDTO | null) => {
    const currentGame = game.data
    const launchUrl = option?.url ?? currentGame?.xcloud_url
    if (!currentGame) return
    if (!launchUrl) return
    recordLaunch({
      gameId: currentGame.id,
      title: currentGame.title,
      platform: currentGame.platform,
      coverUrl,
      launchKind: 'xcloud',
      launchUrl,
    })
  }

  const handleLaunchBrowser = () => {
    const currentGame = game.data
    if (!currentGame || !browserPlayResolution?.canLaunch || !selectedBrowserSelection) return
    const params = new URLSearchParams()
    params.set('source', selectedBrowserSelection.sourceGame.id)
    navigate(
      {
        pathname: `/game/${encodeURIComponent(currentGame.id)}/play`,
        search: params.toString() ? `?${params.toString()}` : '',
      },
      { state: location.state },
    )
  }

  const handleBack = () => {
    const shouldRestoreScroll = from.startsWith('/play') || from.startsWith('/library')
    navigate(from, shouldRestoreScroll ? { state: { restoreScroll: true } } : undefined)
  }

  const handleBrowserSourceChange = (sourceGameId: string) => {
    setSelectedBrowserSourceId(sourceGameId)
    if (!gameData || !browserPlayRuntime || !sourceGameId) return
    writeBrowserPlaySourcePreference(gameData.id, browserPlayRuntime, sourceGameId)
  }

  const handleReclassify = () => {
    if (!game.data) return
    const params = new URLSearchParams()
    params.set('tab', 'undetected')
    const candidateId = game.data.source_games[0]?.id
    if (candidateId) {
      params.set('candidate_id', candidateId)
    }
    params.set('reclassify_game_id', game.data.id)
    params.set('reclassify_title', game.data.title)
    params.set('reclassify_platform', game.data.platform)
    const primarySource = game.data.source_games[0]?.plugin_id
    if (primarySource) {
      params.set('reclassify_source', primarySource)
    }
    navigate({ pathname: '/settings', search: params.toString() })
  }

  const handleToggleFavorite = async () => {
    if (!game.data) return
    await setFavorite({
      gameId: game.data.id,
      favorite: !game.data.favorite,
    })
  }

  const handleRefreshMetadata = async () => {
    if (!game.data || refreshBusy) return
    setRefreshBusy(true)
    setRefreshNotice('')
    setRefreshError('')
    try {
      const refreshed = await refreshGameMetadata(game.data.id)
      queryClient.setQueryData(['game', refreshed.id], refreshed)
      await Promise.all([
        queryClient.invalidateQueries({ queryKey: ['games'] }),
        queryClient.invalidateQueries({ queryKey: ['game', refreshed.id, 'achievements'] }),
      ])
      setRefreshNotice('Metadata and media refresh completed.')
    } catch (error) {
      const message =
        error instanceof ApiError
          ? error.responseText?.trim() || error.message
          : (error instanceof Error ? error.message : 'Refresh failed.')
      setRefreshError(message)
    } finally {
      setRefreshBusy(false)
    }
  }

  const handleRequestHardDelete = (source: SourceGameDetailDTO) => {
    setDeleteError('')
    setDeleteNotice('')
    setDeleteTarget(source)
  }

  const handleConfirmHardDelete = async () => {
    if (!game.data || !deleteTarget || deleteBusy) return
    setDeleteBusy(true)
    setDeleteError('')
    setDeleteNotice('')
    try {
      const result = await deleteSourceGame(game.data.id, deleteTarget.id)
      await Promise.all([
        queryClient.invalidateQueries({ queryKey: ['games'] }),
        queryClient.invalidateQueries({ queryKey: ['game', game.data.id, 'achievements'] }),
      ])
      if (result.game) {
        queryClient.setQueryData(['game', result.game.id], result.game)
      }
      if (result.canonical_exists && result.game) {
        setDeleteNotice(`Deleted ${sourceRecordLabel(deleteTarget)}.`)
        if (result.game.id !== game.data.id) {
          navigate(`/game/${encodeURIComponent(result.game.id)}`, {
            replace: true,
            state: location.state,
          })
        } else {
          queryClient.setQueryData(['game', game.data.id], result.game)
        }
      } else {
        navigate('/library', { replace: true })
      }
      setDeleteTarget(null)
    } catch (error) {
      const message =
        error instanceof ApiError
          ? error.responseText?.trim() || error.message
          : (error instanceof Error ? error.message : 'Hard delete failed.')
      setDeleteError(message)
    } finally {
      setDeleteBusy(false)
    }
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
  const favoriteBusy = isPendingFor(data.id)
  return (
    <div className="w-full space-y-8 pb-32 md:pb-36">
      <section
        id="overview"
        className="relative isolate min-h-[560px] overflow-hidden bg-[#050608] md:min-h-[620px] xl:min-h-[700px]"
      >
        <div className="absolute inset-0 bg-[#050608]" />
        {heroBackdropUrl ? (
          <>
            {heroBackgroundSuitability?.level === 'good' ? (
              <img
                src={heroBackdropUrl}
                alt=""
                className="absolute inset-0 h-full w-full object-cover object-center"
                aria-hidden="true"
              />
            ) : (
              <>
                <img
                  src={heroBackdropUrl}
                  alt=""
                  className="absolute inset-0 h-full w-full scale-[1.12] object-cover object-center opacity-55 blur-[28px]"
                  aria-hidden="true"
                />
                <img
                  src={heroBackdropUrl}
                  alt=""
                  className="absolute right-[clamp(32px,7vw,140px)] top-1/2 z-0 max-h-[78%] w-[min(44vw,620px)] -translate-y-1/2 rounded-[28px] object-contain object-center opacity-72"
                  aria-hidden="true"
                />
              </>
            )}
            <div
              className="absolute inset-0 pointer-events-none"
              style={{
                background:
                  'linear-gradient(90deg, rgba(4,8,20,0.96) 0%, rgba(4,8,20,0.88) 28%, rgba(4,8,20,0.60) 50%, rgba(4,8,20,0.24) 76%, rgba(4,8,20,0.08) 100%)',
              }}
            />
            <div
              className="absolute inset-0 pointer-events-none"
              style={{
                background:
                  'radial-gradient(circle at 72% 28%, rgba(64,126,255,0.08), transparent 40%), linear-gradient(180deg, rgba(0,0,0,0.22) 0%, rgba(0,0,0,0.04) 58%, rgba(0,0,0,0.00) 100%)',
              }}
            />
          </>
        ) : null}

        <div className="relative z-10 mx-auto max-w-[1540px] space-y-6 px-4 pb-10 pt-4 md:px-6 xl:px-8 xl:pb-12 xl:pt-5">
          <Button
            variant="outline"
            size="sm"
            onClick={handleBack}
            className="mb-4 rounded-[14px] border-white/[0.08] bg-black/18 text-white backdrop-blur-[6px] hover:bg-black/30"
          >
            <ArrowLeft size={14} />
            {originLabel}
          </Button>

          <div className="min-h-[460px] md:min-h-[520px] xl:min-h-[600px]">
            <div className="min-w-0 flex max-w-[42rem] flex-col gap-6 lg:gap-7">
              <div className="space-y-4">
                <div className="space-y-3">
                  <h1 className="max-w-3xl text-4xl font-semibold tracking-tight text-white md:text-5xl xl:text-[4.15rem] xl:leading-[1.02]">
                    {data.title}
                  </h1>
                  <AttributionNote sources={metadataSources} prefix="Metadata gathered from" />
                </div>
                {heroDescription.tagline ? (
                  <p className="max-w-2xl text-[18px] font-medium leading-8 text-white/90">
                    {heroDescription.tagline}
                  </p>
                ) : null}
                <p className="max-w-2xl text-sm leading-8 text-white/74 md:text-base">
                  {heroDescription.body || 'No description is available for this game yet.'}
                </p>
              </div>

              {browserPlayable && (browserPlaySelections.length > 1 || browserPlayIssue?.code === 'invalid_remembered_source') ? (
                <div className="max-w-2xl rounded-[24px] bg-black/18 p-4 backdrop-blur-[6px]">
                  <label className="mb-2 block text-xs uppercase tracking-[0.18em] text-white/42">Launch source</label>
                  <select
                    value={selectedBrowserSelection?.sourceGame.id ?? selectedBrowserSourceId}
                    onChange={(event) => handleBrowserSourceChange(event.target.value)}
                    className="w-full rounded-[16px] border border-white/10 bg-[#121a27] px-3 py-2.5 text-sm text-mga-text"
                  >
                    {!selectedBrowserSelection ? (
                      <option value="" disabled>
                        Choose a source to enable launch
                      </option>
                    ) : null}
                    {browserPlaySelections.map((selection) => (
                      <option key={selection.sourceGame.id} value={selection.sourceGame.id}>
                        {browserPlaySourceOptionLabel(selection, browserPlaySelections)}
                      </option>
                    ))}
                  </select>
                  <p className="mt-2 text-xs text-white/58">
                    {selectedBrowserSelection
                      ? browserPlaySourceContext(selectedBrowserSelection)
                      : (browserPlayIssue?.message ?? 'Choose the source or version to launch.')}
                  </p>
                </div>
              ) : null}

              {xcloudLaunchOptions.length > 1 ? (
                <div className="max-w-2xl rounded-[24px] bg-black/18 p-4 backdrop-blur-[6px]">
                  <label className="mb-2 block text-xs uppercase tracking-[0.18em] text-white/42">xCloud source</label>
                  <select
                    value={primaryXcloudOption ? launchOptionKey(primaryXcloudOption) : ''}
                    onChange={(event) => setSelectedXcloudOptionKey(event.target.value)}
                    className="w-full rounded-[16px] border border-white/10 bg-[#121a27] px-3 py-2.5 text-sm text-mga-text"
                  >
                    {xcloudLaunchOptions.map((option) => (
                      <option key={launchOptionKey(option)} value={launchOptionKey(option)}>
                        {launchOptionContext(option) || 'xCloud'}
                      </option>
                    ))}
                  </select>
                </div>
              ) : null}

              <div className="flex flex-wrap gap-3">
                {browserPlayable ? (
                  <HeroActionButton
                    type="button"
                    onClick={handleLaunchBrowser}
                    disabled={!browserPlayResolution?.canLaunch}
                    primary
                    className="min-w-[10rem] px-6"
                  >
                    <PlayCircle size={17} />
                    Play
                  </HeroActionButton>
                ) : null}
                {(primaryXcloudOption?.url ?? data.xcloud_url) ? (
                  <a
                    href={primaryXcloudOption?.url ?? data.xcloud_url}
                    target="_blank"
                    rel="noreferrer"
                    onClick={() => handleLaunchXcloud(primaryXcloudOption)}
                    className="inline-flex h-11 items-center justify-center gap-2 rounded-[15px] border border-white/[0.08] bg-[#101620] px-5 text-sm font-medium text-white transition-colors hover:bg-white/[0.06]"
                  >
                    <BrandIcon brand="xcloud" className="h-4 w-4 invert" />
                    Play xCloud
                  </a>
                ) : null}
                <FavoriteActionButton
                  favorite={data.favorite}
                  busy={favoriteBusy}
                  onClick={() => void handleToggleFavorite()}
                />
                <HeroOverflowMenu onRefresh={handleRefreshMetadata} onReclassify={handleReclassify} refreshBusy={refreshBusy} />
              </div>

              {browserSupported && !browserPlayable ? (
                <p className="text-xs text-white/58">
                  {browserPlayIssue?.message ?? 'Browser Play is supported for this platform, but no launchable source file was found yet.'}
                </p>
              ) : null}
              {browserSupported && browserPlayable && !browserPlayResolution?.canLaunch && browserPlayIssue ? (
                <p className="text-xs text-amber-300">{browserPlayIssue.message}</p>
              ) : null}
              {refreshBusy ? (
                <div className="flex max-w-xl items-center gap-3 rounded-[18px] border border-sky-300/20 bg-sky-300/[0.08] px-4 py-3 text-sm text-sky-50 shadow-[0_12px_28px_rgba(0,0,0,0.2)]">
                  <Loader2 size={18} className="shrink-0 animate-spin text-sky-200" />
                  <div className="min-w-0">
                    <p className="font-medium">Refreshing metadata and achievements...</p>
                    <p className="text-xs text-sky-100/72">
                      Updating provider matches, media, and achievement progress. This can take a moment.
                    </p>
                  </div>
                </div>
              ) : null}
              {refreshNotice ? <p className="text-xs text-green-400">{refreshNotice}</p> : null}
              {deleteNotice ? <p className="text-xs text-green-400">{deleteNotice}</p> : null}
              {refreshError ? <p className="text-xs text-red-400">{refreshError}</p> : null}
              {deleteError ? <p className="text-xs text-red-400">{deleteError}</p> : null}
            </div>
          </div>
        </div>
      </section>

      <div className="mx-auto max-w-[1540px] space-y-8 px-4 md:px-6 lg:px-8">
        <section className="game-info-bar grid gap-px overflow-hidden rounded-[24px] bg-white/[0.05]">
          <div className="grid gap-px bg-white/[0.05] lg:grid-cols-5">
            <div className="min-w-0 bg-[rgba(10,14,22,0.92)]">
              <HeroFactStripItem label="Released" value={formatDateValue(data.release_date)} />
            </div>
            <div className="min-w-0 bg-[rgba(10,14,22,0.92)]">
              <HeroFactStripItem label="Developer" value={detailValue(data.developer)} />
            </div>
            <div className="min-w-0 bg-[rgba(10,14,22,0.92)]">
              <HeroFactStripItem label="Publisher" value={detailValue(data.publisher)} />
            </div>
            <div className="min-w-0 bg-[rgba(10,14,22,0.92)]">
              <HeroFactStripItem label="Play Mode" value={playModeLabel} />
            </div>
            <div className="min-w-0 bg-[rgba(10,14,22,0.92)]">
              <HeroFactStripItem
                label="Availability"
                value={availabilityLabel}
                detail={data.rating ? `Rating ${data.rating.toFixed(1)}` : undefined}
              />
            </div>
          </div>
        </section>

        <section className="featured-media space-y-3">
          <div className="featured-media__header flex items-center justify-between gap-3 px-1">
            <p className="text-[11px] font-medium uppercase tracking-[0.18em] text-white/42">
              Featured Media
            </p>
            <Button
              type="button"
              variant="outline"
              size="sm"
              onClick={() => navigate(`/game/${encodeURIComponent(data.id)}/media`, { state: location.state })}
              className="h-9 rounded-[14px] border-white/[0.08] bg-black/18 text-white backdrop-blur-[6px] hover:bg-black/28"
            >
              Open Gallery
            </Button>
          </div>
          {featuredMedia.length > 0 ? (
            <div className="featured-media__rail -mx-1 max-w-full overflow-x-auto px-1 pb-1">
              <div className="featured-media__items flex w-max max-w-none gap-3">
                {featuredMedia.map((media) => (
                  <HeroMediaThumb
                    key={mediaItemKey(media)}
                    media={media}
                    label={mediaTypeLabel(media.type)}
                    onSelect={(item) => setSelectedMedia(item)}
                  />
                ))}
              </div>
            </div>
          ) : null}
        </section>

        <nav className="sticky top-2 z-20 overflow-x-auto rounded-[24px] border border-white/[0.05] bg-[rgba(10,14,22,0.82)] px-3 py-3 backdrop-blur-xl">
          <div className="flex min-w-max gap-2">
            <HeroTabLink href="#details" label="Details" />
            {data.completion_time?.main_story || data.completion_time?.main_extra || data.completion_time?.completionist ? (
              <HeroTabLink href="#howlongtobeat" label="HowLongToBeat" />
            ) : null}
            {achievementSets.length > 0 ? <HeroTabLink href="#achievements" label="Achievements" /> : null}
            <HeroTabLink href="#game-files" label="Files" />
            <HeroTabLink href="#source-records" label="Sources" />
            {externalLinks.length > 0 ? <HeroTabLink href="#external-links" label="Links" /> : null}
          </div>
        </nav>

        <div className="space-y-6">
          <div id="details" className="grid gap-6 xl:grid-cols-[minmax(0,1.15fr)_minmax(0,1fr)_minmax(320px,0.95fr)]">
            <SectionCard
              title="About This Game"
              icon={<FolderOpen size={18} className="text-mga-accent" />}
              description="High-level metadata surfaced for this canonical game."
            >
              <div className="grid gap-3 sm:grid-cols-2">
                <MetaItem
                  label="Genres"
                  value={data.genres && data.genres.length > 0 ? data.genres.join(', ') : 'Unknown'}
                />
                <MetaItem
                  label="Players"
                  value={data.max_players ? `${data.max_players}` : 'Unknown'}
                />
              </div>
            </SectionCard>

            <SectionCard
              title="Game Info"
              icon={<Database size={18} className="text-mga-accent" />}
              description="Core fields kept on the canonical game."
            >
              <div className="grid gap-3 sm:grid-cols-2">
                <MetaItem label="Developer" value={detailValue(data.developer)} />
                <MetaItem label="Publisher" value={detailValue(data.publisher)} />
                <MetaItem label="Release Date" value={formatDateValue(data.release_date)} />
                <MetaItem label="Platform" value={platformLabel(data.platform)} />
                <MetaItem label="Play Mode" value={playModeLabel} />
                <MetaItem label="Rating" value={data.rating ? data.rating.toFixed(1) : 'Unknown'} />
              </div>
            </SectionCard>

            <SectionCard
              title="Availability & Sources"
              icon={<Database size={18} className="text-mga-accent" />}
              description="Current launch/runtime availability and source-backed coverage."
            >
              <div className="space-y-4">
                <div className="flex flex-wrap gap-2">
                  <Badge variant="platform"><PlatformIcon platform={data.platform} showLabel /></Badge>
                  {sources.map((source) => <SourceBadge key={source} source={source} className="bg-white/5 text-white" />)}
                  {data.xcloud_available ? <BrandBadge brand="xcloud" label="xCloud" /> : null}
                  {data.is_game_pass ? <Badge variant="gamepass">Game Pass</Badge> : null}
                  {browserPlayable ? <Badge variant="playable">Browser Play</Badge> : null}
                </div>
                <div className="grid gap-3 sm:grid-cols-2">
                  <MetaItem label="Launchable Sources" value={launchableSourceCount} />
                  <MetaItem label="Resolver Matches" value={resolverCount} />
                  <MetaItem label="Files" value={fileGroups.all.length} />
                  <MetaItem label="Canonical ID" value={data.id} />
                </div>
              </div>
            </SectionCard>
          </div>

        {(data.completion_time?.main_story || data.completion_time?.main_extra || data.completion_time?.completionist) ? (
          <SectionCard
            id="howlongtobeat"
            title="HowLongToBeat"
            icon={<PlayCircle size={18} className="text-mga-accent" />}
            description="Estimated completion times sourced from the current metadata provider."
          >
            <div className="grid gap-3 md:grid-cols-4">
              <HeroStatCard label="Main Story" value={formatHours(data.completion_time?.main_story)} />
              <HeroStatCard label="Main + Extras" value={formatHours(data.completion_time?.main_extra)} />
              <HeroStatCard label="Completionist" value={formatHours(data.completion_time?.completionist)} />
              <HeroStatCard label="Estimate Source" value={data.completion_time?.source ? pluginLabel(data.completion_time.source) : 'Unknown'} />
            </div>
          </SectionCard>
        ) : null}

        <SectionCard
          id="achievements"
          title="Achievements"
          icon={<Trophy size={18} className="text-mga-accent" />}
          description="Cached achievement progress grouped by connected achievement system."
        >
          {achievements.isPending ? (
            <p className="text-sm text-white/58">Loading achievements...</p>
          ) : achievements.isError ? (
            <div className="rounded-mga border border-red-500/30 bg-red-500/10 p-4">
              <p className="text-sm text-red-400">{achievements.error.message}</p>
            </div>
          ) : achievementSets.length > 0 ? (
            <div className="space-y-6">
              {achievementSets.map((set) => (
                <div key={`${set.source}:${set.external_game_id}:${set.source_game_id ?? ''}`} className="space-y-4">
                  <div className="grid gap-4 xl:grid-cols-[minmax(210px,240px)_repeat(4,minmax(0,1fr))]">
                    <div className="rounded-[24px] border border-white/8 bg-[linear-gradient(180deg,rgba(72,104,236,0.95),rgba(51,75,171,0.96))] p-5 text-white shadow-[0_18px_40px_rgba(0,0,0,0.22)]">
                      <div className="flex h-full flex-col items-center justify-between gap-5 text-center">
                        <div className="flex flex-col items-center gap-2">
                          <SourceBadge source={set.source} className="border-white/20 bg-white/10 text-white" />
                          <div>
                            <p className="text-sm font-semibold text-white">{achievementSetTitle(set)}</p>
                            {achievementSetContext(set) ? (
                              <p className="mt-1 text-xs leading-5 text-white/70">{achievementSetContext(set)}</p>
                            ) : null}
                          </div>
                        </div>
                        <div className="space-y-1">
                          <p className="text-sm font-medium text-white/84">Achievement summary</p>
                          <p className="text-3xl font-semibold">{set.unlocked_count}/{set.total_count}</p>
                        </div>
                        <div className="flex justify-center">
                          <AchievementProgressRing
                            summary={{
                              source_count: 1,
                              total_count: set.total_count,
                              unlocked_count: set.unlocked_count,
                              total_points: set.total_points,
                              earned_points: set.earned_points,
                            }}
                            size={72}
                            strokeWidth={6}
                            showLabel={false}
                            className="text-white"
                          />
                        </div>
                        <div className="w-full border-t border-white/20 pt-4">
                          <p className="text-sm text-white/84">
                            {set.total_points !== undefined && set.total_points > 0
                              ? `${set.earned_points ?? 0}/${set.total_points} points`
                              : 'Points unavailable'}
                          </p>
                        </div>
                      </div>
                    </div>
                    {set.achievements.slice(0, 4).map((achievement) => (
                      <AchievementPreviewCard key={`${set.source}:${set.external_game_id}:${achievement.external_id}`} achievement={achievement} />
                    ))}
                  </div>
                  {set.achievements.length > 4 ? (
                    <details className="rounded-[22px] border border-white/8 bg-[#101622] px-4 py-3">
                      <summary className="cursor-pointer list-none text-sm font-medium text-white">
                        View all achievements ({set.achievements.length})
                      </summary>
                      <div className="mt-4 space-y-3">
                        {set.achievements.map((achievement) => (
                          <AchievementRow key={`${set.source}:${set.external_game_id}:${achievement.external_id}`} achievement={achievement} />
                        ))}
                      </div>
                    </details>
                  ) : null}
                </div>
              ))}
            </div>
          ) : (
            <p className="text-sm text-white/58">No achievements are available for this game.</p>
          )}
        </SectionCard>

        <SectionCard
            id="game-files"
            title="Game Files"
            icon={<HardDrive size={18} className="text-mga-accent" />}
            description="Files are grouped by launch importance so the primary runtime path is visible at a glance."
            actions={
              <div className="flex flex-wrap gap-2">
                <Badge variant="muted">{fileGroups.all.length} total</Badge>
                {fileGroups.primary.length > 0 ? <Badge variant="accent">{fileGroups.primary.length} launch/root</Badge> : null}
              </div>
            }
          >
            {fileGroups.all.length > 0 ? (
              <div className="space-y-5">
                <div className="grid gap-3 md:grid-cols-3">
                  <HeroStatCard label="Launchable" value={fileGroups.primary.length} detail="Primary launch or root files" />
                  <HeroStatCard label="Packages" value={fileGroups.package.length} detail="Installers, archives, and disc images" />
                  <HeroStatCard label="Supporting" value={fileGroups.other.length} detail="Required or optional supporting files" />
                </div>
                <GameFileGroup
                  title="Primary Files"
                  description="Launchable or root files for the stored source records."
                  entries={fileGroups.primary}
                />
                <GameFileGroup
                  title="Installer / Package Files"
                  description="Archives, disc images, or executable package files that are likely part of installation or packaging."
                  entries={fileGroups.package}
                />
                <GameFileGroup
                  title="Supporting Files"
                  description="Required or optional supporting files that belong to the stored source records."
                  entries={fileGroups.other}
                />
              </div>
            ) : (
              <p className="text-sm text-white/58">No source files are available for this game yet.</p>
            )}
          </SectionCard>

        <SectionCard
          id="source-records"
          title="Source Records"
          icon={<Database size={18} className="text-mga-accent" />}
          description="Provider-specific records, resolver matches, and per-source file details remain available for inspection."
        >
          <div className="space-y-4">
            {data.source_games.length === 0 ? (
              <p className="text-sm text-white/58">No source records are stored for this game.</p>
            ) : (
              data.source_games.map((source) => (
                <SourceRecordCard
                  key={source.id}
                  source={source}
                  onHardDelete={handleRequestHardDelete}
                />
              ))
            )}
          </div>
        </SectionCard>

        {externalLinks.length > 0 ? (
          <SectionCard
            id="external-links"
            title="External Links"
            icon={<ExternalLink size={18} className="text-mga-accent" />}
            description="Known provider pages tied to the current canonical game."
          >
            <div className="space-y-3">
              {externalLinks.map((link) => <ExternalLinkCard key={link.id} link={link} />)}
            </div>
          </SectionCard>
        ) : null}

      </div>

      <div
        className={cn(
          'fixed inset-x-0 bottom-0 z-30 px-4 pb-4 transition-all duration-200 md:px-6 lg:px-8',
          showFloatingActions ? 'translate-y-0 opacity-100' : 'pointer-events-none translate-y-6 opacity-0',
        )}
      >
        <div className="mx-auto flex max-w-[1540px] items-center justify-between gap-4 rounded-[22px] border border-white/[0.05] bg-[rgba(9,12,20,0.92)] px-4 py-3 shadow-[0_20px_44px_rgba(0,0,0,0.34)] backdrop-blur-xl">
          <div className="flex min-w-0 items-center gap-3">
            {coverMedia ? (
              <div className="h-14 w-20 shrink-0 overflow-hidden rounded-[14px] bg-black/30">
                <img src={mediaUrl(coverMedia)} alt="" className="h-full w-full object-cover" />
              </div>
            ) : null}
            <div className="min-w-0">
              <p className="truncate text-sm font-medium text-white">{data.title}</p>
              <p className="truncate text-xs text-white/52">
                {browserPlayable ? 'Ready to play' : (primaryXcloudOption?.url ?? data.xcloud_url) ? 'Available in xCloud' : `${data.source_games.length} source record${data.source_games.length === 1 ? '' : 's'}`}
              </p>
            </div>
          </div>
          <div className="flex shrink-0 flex-wrap items-center gap-3">
            {browserPlayable ? (
              <HeroActionButton type="button" primary onClick={handleLaunchBrowser} disabled={!browserPlayResolution?.canLaunch}>
                <PlayCircle size={16} />
                Play
              </HeroActionButton>
            ) : null}
            {(primaryXcloudOption?.url ?? data.xcloud_url) ? (
              <a
                href={primaryXcloudOption?.url ?? data.xcloud_url}
                target="_blank"
                rel="noreferrer"
                onClick={() => handleLaunchXcloud(primaryXcloudOption)}
                className="inline-flex h-12 items-center justify-center gap-2 rounded-[16px] border border-white/[0.08] bg-[#101620] px-5 text-sm font-medium text-white transition-colors hover:bg-white/[0.06]"
              >
                <BrandIcon brand="xcloud" className="h-4 w-4 invert" />
                Play xCloud
              </a>
            ) : null}
            <FavoriteActionButton
              favorite={data.favorite}
              busy={favoriteBusy}
              onClick={() => void handleToggleFavorite()}
            />
            <HeroOverflowMenu direction="up" onRefresh={handleRefreshMetadata} onReclassify={handleReclassify} refreshBusy={refreshBusy} />
          </div>
        </div>
      </div>
      {refreshBusy ? (
        <div className="fixed inset-x-0 bottom-24 z-30 px-4 md:px-6 lg:px-8">
          <div className="mx-auto flex max-w-[1540px] items-center gap-3 rounded-[18px] border border-sky-300/20 bg-[rgba(9,18,30,0.94)] px-4 py-3 text-sm text-sky-50 shadow-[0_20px_44px_rgba(0,0,0,0.34)] backdrop-blur-xl">
            <Loader2 size={18} className="shrink-0 animate-spin text-sky-200" />
            <span>Refreshing metadata and achievements...</span>
          </div>
        </div>
      ) : null}
      </div>

      <MediaViewerDialog media={selectedMedia} onClose={() => setSelectedMedia(null)} />
      <Dialog
        open={deleteTarget !== null}
        onClose={() => {
          if (!deleteBusy) setDeleteTarget(null)
        }}
        title="Hard Delete Source Record"
      >
        {deleteTarget && (
          <div className="space-y-4">
            <p className="text-sm text-mga-muted">
              This permanently deletes the backing files and stored source record for
              {' '}
              <span className="font-medium text-mga-text">{sourceRecordLabel(deleteTarget)}</span>.
            </p>
            {deleteTarget.root_path && (
              <p className="rounded-mga border border-mga-border bg-mga-bg px-3 py-2 text-xs text-mga-muted">
                Root path: {deleteTarget.root_path}
              </p>
            )}
            {deleteTarget.hard_delete?.reason && !deleteTarget.hard_delete.eligible && (
              <p className="text-xs text-amber-300">{deleteTarget.hard_delete.reason}</p>
            )}
            <div className="flex justify-end gap-3">
              <Button
                type="button"
                variant="outline"
                onClick={() => setDeleteTarget(null)}
                disabled={deleteBusy}
              >
                Cancel
              </Button>
              <Button
                type="button"
                variant="outline"
                onClick={() => void handleConfirmHardDelete()}
                disabled={deleteBusy}
                className="border-red-500/30 text-red-200 hover:bg-red-500/10"
              >
                <FileText size={16} />
                {deleteBusy ? 'Deleting...' : 'Delete Source Record'}
              </Button>
            </div>
          </div>
        )}
      </Dialog>
    </div>
  )
}
