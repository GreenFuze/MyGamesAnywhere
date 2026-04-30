export type BrandDefinition = {
  id: string
  label: string
  iconPath?: string
  websiteUrl?: string
  description: string
  creditNote?: string
  tempResourceName?: string
  presentation?: 'default' | 'light_tile'
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
    iconPath: '/brands/rawg.svg',
    websiteUrl: 'https://rawg.io/',
    description: 'Metadata provider used for game facts when available.',
    creditNote: 'Temporary in-app monogram icon created locally.',
    presentation: 'light_tile',
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
    iconPath: '/brands/hltb.svg',
    websiteUrl: 'https://howlongtobeat.com/',
    description: 'Completion-time provider for story and completionist estimates.',
    creditNote: 'Temporary in-app monogram icon created locally.',
    presentation: 'light_tile',
  },
  {
    id: 'mobygames',
    label: 'MobyGames',
    iconPath: '/brands/mobygames.svg',
    websiteUrl: 'https://www.mobygames.com/',
    description: 'External reference site for game pages and release metadata.',
    creditNote: 'Temporary in-app monogram icon created locally.',
  },
  {
    id: 'pcgamingwiki',
    label: 'PCGamingWiki',
    iconPath: '/brands/pcgamingwiki.svg',
    websiteUrl: 'https://www.pcgamingwiki.com/',
    description: 'External reference for platform-specific compatibility and setup details.',
    creditNote: 'Temporary in-app monogram icon created locally.',
  },
  {
    id: 'wikipedia',
    label: 'Wikipedia',
    iconPath: '/brands/wikipedia.svg',
    websiteUrl: 'https://www.wikipedia.org/',
    description: 'External reference site for encyclopedic game coverage.',
    creditNote: 'Temporary in-app monogram icon created locally.',
  },
  {
    id: 'youtube',
    label: 'YouTube',
    iconPath: '/brands/youtube.svg',
    websiteUrl: 'https://www.youtube.com/',
    description: 'External video host for trailers and gameplay media.',
    creditNote: 'Temporary in-app icon created locally.',
  },
  {
    id: 'archive-org',
    label: 'Internet Archive',
    iconPath: '/brands/archive-org.svg',
    websiteUrl: 'https://archive.org/',
    description: 'External archive host for manuals, media, and reference assets.',
    creditNote: 'Temporary in-app monogram icon created locally.',
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
    iconPath: '/brands/epic-games.svg',
    websiteUrl: 'https://store.epicgames.com/',
    description: 'Storefront/source integration for Epic-linked games.',
    creditNote: 'Temporary in-app monogram icon created locally.',
    presentation: 'light_tile',
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
    label: 'GBA',
    iconPath: '/brands/gba.png',
    description: 'Platform mark used for Game Boy Advance titles.',
    creditNote: 'Icon sourced from the provided temp resource.',
    tempResourceName: 'GBA-logo.png',
  },
  {
    id: 'nes',
    label: 'NES',
    iconPath: '/brands/nes.png',
    description: 'Platform mark used for Nintendo Entertainment System titles.',
  },
  {
    id: 'snes',
    label: 'SNES',
    iconPath: '/brands/snes.png',
    description: 'Platform mark used for Super Nintendo titles.',
  },
  {
    id: 'gb',
    label: 'Game Boy',
    iconPath: '/brands/gb.png',
    description: 'Platform mark used for original Game Boy titles.',
  },
  {
    id: 'gbc',
    label: 'Game Boy Color',
    iconPath: '/brands/gbc.png',
    description: 'Platform mark used for Game Boy Color titles.',
  },
  {
    id: 'n64',
    label: 'Nintendo 64',
    iconPath: '/brands/n64.png',
    description: 'Platform mark used for Nintendo 64 titles.',
  },
  {
    id: 'genesis',
    label: 'Genesis',
    iconPath: '/brands/genesis.svg',
    description: 'Platform mark used for Sega Genesis / Mega Drive titles.',
  },
  {
    id: 'sega_master_system',
    label: 'Master System',
    iconPath: '/brands/sega_master_system.png',
    description: 'Platform mark used for Sega Master System titles.',
  },
  {
    id: 'game_gear',
    label: 'Game Gear',
    iconPath: '/brands/game_gear.png',
    description: 'Platform mark used for Sega Game Gear titles.',
  },
  {
    id: 'sega_cd',
    label: 'Sega CD',
    iconPath: '/brands/sega_cd.png',
    description: 'Platform mark used for Sega CD titles.',
  },
  {
    id: 'sega_32x',
    label: 'Sega 32X',
    iconPath: '/brands/sega_32x.png',
    description: 'Platform mark used for Sega 32X titles.',
  },
  {
    id: 'ps1',
    label: 'PS1',
    iconPath: '/brands/ps1.png',
    description: 'Platform mark used for PlayStation titles.',
  },
  {
    id: 'ps2',
    label: 'PS2',
    iconPath: '/brands/ps2.png',
    description: 'Platform mark used for PlayStation 2 titles.',
  },
  {
    id: 'ps3',
    label: 'PS3',
    iconPath: '/brands/ps3.png',
    description: 'Platform mark used for PlayStation 3 titles.',
  },
  {
    id: 'psp',
    label: 'PSP',
    iconPath: '/brands/psp.png',
    description: 'Platform mark used for PlayStation Portable titles.',
  },
  {
    id: 'ps4',
    label: 'PS4',
    iconPath: '/brands/ps4.png',
    description: 'Dormant staged platform mark for future PlayStation 4 support.',
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
  {
    id: 'emulatorjs',
    label: 'EmulatorJS',
    iconPath: '/brands/emulatorjs.svg',
    websiteUrl: 'https://emulatorjs.org/',
    description: 'Browser runtime used for cartridge and console playback in the embedded player.',
    creditNote: 'Logo based on the EmulatorJS mark provided for in-app attribution.',
  },
  {
    id: 'js-dos',
    label: 'js-dos',
    iconPath: '/brands/dosbox.svg',
    websiteUrl: 'https://js-dos.com/',
    description: 'Browser runtime used for DOS playback in the embedded player.',
    creditNote: 'Uses the bundled DOSBox icon for DOS runtime attribution.',
    presentation: 'light_tile',
  },
  {
    id: 'dosbox',
    label: 'DOSBox',
    iconPath: '/brands/dosbox.svg',
    websiteUrl: 'https://www.dosbox.com/',
    description: 'DOS emulation core used behind the js-dos browser runtime.',
    creditNote: 'Icon asset provided for MGA DOS runtime attribution.',
    presentation: 'light_tile',
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
  ['howlongtobeat', 'hltb'],
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
  ['mobygames', 'mobygames'],
  ['pcgamingwiki', 'pcgamingwiki'],
  ['wikipedia', 'wikipedia'],
  ['youtube', 'youtube'],
  ['archive-org', 'archive-org'],
  ['archive', 'archive-org'],
  ['windows', 'windows'],
  ['windows-pc', 'windows'],
  ['windows_pc', 'windows'],
  ['ms-dos', 'ms-dos'],
  ['ms_dos', 'ms-dos'],
  ['gba', 'gba'],
  ['game-boy-advance', 'gba'],
  ['arcade', 'arcade'],
  ['scummvm', 'scummvm'],
  ['emulatorjs', 'emulatorjs'],
  ['emulator-js', 'emulatorjs'],
  ['js-dos', 'js-dos'],
  ['jsdos', 'js-dos'],
  ['dosbox', 'dosbox'],
])

