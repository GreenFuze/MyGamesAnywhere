import type { CompletionTime, GameDetailResponse, GameMediaDetailDTO } from '@/api/client'
import {
  PLATFORM_META,
  platformEmoji,
  platformLabel,
  pluginLabel,
  sourceLabel,
} from '@/lib/displayText'
import { getBrowserPlayPreferenceRuntime, listBrowserPlaySelections } from '@/lib/browserPlay'
import { GameMediaCollection, mediaUrl } from '@/lib/gameMedia'

// ---------------------------------------------------------------------------
// Cover art selection
// ---------------------------------------------------------------------------

/** Pick the best cover URL from a game's media array, honoring a pinned override. */
export function selectCoverUrl(
  media: GameMediaDetailDTO[] | undefined,
  coverOverride?: GameMediaDetailDTO,
): string | null {
  if (coverOverride) return mediaUrl(coverOverride)
  return new GameMediaCollection(media).coverUrl()
}

export function selectGameCoverUrl(game: Pick<GameDetailResponse, 'media' | 'cover_override'>): string | null {
  return selectCoverUrl(game.media, game.cover_override)
}

const PREVIEW_MEDIA_PRIORITY = [
  'screenshot',
  'background',
  'backdrop',
  'banner',
  'hero',
  'artwork',
  'fanart',
] as const

export function selectPreviewImageUrl(
  media: GameMediaDetailDTO[] | undefined,
  coverOverride?: GameMediaDetailDTO,
  hoverOverride?: GameMediaDetailDTO,
): string | null {
  if (hoverOverride) return mediaUrl(hoverOverride)
  const collection = new GameMediaCollection(media)
  const imageMedia = collection.imageMedia()

  for (const type of PREVIEW_MEDIA_PRIORITY) {
    const match = imageMedia.find((item) => item.type === type)
    if (match) return mediaUrl(match)
  }

  return selectCoverUrl(media, coverOverride)
}

export function selectGamePreviewImageUrl(
  game: Pick<GameDetailResponse, 'media' | 'cover_override' | 'hover_override'>,
): string | null {
  return selectPreviewImageUrl(game.media, game.cover_override, game.hover_override)
}

// ---------------------------------------------------------------------------
// Browser play
// ---------------------------------------------------------------------------

export function hasBrowserPlaySupport(game: Pick<GameDetailResponse, 'play' | 'platform' | 'source_games'>): boolean {
  return getBrowserPlayPreferenceRuntime(game as GameDetailResponse) !== null
}

export function isPlayable(game: Pick<GameDetailResponse, 'play' | 'platform' | 'source_games'>): boolean {
  return listBrowserPlaySelections(game as GameDetailResponse).length > 0
}

export function isActionable(
  game: Pick<GameDetailResponse, 'play' | 'platform' | 'source_games' | 'xcloud_available'>,
): boolean {
  return isPlayable(game) || game.xcloud_available === true
}

export { PLATFORM_META, platformEmoji, platformLabel, pluginLabel, sourceLabel }

// ---------------------------------------------------------------------------
// Plugin lucide icon mapping (swappable — designed for future custom icons)
// ---------------------------------------------------------------------------

/** Maps plugin_id → lucide-react icon name. Used by PluginIcon component. */
export const PLUGIN_LUCIDE_ICONS: Record<string, string> = {
  'game-source-smb':            'Server',
  'game-source-steam':          'Gamepad2',
  'game-source-xbox':           'Gamepad',
  'game-source-epic':           'Zap',
  'game-source-google-drive':   'Cloud',
  'game-source-gdrive':         'Cloud',
  'metadata-steam':             'Search',
  'metadata-rawg':              'Dice5',
  'metadata-igdb':              'BookOpen',
  'metadata-gog':               'Store',
  'metadata-hltb':              'Timer',
  'metadata-launchbox':         'Package',
  'metadata-mame-dat':          'Joystick',
  'retroachievements':          'Trophy',
  'sync-settings-google-drive': 'RefreshCw',
  'save-sync-google-drive':     'HardDriveUpload',
  'save-sync-local-disk':       'HardDrive',
}

