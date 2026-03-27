export type BrandDefinition = {
  id: string
  label: string
  iconPath?: string
  websiteUrl?: string
  description: string
  creditNote?: string
  tempResourceName?: string
}

function normalizeKey(value: string): string {
  return value.trim().toLowerCase().replace(/[^a-z0-9]+/g, '-')
}

const BRAND_DEFINITIONS: BrandDefinition[] = [
  {
    id: 'steam',
    label: 'Steam',
    iconPath: '/brands/steam.svg',
    websiteUrl: 'https://store.steampowered.com/',
    description: 'Storefront, source integration, metadata, and achievement provider.',
    creditNote: 'Icon sourced from Wikipedia.',
    tempResourceName: 'steam-logo-from-wikipedia.svg',
  },
  {
    id: 'xbox',
    label: 'Xbox',
    iconPath: '/brands/xbox.png',
    websiteUrl: 'https://www.xbox.com/',
    description: 'Source integration and platform ecosystem for Xbox library records.',
    creditNote: 'Icon sourced from CityPNG.',
    tempResourceName: 'xbox-logo-from-citypng-com.png',
  },
  {
    id: 'xcloud',
    label: 'Xbox Cloud Gaming',
    iconPath: '/brands/xcloud.png',
    websiteUrl: 'https://www.xbox.com/play',
    description: 'Cloud streaming launch target for supported Xbox titles.',
    creditNote: 'Icon sourced from the Greenlight GitHub issue reference noted in the temp resources.',
    tempResourceName: 'xcloud-logo-from-https-github.com-unknownskl-greenlight-issues-351.png',
  },
  {
    id: 'igdb',
    label: 'IGDB',
    iconPath: '/brands/igdb.svg',
    websiteUrl: 'https://www.igdb.com/',
    description: 'Metadata provider for descriptions, release facts, and related game info.',
    creditNote: 'Icon sourced from Wikimedia.',
    tempResourceName: 'IGDB_logo-from-wikimedia.svg',
  },
  {
    id: 'rawg',
    label: 'RAWG',
    websiteUrl: 'https://rawg.io/',
    description: 'Metadata provider used for game facts when available.',
  },
  {
    id: 'gog',
    label: 'GOG',
    iconPath: '/brands/gog.png',
    websiteUrl: 'https://www.gog.com/',
    description: 'Storefront and metadata source for GOG-linked games.',
    creditNote: 'Icon sourced from Wikimedia.',
    tempResourceName: 'gog-logo-from-wikimedia.png',
  },
  {
    id: 'launchbox',
    label: 'LaunchBox',
    iconPath: '/brands/launchbox.png',
    websiteUrl: 'https://www.launchbox-app.com/',
    description: 'Metadata provider for cover art, descriptions, and game facts.',
    creditNote: 'Icon sourced from the provided LaunchBox temp resource.',
    tempResourceName: 'launchbox-logo.png',
  },
  {
    id: 'retroachievements',
    label: 'RetroAchievements',
    iconPath: '/brands/retroachievements.png',
    websiteUrl: 'https://retroachievements.org/',
    description: 'Achievement provider for supported retro platforms.',
    creditNote: 'Icon sourced from Wikimedia.',
    tempResourceName: 'retroachievements-logo-from-wikimedia.png',
  },
  {
    id: 'hltb',
    label: 'HowLongToBeat',
    websiteUrl: 'https://howlongtobeat.com/',
    description: 'Completion-time provider for story and completionist estimates.',
  },
  {
    id: 'mame',
    label: 'MAME',
    iconPath: '/brands/mame.png',
    websiteUrl: 'https://www.mamedev.org/',
    description: 'Arcade metadata/catalog source through MAME DAT integration.',
    creditNote: 'Icon sourced from Wikimedia.',
    tempResourceName: 'mame-logo-from-wikimedia.png',
  },
  {
    id: 'google-drive',
    label: 'Google Drive',
    iconPath: '/brands/google-drive.png',
    websiteUrl: 'https://drive.google.com/',
    description: 'Source and sync integration for cloud-backed libraries.',
    creditNote: 'Icon sourced from Wikimedia.',
    tempResourceName: 'google-drive-logo-from-wikimedia.png',
  },
  {
    id: 'smb',
    label: 'SMB File Share',
    iconPath: '/brands/smb.png',
    description: 'Network file-share integration for local and NAS libraries.',
    creditNote: 'Icon sourced from Flaticon.',
    tempResourceName: 'smb-from-flaticon-com.png',
  },
  {
    id: 'epic-games',
    label: 'Epic Games',
    websiteUrl: 'https://store.epicgames.com/',
    description: 'Storefront/source integration for Epic-linked games.',
  },
  {
    id: 'tgdb',
    label: 'TheGamesDB',
    websiteUrl: 'https://thegamesdb.net/',
    description: 'Metadata provider that remains disabled due to API quota constraints.',
  },
  {
    id: 'windows',
    label: 'Windows',
    iconPath: '/brands/windows.png',
    websiteUrl: 'https://www.microsoft.com/windows',
    description: 'Platform mark used for Windows PC titles.',
    creditNote: 'Icon sourced from FreeIconsPNG.',
    tempResourceName: 'windows-logo-from-freeiconspng-com.png',
  },
  {
    id: 'ms-dos',
    label: 'MS-DOS',
    iconPath: '/brands/ms-dos.svg',
    description: 'Platform mark used for DOS titles.',
    creditNote: 'Icon sourced from Wikimedia.',
    tempResourceName: 'MS-DOS-logo-wikimedia.svg',
  },
  {
    id: 'gba',
    label: 'Game Boy Advance',
    iconPath: '/brands/gba.svg',
    description: 'Platform mark used for GBA titles.',
    creditNote: 'Icon sourced from Wikimedia.',
    tempResourceName: 'Game_Boy_Advance_logo-from-wikimedia-org.svg',
  },
  {
    id: 'arcade',
    label: 'Arcade',
    iconPath: '/brands/arcade.png',
    description: 'Platform mark used for arcade titles.',
    creditNote: 'Icon sourced from Clipmax.',
    tempResourceName: 'arcade-icon-from-clipmax-com.png',
  },
  {
    id: 'scummvm',
    label: 'ScummVM',
    iconPath: '/brands/scummvm.png',
    websiteUrl: 'https://www.scummvm.org/',
    description: 'Platform mark used for ScummVM-compatible adventure titles.',
    creditNote: 'Icon sourced from Wikimedia.',
    tempResourceName: 'ScummVM-logo-wikimedia.png',
  },
]