const BRAND_HOST_ALIASES = new Map<string, string>([
  ['store.steampowered.com', 'steam'],
  ['steamcommunity.com', 'steam'],
  ['xbox.com', 'xbox'],
  ['www.xbox.com', 'xbox'],
  ['gog.com', 'gog'],
  ['www.gog.com', 'gog'],
  ['igdb.com', 'igdb'],
  ['www.igdb.com', 'igdb'],
  ['rawg.io', 'rawg'],
  ['www.rawg.io', 'rawg'],
  ['howlongtobeat.com', 'hltb'],
  ['www.howlongtobeat.com', 'hltb'],
  ['launchbox-app.com', 'launchbox'],
  ['www.launchbox-app.com', 'launchbox'],
  ['gamesdb.launchbox-app.com', 'launchbox'],
  ['retroachievements.org', 'retroachievements'],
  ['www.retroachievements.org', 'retroachievements'],
  ['mobygames.com', 'mobygames'],
  ['www.mobygames.com', 'mobygames'],
  ['pcgamingwiki.com', 'pcgamingwiki'],
  ['www.pcgamingwiki.com', 'pcgamingwiki'],
  ['wikipedia.org', 'wikipedia'],
  ['www.wikipedia.org', 'wikipedia'],
  ['youtube.com', 'youtube'],
  ['www.youtube.com', 'youtube'],
  ['youtu.be', 'youtube'],
  ['archive.org', 'archive-org'],
  ['www.archive.org', 'archive-org'],
  ['arcade-museum.com', 'mame'],
  ['www.arcade-museum.com', 'mame'],
  ['store.epicgames.com', 'epic-games'],
  ['epicgames.com', 'epic-games'],
  ['www.epicgames.com', 'epic-games'],
  ['mamedev.org', 'mame'],
  ['www.mamedev.org', 'mame'],
  ['scummvm.org', 'scummvm'],
  ['www.scummvm.org', 'scummvm'],
  ['emulatorjs.org', 'emulatorjs'],
  ['www.emulatorjs.org', 'emulatorjs'],
  ['js-dos.com', 'js-dos'],
  ['www.js-dos.com', 'js-dos'],
  ['dosbox.com', 'dosbox'],
  ['www.dosbox.com', 'dosbox'],
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
  'scummvm',
  'emulatorjs',
  'js-dos',
  'dosbox',
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

export function resolveBrandDefinitionFromUrl(url: string | undefined | null): BrandDefinition | null {
  if (!url) return null

  try {
    const parsed = new URL(url)
    const hostname = parsed.hostname.toLowerCase()
    if ((hostname === 'xbox.com' || hostname === 'www.xbox.com') && parsed.pathname.startsWith('/play')) {
      return BRAND_BY_ID.get('xcloud') ?? null
    }
    const alias = BRAND_HOST_ALIASES.get(hostname)
    return alias ? BRAND_BY_ID.get(alias) ?? null : null
  } catch {
    return null
  }
}

export function brandLabel(value: string | undefined | null, fallback?: string): string {
  const brand = resolveBrandDefinition(value)
  return brand?.label ?? fallback ?? value ?? 'Unknown'
}