// ---------------------------------------------------------------------------
// Capability metadata (for grouping integrations)
// ---------------------------------------------------------------------------

type CapabilityMeta = { label: string; icon: string; order: number }

export const CAPABILITY_META: Record<string, CapabilityMeta> = {
  source:       { label: 'Game Sources',        icon: 'Gamepad2',  order: 0 },
  metadata:     { label: 'Metadata Providers',  icon: 'BookOpen',  order: 1 },
  achievements: { label: 'Achievements',        icon: 'Trophy',    order: 2 },
  sync:         { label: 'Sync',                icon: 'RefreshCw', order: 3 },
  save_sync:    { label: 'Save Sync',           icon: 'HardDrive', order: 4 },
}

export const CAPABILITY_ORDER: string[] = ['source', 'metadata', 'achievements', 'sync', 'save_sync']

// ---------------------------------------------------------------------------
// Plugin config schema helpers
// ---------------------------------------------------------------------------

/** A single field definition from a plugin's flat config schema. */
export type PluginConfigField = {
  type?: string
  required?: boolean
  default?: unknown
  description?: string
  items?: PluginConfigField
  properties?: Record<string, PluginConfigField>
  'x-secret'?: boolean
  'x-help-url'?: string
}

export type FilesystemIncludePath = {
  path: string
  recursive: boolean
}

/**
 * Parse a plugin's flat config schema map into an iterable array.
 * The plugin schema is NOT JSON Schema — it's a flat map of field_name → field definition.
 */
export function parsePluginConfigSchema(
  config: Record<string, unknown> | undefined,
): Array<{ key: string; field: PluginConfigField }> {
  if (!config) return []
  return Object.entries(config).map(([key, def]) => ({
    key,
    field: (def ?? {}) as PluginConfigField,
  }))
}

export function isFilesystemSourcePlugin(pluginId: string): boolean {
  return pluginId === 'game-source-smb' || pluginId === 'game-source-google-drive'
}

export function normalizeFilesystemIncludePaths(
  pluginId: string,
  config: Record<string, unknown> | undefined,
): FilesystemIncludePath[] {
  if (!isFilesystemSourcePlugin(pluginId)) return []

  const includePaths = config?.include_paths
  if (Array.isArray(includePaths)) {
    const normalized = includePaths
      .map((entry) => {
        if (!entry || typeof entry !== 'object') return null
        const value = entry as Record<string, unknown>
        return {
          path: typeof value.path === 'string' ? normalizeLogicalPath(value.path) : '',
          recursive: typeof value.recursive === 'boolean' ? value.recursive : true,
        }
      })
      .filter((entry): entry is FilesystemIncludePath => entry !== null)
    if (normalized.length > 0) {
      return normalized
    }
  }

  const legacyKey = pluginId === 'game-source-smb' ? 'path' : 'root_path'
  const legacyValue = config?.[legacyKey]
  if (typeof legacyValue === 'string') {
    return [{ path: normalizeLogicalPath(legacyValue), recursive: true }]
  }

  return [{ path: '', recursive: true }]
}

function normalizeLogicalPath(value: string): string {
  return value
    .trim()
    .replaceAll('\\', '/')
    .replace(/^\/+|\/+$/g, '')
}

// ---------------------------------------------------------------------------
// Config summary builder (for integration cards)
// ---------------------------------------------------------------------------

