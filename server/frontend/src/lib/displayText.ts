import { brandLabel } from '@/lib/brands'

type PlatformMeta = { label: string; emoji: string }

export const PLATFORM_META: Record<string, PlatformMeta> = {
  windows_pc: { label: 'Windows PC', emoji: '\u{1F4BB}' },
  ms_dos: { label: 'MS-DOS', emoji: '\u{1F4BE}' },
  arcade: { label: 'Arcade', emoji: '\u{1F579}\uFE0F' },
  nes: { label: 'NES', emoji: '\u{1F3AE}' },
  snes: { label: 'SNES', emoji: '\u{1F3AE}' },
  gb: { label: 'Game Boy', emoji: '\u{1F3AE}' },
  gbc: { label: 'Game Boy Color', emoji: '\u{1F3AE}' },
  gba: { label: 'GBA', emoji: '\u{1F3AE}' },
  n64: { label: 'Nintendo 64', emoji: '\u{1F3AE}' },
  genesis: { label: 'Genesis', emoji: '\u{1F3AE}' },
  sega_master_system: { label: 'Master System', emoji: '\u{1F3AE}' },
  game_gear: { label: 'Game Gear', emoji: '\u{1F3AE}' },
  sega_cd: { label: 'Sega CD', emoji: '\u{1F3AE}' },
  sega_32x: { label: 'Sega 32X', emoji: '\u{1F3AE}' },
  ps1: { label: 'PS1', emoji: '\u{1F3AE}' },
  ps2: { label: 'PS2', emoji: '\u{1F3AE}' },
  ps3: { label: 'PS3', emoji: '\u{1F3AE}' },
  psp: { label: 'PSP', emoji: '\u{1F3AE}' },
  xbox_360: { label: 'Xbox 360', emoji: '\u{1F3AE}' },
  xbox_one: { label: 'Xbox One', emoji: '\u{1F3AE}' },
  xbox_series: { label: 'Xbox Series', emoji: '\u{1F3AE}' },
  scummvm: { label: 'ScummVM', emoji: '\u{1F5B1}\uFE0F' },
  unknown: { label: 'Unknown', emoji: '\u{2753}' },
}

export const PLUGIN_LABELS: Record<string, string> = {
  'game-source-smb': 'SMB File Share',
  'game-source-steam': 'Steam',
  'game-source-xbox': 'Xbox',
  'game-source-epic': 'Epic Games',
  'game-source-google-drive': 'Google Drive',
  'game-source-gdrive': 'Google Drive',
  'metadata-steam': 'Steam Metadata',
  'metadata-rawg': 'RAWG',
  'metadata-igdb': 'IGDB',
  'metadata-gog': 'GOG Metadata',
  'metadata-hltb': 'HowLongToBeat',
  'metadata-launchbox': 'LaunchBox',
  'metadata-mame-dat': 'MAME DAT',
  'retroachievements': 'RetroAchievements',
  'sync-settings-google-drive': 'Google Drive Sync',
  'save-sync-google-drive': 'Google Drive Save Sync',
  'save-sync-local-disk': 'Local Disk Save Sync',
}

export function humanizeIdentifier(value: string): string {
  return value
    .trim()
    .split(/[_-]+/g)
    .filter(Boolean)
    .map((part) => {
      if (part.length <= 3) return part.toUpperCase()
      return part.charAt(0).toUpperCase() + part.slice(1)
    })
    .join(' ')
}

export function platformLabel(platform: string): string {
  return PLATFORM_META[platform]?.label ?? humanizeIdentifier(platform)
}

export function platformEmoji(platform: string): string {
  return PLATFORM_META[platform]?.emoji ?? '\u{1F3AE}'
}

export function pluginLabel(pluginId: string): string {
  return PLUGIN_LABELS[pluginId] ?? humanizeIdentifier(pluginId)
}

export function sourceLabel(pluginId: string): string {
  return brandLabel(pluginId, pluginLabel(pluginId))
}
