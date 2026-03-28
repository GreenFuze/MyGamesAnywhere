import type {
  GameDetailResponse,
  GameFileDTO,
  GameLaunchSourceDTO,
  SourceGameDetailDTO,
} from '@/api/client'

export type BrowserPlayRuntime = 'emulatorjs' | 'jsdos' | 'scummvm'

export type BrowserPlaySessionFile = {
  id: string
  path: string
  role: string
  size: number
  fileKind?: string
  url: string
}

export type BrowserPlaySession =
  | {
      runtime: 'emulatorjs'
      title: string
      sourceGameId: string
      gameUrl: string
      gameName: string
      core: string
    }
  | {
      runtime: 'jsdos'
      title: string
      sourceGameId: string
      rootFilePath: string
      files: BrowserPlaySessionFile[]
      bundleUrl?: string
    }
  | {
      runtime: 'scummvm'
      title: string
      sourceGameId: string
      launchDirectoryPath: string
      files: BrowserPlaySessionFile[]
    }

export type BrowserPlaySelection = {
  runtime: BrowserPlayRuntime
  sourceGame: SourceGameDetailDTO
  launchSource: GameLaunchSourceDTO
  rootFile: GameFileDTO | null
}

const SESSION_PREFIX = 'mga.browserPlaySession.'

const EMULATORJS_CORES: Record<string, string> = {
  nes: 'fceumm',
  snes: 'snes9x',
  gb: 'gambatte',
  gbc: 'gambatte',
  gba: 'mgba',
  genesis: 'picodrive',
  sega_master_system: 'picodrive',
  game_gear: 'picodrive',
  sega_cd: 'picodrive',
  sega_32x: 'picodrive',
  ps1: 'pcsx_rearmed',
  arcade: 'fbneo',
}

export function getBrowserPlayRuntime(platform: string): BrowserPlayRuntime | null {
  if (platform in EMULATORJS_CORES) return 'emulatorjs'
  if (platform === 'ms_dos') return 'jsdos'
  if (platform === 'scummvm') return 'scummvm'
  return null
}

export function browserPlayRuntimeLabel(runtime: BrowserPlayRuntime): string {
  switch (runtime) {
    case 'emulatorjs':
      return 'EmulatorJS'
    case 'jsdos':
      return 'js-dos'
    case 'scummvm':
      return 'ScummVM'
  }
}

export function runtimeSupportsSaveSync(runtime: BrowserPlayRuntime): boolean {
  return runtime === 'emulatorjs' || runtime === 'jsdos' || runtime === 'scummvm'
}

export function sessionSupportsSaveSync(session: BrowserPlaySession): boolean {
  if (session.runtime === 'jsdos') {
    return typeof session.bundleUrl === 'string' && session.bundleUrl.length > 0
  }
  return true
}

export function getEmulatorJsCore(platform: string): string | null {
  return EMULATORJS_CORES[platform] ?? null
}

export function buildPlayFileUrl(gameId: string, fileId: string): string {
  return `/api/games/${encodeURIComponent(gameId)}/play?file_id=${encodeURIComponent(fileId)}`
}

export function selectBrowserPlaySelection(game: GameDetailResponse): BrowserPlaySelection | null {
  const play = game.play
  if (!play?.available || !play.launch_sources || play.launch_sources.length === 0) {
    return null
  }

  const runtime = getBrowserPlayRuntime(game.platform)
  if (!runtime) return null

  for (const launchSource of play.launch_sources) {
    if (!launchSource.launchable) continue

    const sourceGame = game.source_games.find((candidate) => candidate.id === launchSource.source_game_id)
    if (!sourceGame) continue

    const rootFile = launchSource.root_file_id
      ? sourceGame.files.find((file) => file.id === launchSource.root_file_id) ?? null
      : null

    return { runtime, sourceGame, launchSource, rootFile }
  }

  return null
}

function buildSourceSessionFiles(
  game: GameDetailResponse,
  sourceGame: SourceGameDetailDTO,
): BrowserPlaySessionFile[] {
  return sourceGame.files.map((file) => ({
    id: file.id,
    path: file.path,
    role: file.role,
    size: file.size,
    fileKind: file.file_kind,
    url: buildPlayFileUrl(game.id, file.id),
  }))
}

function normalizePlayPath(path: string): string {
  return path.replaceAll('\\', '/').replace(/^\.\/+/, '').replace(/^\/+/, '')
}

function commonDirectoryPath(paths: string[]): string {
  const normalized = paths
    .map((path) => normalizePlayPath(path))
    .filter((path) => path.length > 0)
    .map((path) => path.split('/').slice(0, -1))

  if (normalized.length === 0) return ''

  const prefix = [...normalized[0]]
  for (let i = 1; i < normalized.length; i += 1) {
    let shared = 0
    while (
      shared < prefix.length &&
      shared < normalized[i].length &&
      prefix[shared] === normalized[i][shared]
    ) {
      shared += 1
    }
    prefix.length = shared
    if (prefix.length === 0) break
  }

  return prefix.join('/')
}

export function buildBrowserPlaySession(
  game: GameDetailResponse,
  selection: BrowserPlaySelection,
): BrowserPlaySession | null {
  if (selection.runtime === 'emulatorjs') {
    const core = getEmulatorJsCore(game.platform)
    if (!selection.rootFile || !core) return null

    return {
      runtime: 'emulatorjs',
      title: game.title,
      sourceGameId: selection.sourceGame.id,
      gameName: game.title,
      gameUrl: buildPlayFileUrl(game.id, selection.rootFile.id),
      core,
    }
  }

  if (selection.runtime === 'jsdos') {
    if (!selection.rootFile) return null

    const rootPath = selection.rootFile.path.toLowerCase()
    const isBundle = rootPath.endsWith('.jsdos') || rootPath.endsWith('.zip')

    return {
      runtime: 'jsdos',
      title: game.title,
      sourceGameId: selection.sourceGame.id,
      rootFilePath: selection.rootFile.path,
      files: buildSourceSessionFiles(game, selection.sourceGame),
      bundleUrl: isBundle ? buildPlayFileUrl(game.id, selection.rootFile.id) : undefined,
    }
  }

  return {
    runtime: 'scummvm',
    title: game.title,
    sourceGameId: selection.sourceGame.id,
    launchDirectoryPath: commonDirectoryPath(selection.sourceGame.files.map((file) => file.path)),
    files: buildSourceSessionFiles(game, selection.sourceGame),
  }
}

export function persistBrowserPlaySession(session: BrowserPlaySession): string {
  const token =
    typeof crypto !== 'undefined' && typeof crypto.randomUUID === 'function'
      ? crypto.randomUUID()
      : `${Date.now()}-${Math.random().toString(36).slice(2)}`
  sessionStorage.setItem(`${SESSION_PREFIX}${token}`, JSON.stringify(session))
  return token
}

export function clearBrowserPlaySession(token: string | null) {
  if (!token) return
  sessionStorage.removeItem(`${SESSION_PREFIX}${token}`)
}

export function buildBrowserPlayerUrl(runtime: BrowserPlayRuntime, token: string): string {
  return `/runtimes/${runtime === 'jsdos' ? 'jsdos' : runtime}/player.html?session=${encodeURIComponent(token)}`
}
