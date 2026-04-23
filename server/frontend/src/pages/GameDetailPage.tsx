import { useEffect, useLayoutEffect, useMemo, useRef, useState, type ReactNode } from 'react'
import { useQuery, useQueryClient } from '@tanstack/react-query'
import { createPortal } from 'react-dom'
import {
  ArrowLeft,
  ArrowRightLeft,
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
  deleteSourceGame,
  getGame,
  getGameAchievements,
  refreshGameMetadata,
  setGameCoverOverride,
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
  formatHLTB,
  hasBrowserPlaySupport,
  pluginLabel,
  platformLabel,
  selectSourcePlugins,
  sourceMatchCount,
} from '@/lib/gameUtils'
import { GameMediaCollection, mediaUrl, youtubeEmbedUrl } from '@/lib/gameMedia'
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
        'scroll-mt-32 overflow-hidden rounded-[24px] border border-white/10 bg-[rgba(21,17,29,0.92)] shadow-[0_22px_50px_rgba(0,0,0,0.24)] backdrop-blur-sm',
        className,
      )}
    >
      <div className="flex flex-wrap items-start justify-between gap-3 border-b border-white/10 px-4 py-3.5 md:px-5">
        <div className="space-y-1">
          <div className="flex items-center gap-2">
            {icon}
            <h2 className="text-base font-semibold text-mga-text">{title}</h2>
          </div>
          {description ? <div className="text-sm leading-6 text-mga-muted">{description}</div> : null}
        </div>
        {actions ? <div className="flex shrink-0 flex-wrap gap-2">{actions}</div> : null}
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
    <div className="rounded-[18px] border border-white/10 bg-black/20 p-3.5 shadow-[0_8px_20px_rgba(0,0,0,0.12)]">
      <p className="text-[11px] font-medium uppercase tracking-[0.18em] text-mga-muted">{label}</p>
      <div className="mt-1.5 text-sm font-medium text-mga-text">{value}</div>
      <AttributionNote sources={attributionSources} prefix={attributionPrefix} />
    </div>
  )
}

function HeroStatCard({ label, value, detail }: { label: string; value: ReactNode; detail?: ReactNode }) {
  return (
    <div className="rounded-[18px] border border-white/10 bg-black/20 px-3.5 py-3.5 shadow-[0_12px_28px_rgba(0,0,0,0.16)]">
      <p className="text-[11px] font-medium uppercase tracking-[0.18em] text-mga-muted">{label}</p>
      <div className="mt-1.5 text-lg font-semibold text-mga-text">{value}</div>
      {detail ? <div className="mt-1.5 text-xs leading-5 text-mga-muted">{detail}</div> : null}
    </div>
  )
}