/** Extracts a human-readable summary from an integration's config_json. */
export class ConfigSummaryBuilder {
  private static strategies: Record<string, (config: Record<string, unknown>) => string> = {
    'game-source-smb': (c) => {
      const host = c.host ?? ''
      const share = c.share ?? ''
      const paths = normalizeFilesystemIncludePaths('game-source-smb', c)
      const summary = summarizeIncludePaths(paths, '\\')
      return `\\\\${host}\\${share}${summary}`
    },
    'game-source-steam': (c) => {
      if (c.vanity_url) return `Vanity: ${c.vanity_url}`
      if (c.steam_id) return `Steam ID: ${c.steam_id}`
      return ConfigSummaryBuilder.hintSecret(c, 'api_key')
    },
    'game-source-xbox': (c) => ConfigSummaryBuilder.hintSecret(c, 'client_id'),
    'metadata-igdb': (c) => ConfigSummaryBuilder.hintSecret(c, 'client_id'),
    'metadata-rawg': (c) => ConfigSummaryBuilder.hintSecret(c, 'api_key'),
    'retroachievements': (c) => (c.username ? `User: ${c.username}` : ''),
    'game-source-google-drive': (c) => summarizeDriveIncludePaths(c),
    'game-source-gdrive': (c) => summarizeDriveIncludePaths(c),
    'sync-settings-google-drive': (c) => (c.sync_path ? `Path: ${c.sync_path}` : ''),
    'save-sync-google-drive': (c) => (c.root_path ? `Path: ${c.root_path}` : 'Root'),
    'save-sync-local-disk': () => 'Server-managed root',
  }

  private static hintSecret(config: Record<string, unknown>, key: string): string {
    const val = config[key]
    if (typeof val !== 'string' || val.length === 0) return ''
    return `${key}: ${val.substring(0, 4)}...`
  }

  /** Generate a human-readable config summary for display on cards. */
  static summarize(pluginId: string, configJson: string): string {
    let config: Record<string, unknown>
    try {
      config = JSON.parse(configJson)
    } catch {
      return ''
    }

    const strategy = this.strategies[pluginId]
    if (strategy) {
      const result = strategy(config)
      if (result) return result
    }

    return this.fallbackSummary(config)
  }

  private static fallbackSummary(config: Record<string, unknown>): string {
    for (const [key, value] of Object.entries(config)) {
      if (typeof value === 'string' && value.length > 0 && !this.looksSecret(key)) {
        return `${key}: ${value}`
      }
    }
    return ''
  }

  private static looksSecret(key: string): boolean {
    return /password|secret|token|key/i.test(key)
  }
}

function summarizeIncludePaths(paths: FilesystemIncludePath[], separator = '/'): string {
  if (paths.length === 0) return ''
  if (paths.length === 1) {
    return paths[0].path ? `${separator}${paths[0].path.replaceAll('/', separator)}` : ''
  }
  const first = paths[0].path ? paths[0].path.replaceAll('/', separator) : '(root)'
  return ` [${first} +${paths.length - 1} more]`
}

function summarizeDriveIncludePaths(config: Record<string, unknown>): string {
  const paths = normalizeFilesystemIncludePaths('game-source-google-drive', config)
  if (paths.length === 1) {
    return paths[0].path ? `Path: ${paths[0].path}` : 'Root'
  }
  const first = paths[0]?.path || '(root)'
  return `Paths: ${first} +${paths.length - 1} more`
}

/** Unique source plugin IDs for a game. */
export function selectSourcePlugins(game: GameDetailResponse): string[] {
  if (!game.source_games || game.source_games.length === 0) return []

  const seen = new Set<string>()
  for (const sg of game.source_games) {
    seen.add(sg.plugin_id)
  }
  return Array.from(seen)
}

export function primarySourcePlugin(game: GameDetailResponse): string | null {
  return selectSourcePlugins(game)[0] ?? null
}

function hasTextValue(value: string | undefined): value is string {
  return typeof value === 'string' && value.trim().length > 0
}

export function preferredSecondaryText(game: GameDetailResponse): string | null {
  if (hasTextValue(game.developer)) return game.developer.trim()
  if (hasTextValue(game.publisher)) return game.publisher.trim()

  const source = primarySourcePlugin(game)
  return source ? sourceLabel(source) : null
}

// ---------------------------------------------------------------------------
// HLTB formatting
// ---------------------------------------------------------------------------

export function formatHLTB(ct: CompletionTime | undefined): string | null {
  if (!ct) return null

  const hours = ct.main_story ?? ct.main_extra ?? ct.completionist
  if (!hours || hours <= 0) return null

  return `${Math.round(hours)}h`
}

// ---------------------------------------------------------------------------
// Source match count
// ---------------------------------------------------------------------------

/** Total linked source game records for a canonical game. */
export function sourceMatchCount(game: GameDetailResponse): number {
  return game.source_games?.length ?? 0
}
