import type { CompletionTime, GameDetailResponse, GameMediaDetailDTO } from '@/api/client'
import { brandLabel } from '@/lib/brands'

// ---------------------------------------------------------------------------
// Cover art selection
// ---------------------------------------------------------------------------

/** Pick the best cover URL from a game's media array. */
export function selectCoverUrl(media: GameMediaDetailDTO[] | undefined): string | null {
  if (!media || media.length === 0) return null

  const cover = media.find((m) => m.type === 'cover')
  if (!cover) return null

  // Prefer locally-cached file served by the Go media endpoint
  if (cover.local_path) return `/api/media/${cover.asset_id}`

  return cover.url || null
}

// ---------------------------------------------------------------------------
// Browser play
// ---------------------------------------------------------------------------

export function hasBrowserPlaySupport(game: Pick<GameDetailResponse, 'play'>): boolean {
  return game.play?.platform_supported === true
}

export function isPlayable(game: Pick<GameDetailResponse, 'play'>): boolean {
  return game.play?.available === true
}

export function isActionable(
  game: Pick<GameDetailResponse, 'play' | 'xcloud_available'>,
): boolean {
  return isPlayable(game) || game.xcloud_available === true
}

// ---------------------------------------------------------------------------
// Platform display labels + emoji
// ---------------------------------------------------------------------------

type PlatformMeta = { label: string; emoji: string }

export const PLATFORM_META: Record<string, PlatformMeta> = {
  windows_pc: { label: 'Windows PC', emoji: '\u{1F4BB}' }, // 💻
  ms_dos:     { label: 'MS-DOS',     emoji: '\u{1F4BE}' }, // 💾
  arcade:     { label: 'Arcade',     emoji: '\u{1F579}\uFE0F' }, // 🕹️
  nes:        { label: 'NES',        emoji: '\u{1F3AE}' }, // 🎮
  snes:       { label: 'SNES',       emoji: '\u{1F3AE}' }, // 🎮
  gb:         { label: 'Game Boy',   emoji: '\u{1F3AE}' }, // 🎮
  gbc:        { label: 'Game Boy Color', emoji: '\u{1F3AE}' }, // 🎮
  gba:        { label: 'GBA',        emoji: '\u{1F3AE}' }, // 🎮
  genesis:    { label: 'Genesis',    emoji: '\u{1F3AE}' }, // 🎮
  sega_master_system: { label: 'Master System', emoji: '\u{1F3AE}' }, // 🎮
  game_gear:  { label: 'Game Gear',  emoji: '\u{1F3AE}' }, // 🎮
  sega_cd:    { label: 'Sega CD',    emoji: '\u{1F3AE}' }, // 🎮
  sega_32x:   { label: 'Sega 32X',   emoji: '\u{1F3AE}' }, // 🎮
  ps1:        { label: 'PS1',        emoji: '\u{1F3AE}' }, // 🎮
  ps2:        { label: 'PS2',        emoji: '\u{1F3AE}' }, // 🎮
  ps3:        { label: 'PS3',        emoji: '\u{1F3AE}' }, // 🎮
  psp:        { label: 'PSP',        emoji: '\u{1F3AE}' }, // 🎮
  xbox_360:   { label: 'Xbox 360',   emoji: '\u{1F3AE}' }, // 🎮
  scummvm:    { label: 'ScummVM',    emoji: '\u{1F5B1}\uFE0F' }, // 🖱️
  unknown:    { label: 'Unknown',    emoji: '\u{2753}' }, // ❓
}

export function platformLabel(platform: string): string {
  return PLATFORM_META[platform]?.label ?? platform
}

export function platformEmoji(platform: string): string {
  return PLATFORM_META[platform]?.emoji ?? '\u{1F3AE}' // 🎮
}

// ---------------------------------------------------------------------------
// Source plugin labels
// ---------------------------------------------------------------------------

export const PLUGIN_LABELS: Record<string, string> = {
  'game-source-smb':            'SMB File Share',
  'game-source-steam':          'Steam',
  'game-source-xbox':           'Xbox',
  'game-source-epic':           'Epic Games',
  'game-source-google-drive':   'Google Drive',
  'game-source-gdrive':         'Google Drive',
  'metadata-steam':             'Steam Metadata',
  'metadata-rawg':              'RAWG',
  'metadata-igdb':              'IGDB',
  'metadata-gog':               'GOG Metadata',
  'metadata-hltb':              'HowLongToBeat',
  'metadata-launchbox':         'LaunchBox',
  'metadata-mame-dat':          'MAME DAT',
  'metadata-tgdb':              'TGDB',
  'retroachievements':          'RetroAchievements',
  'sync-settings-google-drive': 'Google Drive Sync',
  'save-sync-google-drive':     'Google Drive Save Sync',
  'save-sync-local-disk':       'Local Disk Save Sync',
}

export function pluginLabel(pluginId: string): string {
  return PLUGIN_LABELS[pluginId] ?? pluginId
}

export function sourceLabel(pluginId: string): string {
  return brandLabel(pluginId, pluginLabel(pluginId))
}

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
  'metadata-tgdb':              'Database',
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
  'x-secret'?: boolean
  'x-help-url'?: string
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

// ---------------------------------------------------------------------------
// Config summary builder (for integration cards)
// ---------------------------------------------------------------------------

/** Extracts a human-readable summary from an integration's config_json. */
export class ConfigSummaryBuilder {
  private static strategies: Record<string, (config: Record<string, unknown>) => string> = {
    'game-source-smb': (c) => {
      const host = c.host ?? ''
      const share = c.share ?? ''
      const path = c.path ?? ''
      return `\\\\${host}\\${share}${path ? '\\' + path : ''}`
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
    'game-source-google-drive': (c) => (c.root_path ? `Path: ${c.root_path}` : 'Root'),
    'game-source-gdrive': (c) => (c.root_path ? `Path: ${c.root_path}` : 'Root'),
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
// Resolver match count (metadata confidence)
// ---------------------------------------------------------------------------

/** Total resolver matches across all source games. */
export function resolverMatchCount(game: GameDetailResponse): number {
  if (!game.source_games) return 0

  let count = 0
  for (const sg of game.source_games) {
    if (sg.resolver_matches) {
      count += sg.resolver_matches.length
    }
  }
  return count
}