function SectionJumpLink({ href, label }: { href: string; label: string }) {
  return (
    <a
      href={href}
      className="inline-flex h-9 items-center justify-center rounded-full border border-white/10 bg-white/5 px-3.5 text-xs font-medium text-white/80 transition-colors hover:bg-white/10"
    >
      {label}
    </a>
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

type MenuPoint = { x: number; y: number }
const VIEWPORT_MARGIN = 8

function MediaContextMenu({
  media,
  point,
  busy,
  current,
  onClose,
  onSetCover,
}: {
  media: GameMediaDetailDTO | null
  point: MenuPoint | null
  busy: boolean
  current: boolean
  onClose: () => void
  onSetCover: (media: GameMediaDetailDTO) => void
}) {
  const menuRef = useRef<HTMLDivElement | null>(null)
  const [menuPosition, setMenuPosition] = useState<MenuPoint | null>(null)

  useEffect(() => {
    if (!point) {
      setMenuPosition(null)
      return
    }
    setMenuPosition(point)
  }, [point])

  useEffect(() => {
    if (!point) return
    const onKeyDown = (event: KeyboardEvent) => {
      if (event.key === 'Escape') onClose()
    }
    const onPointerDown = () => onClose()
    window.addEventListener('keydown', onKeyDown)
    window.addEventListener('pointerdown', onPointerDown)
    return () => {
      window.removeEventListener('keydown', onKeyDown)
      window.removeEventListener('pointerdown', onPointerDown)
    }
  }, [onClose, point])

  useLayoutEffect(() => {
    if (!point || !menuRef.current) return

    const rect = menuRef.current.getBoundingClientRect()
    const nextX = Math.max(
      VIEWPORT_MARGIN,
      Math.min(point.x, window.innerWidth - rect.width - VIEWPORT_MARGIN),
    )
    const nextY = Math.max(
      VIEWPORT_MARGIN,
      Math.min(point.y, window.innerHeight - rect.height - VIEWPORT_MARGIN),
    )

    if (!menuPosition || menuPosition.x !== nextX || menuPosition.y !== nextY) {
      setMenuPosition({ x: nextX, y: nextY })
    }
  }, [menuPosition, point])

  if (!media || !point || typeof document === 'undefined') return null

  return createPortal(
    <div
      ref={menuRef}
      className="fixed z-[200] min-w-52 rounded-mga border border-mga-border bg-mga-surface p-1 shadow-xl shadow-black/30"
      style={{ left: menuPosition?.x ?? point.x, top: menuPosition?.y ?? point.y }}
      onClick={(event) => event.stopPropagation()}
      onPointerDown={(event) => event.stopPropagation()}
      onContextMenu={(event) => event.preventDefault()}
      role="menu"
    >
      <button
        type="button"
        role="menuitem"
        disabled={busy || current}
        onClick={() => onSetCover(media)}
        className="block w-full rounded-mga px-3 py-2 text-left text-sm hover:bg-mga-elevated disabled:text-mga-muted disabled:hover:bg-transparent"
      >
        {current ? 'Current cover image' : (busy ? 'Setting cover...' : 'Set as cover image')}
      </button>
    </div>,
    document.body,
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
  const [selectedMediaMenu, setSelectedMediaMenu] = useState<{
    media: GameMediaDetailDTO
    point: MenuPoint
  } | null>(null)
  const [refreshBusy, setRefreshBusy] = useState(false)
  const [refreshNotice, setRefreshNotice] = useState('')
  const [refreshError, setRefreshError] = useState('')
  const [coverBusy, setCoverBusy] = useState(false)
  const [coverError, setCoverError] = useState('')
  const [deleteTarget, setDeleteTarget] = useState<SourceGameDetailDTO | null>(null)
  const [deleteBusy, setDeleteBusy] = useState(false)
  const [deleteNotice, setDeleteNotice] = useState('')
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
  const imageMedia = useMemo(() => mediaCollection.imageMedia(), [mediaCollection])
  const nonImageMedia = useMemo(() => mediaCollection.nonImageMedia(), [mediaCollection])
  const coverMedia = useMemo(() => gameData?.cover_override ?? mediaCollection.cover(), [gameData?.cover_override, mediaCollection])
  const heroMedia = useMemo(() => mediaCollection.hero(), [mediaCollection])
  const heroUrl = heroMedia ? mediaUrl(heroMedia) : null
  const coverUrl = coverMedia ? mediaUrl(coverMedia) : null
  const [selectedBrowserSourceId, setSelectedBrowserSourceId] = useState('')
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
  const browserPlayable = browserPlaySelections.length > 0
  const browserPlayIssue = browserPlayResolution?.issue ?? null
  const browserPlayRuntime = browserPlayResolution?.runtime ?? null
  const sources = gameData ? selectSourcePlugins(gameData) : []
  const hltb = gameData ? formatHLTB(gameData.completion_time) : null
  const matchCount = gameData ? sourceMatchCount(gameData) : 0
  const resolverCount = gameData
    ? gameData.source_games.reduce(
        (total, sourceGame) => total + sourceGame.resolver_matches.length,
        0,
      )
    : 0
  const externalLinks = useMemo(() => buildExternalLinks(gameData?.external_ids), [gameData?.external_ids])
  const fileGroups = useMemo(() => buildGameFileGroups(gameData?.source_games ?? []), [gameData?.source_games])
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
  const launchableSourceCount = browserPlaySelections.length
  useEffect(() => {
    hasRetried404Ref.current = false
  }, [id])

  useEffect(() => {
    setSelectedBrowserSourceId('')
  }, [id])

  useEffect(() => {
    if (!achievements.isSuccess) return
    void queryClient.invalidateQueries({ queryKey: ['games'] })
  }, [achievements.isSuccess, queryClient])

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

  const handleSetCoverOverride = async (media: GameMediaDetailDTO) => {
    if (!game.data || coverBusy) return
    setCoverBusy(true)
    setCoverError('')
    try {
      const updated = await setGameCoverOverride(game.data.id, media.asset_id)
      queryClient.setQueryData(['game', updated.id], updated)
      await queryClient.invalidateQueries({ queryKey: ['games'] })
      setSelectedMediaMenu(null)
    } catch (error) {
      const message =
        error instanceof ApiError
          ? error.responseText?.trim() || error.message
          : (error instanceof Error ? error.message : 'Set cover override failed.')
      setCoverError(message)
    } finally {
      setCoverBusy(false)
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
  const achievementPercent = achievementSummary.totalCount > 0 ? (achievementSummary.unlockedCount / achievementSummary.totalCount) * 100 : 0
  const gameFilesComplete = fileGroups.all.length > 0 || data.source_games.some((source) => source.files.length === 0)

  return (
    <div className="mx-auto max-w-[1500px] space-y-6 p-4 md:p-6">
      <div className="flex flex-wrap items-center justify-between gap-3">
        <Button variant="outline" size="sm" onClick={handleBack}>
          <ArrowLeft size={14} />
          {originLabel}
        </Button>
        <div className="flex flex-wrap gap-2 text-xs text-mga-muted">
          <span className="inline-flex items-center rounded-full border border-white/10 bg-white/5 px-3 py-1.5">
            {data.source_games.length} source record{data.source_games.length === 1 ? '' : 's'}
          </span>
          <span className="inline-flex items-center rounded-full border border-white/10 bg-white/5 px-3 py-1.5">
            {imageMedia.length} image{imageMedia.length === 1 ? '' : 's'}
          </span>
        </div>
      </div>

      <section
        id="overview"
        className="relative overflow-hidden rounded-[32px] border border-white/10 bg-[#14101b] shadow-[0_30px_80px_rgba(0,0,0,0.34)]"
      >
        {heroUrl && (
          <div className="absolute inset-0">
            <img
              src={heroUrl}
              alt=""
              className="h-full w-full scale-105 object-cover opacity-25 blur-[34px]"
              aria-hidden="true"
            />
            <div className="absolute inset-0 bg-gradient-to-br from-[#0a0810]/96 via-[#14101b]/88 to-[#0b0911]/96" />
          </div>
        )}
        <div className="absolute inset-0 bg-[radial-gradient(circle_at_top_right,rgba(61,184,255,0.16),transparent_32%),radial-gradient(circle_at_bottom_left,rgba(255,255,255,0.06),transparent_28%)]" />

        <div className="relative grid gap-6 p-5 xl:grid-cols-[minmax(260px,320px)_minmax(0,1.22fr)_minmax(280px,340px)] xl:p-8">
          <div className="space-y-4">
            <CoverImage src={coverUrl} alt={data.title} fit="contain" variant="hero" className="w-full" />
            <div className="grid grid-cols-2 gap-3">
              <HeroStatCard label="Source Records" value={data.source_games.length} detail={`${launchableSourceCount} ready to launch`} />
              <HeroStatCard label="Game Files" value={fileGroups.all.length} detail={fileGroups.primary.length > 0 ? `${fileGroups.primary.length} primary/root` : 'No primary file'} />
              <HeroStatCard label="Media" value={data.media?.length ?? 0} detail={imageMedia.length > 0 ? `${imageMedia.length} image gallery items` : 'No gallery yet'} />
              <HeroStatCard
                label="Achievements"
                value={achievementSummary.totalCount > 0 ? `${achievementSummary.unlockedCount}/${achievementSummary.totalCount}` : 'None'}
                detail={achievementSets.length > 0 ? `${achievementSets.length} system${achievementSets.length === 1 ? '' : 's'}` : 'No achievement feeds'}
              />
            </div>
          </div>

          <div className="space-y-5">
            <div className="space-y-4">
              <div className="flex flex-wrap items-center gap-2 text-xs text-white/70">
                <span className="inline-flex items-center rounded-full border border-white/10 bg-white/5 px-3 py-1.5">
                  Game Detail
                </span>
                <span className="inline-flex items-center rounded-full border border-white/10 bg-white/5 px-3 py-1.5">
                  {humanizeValue(data.kind)}
                  {data.group_kind ? ` · ${humanizeValue(data.group_kind)}` : ''}
                </span>
              </div>

              <div className="space-y-4">
                <div className="space-y-3">
                  <h1 className="max-w-4xl text-3xl font-semibold tracking-tight text-white md:text-5xl">{data.title}</h1>
                  <div className="flex flex-wrap gap-2">
                    <Badge variant="platform" className="border-white/10 bg-white/10 text-white">
                      <PlatformIcon platform={data.platform} showLabel />
                    </Badge>
                    {sources.map((source) => (
                      <SourceBadge key={source} source={source} className="border-white/10 bg-white/10 text-white" />
                    ))}
                    {data.xcloud_available && <BrandBadge brand="xcloud" label="xCloud" className="border-white/10 bg-white/10 text-white" />}
                    {data.is_game_pass && <Badge variant="gamepass" className="border-white/10 bg-white/10 text-white">Game Pass</Badge>}
                    {browserPlayable && <Badge variant="playable" className="border-white/10 bg-white/10 text-white">Browser Play</Badge>}
                  </div>
                </div>
                <AttributionNote sources={metadataAttribution.title} prefix="Title aligned to" />
                <p className="max-w-4xl text-sm leading-7 text-white/70 md:text-[15px]">
                  {data.description || 'No description is available for this game yet.'}
                </p>
                {data.description && <AttributionNote sources={metadataAttribution.description} prefix="Description attributed to" />}
              </div>
            </div>

            <div className="grid gap-3 md:grid-cols-3">
              <MetaItem label="Primary Identity" value={<span className="text-white/90">{platformLabel(data.platform)}</span>} />
              <MetaItem label="Availability" value={<span className="text-white/90">{[data.xcloud_available ? 'xCloud' : null, data.is_game_pass ? 'Game Pass' : null, browserPlayable ? 'Browser Play' : null].filter(Boolean).join(' · ') || 'Library only'}</span>} />
              <MetaItem label="Stats" value={<span className="text-white/90">{[hltb ? `${hltb} main` : null, matchCount > 0 ? `${matchCount} sources` : null, achievementSummary.totalCount > 0 ? `${achievementSummary.unlockedCount}/${achievementSummary.totalCount} achievements` : null].filter(Boolean).join(' · ') || 'No extra stats yet'}</span>} />
            </div>

            <div className="space-y-3 rounded-[24px] border border-white/10 bg-black/20 p-4 shadow-[0_16px_36px_rgba(0,0,0,0.2)]">
              <div className="flex flex-wrap items-start justify-between gap-3">
                <div>
                  <p className="text-sm font-semibold text-mga-text">Actions</p>
                  <p className="mt-1 text-xs leading-5 text-white/70">
                    Launch, inspect, refresh, or reclassify this canonical game without losing page context.
                  </p>
                </div>
                <div className="flex flex-wrap gap-2">
                  <SectionJumpLink href="#media-gallery" label="Media" />
                  <SectionJumpLink href="#game-files" label="Files" />
                  <SectionJumpLink href="#source-records" label="Sources" />
                  {achievementSets.length > 0 ? <SectionJumpLink href="#achievements" label="Achievements" /> : null}
                </div>
              </div>

              {browserPlayable && (browserPlaySelections.length > 1 || browserPlayIssue?.code === 'invalid_remembered_source') && (
                <div className="rounded-[18px] border border-white/10 bg-black/20 p-3">
                  <label className="mb-1 block text-xs uppercase tracking-[0.18em] text-mga-muted">Source</label>
                  <select
                    value={selectedBrowserSelection?.sourceGame.id ?? selectedBrowserSourceId}
                    onChange={(event) => handleBrowserSourceChange(event.target.value)}
                    className="w-full rounded-mga border border-white/10 bg-[#120f18] px-3 py-2 text-sm text-mga-text"
                  >
                    {!selectedBrowserSelection && (
                      <option value="" disabled>
                        Choose a source to enable launch
                      </option>
                    )}
                    {browserPlaySelections.map((selection) => (
                      <option key={selection.sourceGame.id} value={selection.sourceGame.id}>
                        {browserPlaySourceOptionLabel(selection, browserPlaySelections)}
                      </option>
                    ))}
                  </select>
                  <p className="mt-2 text-xs text-mga-muted">
                    {selectedBrowserSelection
                      ? browserPlaySourceContext(selectedBrowserSelection)
                      : (browserPlayIssue?.message ?? 'Choose the source or version to launch.')}
                  </p>
                </div>
              )}
              <div className="flex flex-wrap gap-3">
                {data.xcloud_url && (
                  <a
                    href={data.xcloud_url}
                    target="_blank"
                    rel="noreferrer"
                    onClick={handleLaunchXcloud}
                    className="inline-flex h-11 items-center justify-center gap-2 rounded-full bg-mga-accent px-5 py-2 text-sm font-medium text-white transition-opacity hover:opacity-90"
                  >
                    <BrandIcon brand="xcloud" className="h-4 w-4 invert" />
                    Play on xCloud
                  </a>
                )}
                {browserPlayable && (
                  <button
                    type="button"
                    onClick={handleLaunchBrowser}
                    disabled={!browserPlayResolution?.canLaunch}
                    className="inline-flex h-11 items-center justify-center gap-2 rounded-full border border-white/10 bg-white/5 px-5 py-2 text-sm font-medium text-mga-text transition-colors hover:bg-white/10 disabled:cursor-not-allowed disabled:opacity-60"
                  >
                    <PlayCircle size={16} />
                    Play in Browser
                  </button>
                )}
                {externalLinks.length > 0 && (
                  <a
                    href="#external-links"
                    className="inline-flex h-11 items-center justify-center gap-2 rounded-full border border-white/10 bg-white/5 px-5 py-2 text-sm font-medium text-mga-text transition-colors hover:bg-white/10"
                  >
                    <ExternalLink size={16} />
                    External Links
                  </a>
                )}
                {data.source_games.length > 0 && (
                  <a
                    href="#source-records"
                    className="inline-flex h-11 items-center justify-center gap-2 rounded-full border border-white/10 bg-white/5 px-5 py-2 text-sm font-medium text-mga-text transition-colors hover:bg-white/10"
                  >
                    <Database size={16} />
                    Source Records
                  </a>
                )}
                <button
                  type="button"
                  onClick={handleReclassify}
                  className="inline-flex h-11 items-center justify-center gap-2 rounded-full border border-white/10 bg-white/5 px-5 py-2 text-sm font-medium text-mga-text transition-colors hover:bg-white/10"
                >
                  <ArrowRightLeft size={16} />
                  Reclassify
                </button>
                <button
                  type="button"
                  onClick={handleRefreshMetadata}
                  disabled={refreshBusy}
                  className="inline-flex h-11 items-center justify-center gap-2 rounded-full border border-white/10 bg-white/5 px-5 py-2 text-sm font-medium text-mga-text transition-colors hover:bg-white/10 disabled:cursor-not-allowed disabled:opacity-60"
                >
                  <Database size={16} />
                  {refreshBusy ? 'Refreshing...' : 'Refresh Metadata & Media'}
                </button>
              </div>
              {data.xcloud_url && <AttributionNote sources={['xcloud']} prefix="Streaming target" />}
              {browserSupported && !browserPlayable && (
                <p className="text-xs text-mga-muted">
                  {browserPlayIssue?.message ?? 'Browser Play is supported for this platform, but no launchable source file was found yet.'}
                </p>
              )}
              {browserSupported && browserPlayable && !browserPlayResolution?.canLaunch && browserPlayIssue && (
                <p className="text-xs text-amber-300">{browserPlayIssue.message}</p>
              )}
              {refreshNotice && <p className="text-xs text-green-400">{refreshNotice}</p>}
              {deleteNotice && <p className="text-xs text-green-400">{deleteNotice}</p>}
              {refreshError && <p className="text-xs text-red-400">{refreshError}</p>}
              {deleteError && <p className="text-xs text-red-400">{deleteError}</p>}
            </div>
          </div>

          <div className="space-y-4">
            <SectionCard
              title="Quick Facts"
              icon={<FolderOpen size={18} className="text-mga-accent" />}
              description="Fast-reference facts for this canonical entry."
              className="bg-black/20"
            >
              <div className="grid gap-3 sm:grid-cols-2 xl:grid-cols-1">
                <MetaItem label="Release Date" value={formatDateValue(data.release_date)} attributionSources={metadataAttribution.release_date} />
                <MetaItem label="Developer" value={detailValue(data.developer)} attributionSources={metadataAttribution.developer} />
                <MetaItem label="Publisher" value={detailValue(data.publisher)} attributionSources={metadataAttribution.publisher} />
                <MetaItem label="Genres" value={data.genres && data.genres.length > 0 ? data.genres.join(', ') : 'Unknown'} attributionSources={metadataAttribution.genres} />
                <MetaItem label="Players" value={data.max_players ? `${data.max_players}` : 'Unknown'} attributionSources={metadataAttribution.max_players} />
                <MetaItem label="Rating" value={data.rating ? data.rating.toFixed(1) : 'Unknown'} attributionSources={metadataAttribution.rating} />
                <MetaItem label="Main Story" value={formatHours(data.completion_time?.main_story)} attributionSources={data.completion_time?.source ? [data.completion_time.source] : null} attributionPrefix="Estimate from" />
                <MetaItem label="Completionist" value={formatHours(data.completion_time?.completionist)} attributionSources={data.completion_time?.source ? [data.completion_time.source] : null} attributionPrefix="Estimate from" />
              </div>
            </SectionCard>
          </div>
        </div>
      </section>

      <nav className="sticky top-[4.75rem] z-10 overflow-x-auto rounded-full border border-white/10 bg-[rgba(20,16,27,0.88)] p-2 backdrop-blur-md">
        <div className="flex min-w-max gap-2">
          <SectionJumpLink href="#overview" label="Overview" />
          <SectionJumpLink href="#media-gallery" label="Media" />
          <SectionJumpLink href="#game-files" label="Files" />
          <SectionJumpLink href="#source-records" label="Sources" />
          {achievementSets.length > 0 ? <SectionJumpLink href="#achievements" label="Achievements" /> : null}
          {externalLinks.length > 0 ? <SectionJumpLink href="#external-links" label="Links" /> : null}
        </div>
      </nav>

      <div className="grid gap-6 xl:grid-cols-[minmax(0,1.45fr)_minmax(320px,0.92fr)]">
        <div className="space-y-6">
          <SectionCard
            id="media-gallery"
            title="Media Gallery"
            icon={<ImageIcon size={18} className="text-mga-accent" />}
            description="Cover art, screenshots, and linked image assets for this canonical entry."
          >
            {imageMedia.length === 0 ? (
              <p className="text-sm text-mga-muted">No image media is available for this game yet.</p>
            ) : (
              <div className="grid grid-cols-2 gap-3 md:grid-cols-3">
                {imageMedia.map((media) => (
                  <button
                    key={`${media.asset_id}:${media.type}`}
                    type="button"
                    onClick={() => setSelectedMedia(media)}
                    onContextMenu={(event) => {
                      event.preventDefault()
                      setCoverError('')
                      setSelectedMediaMenu({
                        media,
                        point: { x: event.clientX, y: event.clientY },
                      })
                    }}
                    className="group overflow-hidden rounded-[20px] border border-white/10 bg-black/20 text-left transition-transform duration-200 hover:-translate-y-0.5 hover:border-mga-accent"
                  >
                    <img
                      src={mediaUrl(media)}
                      alt={mediaTypeLabel(media.type)}
                      loading="lazy"
                      decoding="async"
                      className="aspect-video w-full object-cover transition-transform duration-200 group-hover:scale-[1.02]"
                    />
                    <div className="space-y-2 border-t border-white/10 px-3 py-2.5">
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
            {coverError ? <p className="mt-3 text-sm text-red-400">{coverError}</p> : null}
          </SectionCard>

          {nonImageMedia.length > 0 && (
            <SectionCard title="Other Media" icon={<Video size={18} className="text-mga-accent" />}>
              <div className="space-y-4">
                {nonImageMedia.map((media) => <OtherMediaCard key={`${media.asset_id}:${media.type}`} media={media} />)}
              </div>
            </SectionCard>
          )}

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
              <p className="text-sm text-mga-muted">No source files are available for this game yet.</p>
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
                <p className="text-sm text-mga-muted">No source records are stored for this game.</p>
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
        </div>

        <div className="space-y-6 xl:sticky xl:top-[8.5rem] xl:self-start">
          <SectionCard
            id="achievements"
            title="Achievements"
            icon={<Trophy size={18} className="text-mga-accent" />}
            description="Cached achievement progress grouped by connected achievement system."
          >
            {achievements.isPending ? (
              <p className="text-sm text-mga-muted">Loading achievements...</p>
            ) : achievements.isError ? (
              <div className="rounded-mga border border-red-500/30 bg-red-500/10 p-4">
                <p className="text-sm text-red-400">{achievements.error.message}</p>
              </div>
            ) : achievementSets.length > 0 ? (
              <div className="space-y-4">
                <div className="grid gap-3 md:grid-cols-3 xl:grid-cols-1">
                  <MetaItem label="Sources" value={achievementSets.length} />
                  <MetaItem label="Unlocked" value={`${achievementSummary.unlockedCount}/${achievementSummary.totalCount}`} />
                  <MetaItem label="Points" value={achievementSummary.totalPoints > 0 ? `${achievementSummary.earnedPoints}/${achievementSummary.totalPoints}` : 'Unknown'} />
                </div>
                <ProgressBar value={achievementPercent} label={`${achievementSummary.unlockedCount}/${achievementSummary.totalCount}`} />
                {achievementSets.map((set) => (
                  <div key={`${set.source}:${set.external_game_id}`} className="space-y-4 rounded-[20px] border border-white/10 bg-black/20 p-4">
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

          <SectionCard
            id="quick-facts"
            title="Overview Summary"
            icon={<FolderOpen size={18} className="text-mga-accent" />}
            description="A compact read of the canonical record and current metadata confidence."
          >
            <div className="grid gap-3 sm:grid-cols-2 xl:grid-cols-1">
              <MetaItem label="Canonical ID" value={data.id} />
              <MetaItem label="Source Records" value={data.source_games.length} />
              <MetaItem label="Media Items" value={data.media?.length ?? 0} />
              <MetaItem label="Files" value={fileGroups.all.length} />
              <MetaItem label="Resolver Matches" value={resolverCount} />
              <MetaItem label="Root Path" value={data.root_path ?? 'Unknown'} />
              <MetaItem label="HLTB Main + Extra" value={formatHours(data.completion_time?.main_extra)} attributionSources={data.completion_time?.source ? [data.completion_time.source] : null} attributionPrefix="Estimate from" />
              <MetaItem label="HLTB Source" value={data.completion_time?.source ? <SourceBadge source={data.completion_time.source} /> : 'Unknown'} />
            </div>
          </SectionCard>

          {externalLinks.length > 0 && (
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
          )}

          {gameFilesComplete && (
            <SectionCard
              title="Coverage Notes"
              icon={<HardDrive size={18} className="text-mga-accent" />}
              description="The page exposes both canonical grouping and per-source file inventory."
            >
              <div className="space-y-3 text-sm text-mga-muted">
                <p>Canonical grouping answers what launches, what packages the game, and what supports it.</p>
                <p>Per-source inventories remain available inside each source record for deeper inspection.</p>
              </div>
            </SectionCard>
          )}
        </div>
      </div>

      <MediaViewerDialog media={selectedMedia} onClose={() => setSelectedMedia(null)} />
      <MediaContextMenu
        media={selectedMediaMenu?.media ?? null}
        point={selectedMediaMenu?.point ?? null}
        busy={coverBusy}
        current={selectedMediaMenu?.media?.asset_id === data.cover_override?.asset_id}
        onClose={() => setSelectedMediaMenu(null)}
        onSetCover={(media) => void handleSetCoverOverride(media)}
      />
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