const BRAND_BY_ID = new Map(BRAND_DEFINITIONS.map((brand) => [brand.id, brand] as const))

const BRAND_ALIASES = new Map<string, string>([
  ['steam', 'steam'],
  ['game-source-steam', 'steam'],
  ['metadata-steam', 'steam'],
  ['xbox', 'xbox'],
  ['game-source-xbox', 'xbox'],
  ['xcloud', 'xcloud'],
  ['igdb', 'igdb'],
  ['metadata-igdb', 'igdb'],
  ['rawg', 'rawg'],
  ['metadata-rawg', 'rawg'],
  ['gog', 'gog'],
  ['metadata-gog', 'gog'],
  ['launchbox', 'launchbox'],
  ['metadata-launchbox', 'launchbox'],
  ['retroachievements', 'retroachievements'],
  ['hltb', 'hltb'],
  ['howlongtobeat', 'hltb'],
  ['metadata-hltb', 'hltb'],
  ['mame', 'mame'],
  ['mame-dat', 'mame'],
  ['metadata-mame-dat', 'mame'],
  ['google-drive', 'google-drive'],
  ['google-drive-sync', 'google-drive'],
  ['game-source-google-drive', 'google-drive'],
  ['game-source-gdrive', 'google-drive'],
  ['sync-settings-google-drive', 'google-drive'],
  ['smb', 'smb'],
  ['game-source-smb', 'smb'],
  ['epic', 'epic-games'],
  ['epic-games', 'epic-games'],
  ['game-source-epic', 'epic-games'],
  ['tgdb', 'tgdb'],
  ['metadata-tgdb', 'tgdb'],
  ['windows', 'windows'],
  ['windows-pc', 'windows'],
  ['windows_pc', 'windows'],
  ['ms-dos', 'ms-dos'],
  ['ms_dos', 'ms-dos'],
  ['gba', 'gba'],
  ['game-boy-advance', 'gba'],
  ['arcade', 'arcade'],
  ['scummvm', 'scummvm'],
])

export const POWERED_BY_BRAND_IDS = [
  'steam',
  'xbox',
  'xcloud',
  'gog',
  'igdb',
  'rawg',
  'hltb',
  'launchbox',
  'retroachievements',
  'mame',
  'google-drive',
  'smb',
  'epic-games',
  'tgdb',
  'scummvm',
] as const

export const SHIPPED_ICON_BRAND_IDS = BRAND_DEFINITIONS.filter((brand) => brand.iconPath).map(
  (brand) => brand.id,
)

export function getBrandDefinition(id: string): BrandDefinition | null {
  return BRAND_BY_ID.get(id) ?? null
}

export function resolveBrandDefinition(value: string | undefined | null): BrandDefinition | null {
  if (!value) return null
  const normalized = normalizeKey(value)
  const alias = BRAND_ALIASES.get(normalized) ?? normalized
  return BRAND_BY_ID.get(alias) ?? null
}

export function brandLabel(value: string | undefined | null, fallback?: string): string {
  const brand = resolveBrandDefinition(value)
  return brand?.label ?? fallback ?? value ?? 'Unknown'
}
