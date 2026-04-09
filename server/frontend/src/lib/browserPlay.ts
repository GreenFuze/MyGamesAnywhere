import type {
  GameDetailResponse,
  GameFileDTO,
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
      executablePath?: string
      launchDirectoryPath?: string
      files: BrowserPlaySessionFile[]
      bundleUrl?: string
    }
  | {
      runtime: 'scummvm'
      title: string
      sourceGameId: string
      launchDirectoryPath: string
      files: BrowserPlaySessionFile[]
      enginePluginFileName?: string
    }

export type BrowserPlaySelection = {
  runtime: BrowserPlayRuntime
  profile: BrowserSourceProfile
  sourceGame: SourceGameDetailDTO
  deliveryProfile: {
    profile: string
    mode: 'direct' | 'materialized' | 'unavailable'
    prepare_required?: boolean
    ready?: boolean
    root_file_id?: string
  }
  rootFile: GameFileDTO | null
}

export type BrowserSourceProfile =
  | 'browser.emulatorjs'
  | 'browser.jsdos'
  | 'browser.scummvm'

export type BrowserPlaySelectionIssue = {
  code:
    | 'unsupported_platform'
    | 'missing_launch_source'
    | 'missing_source_game'
    | 'missing_root_file'
    | 'missing_runtime_core'
    | 'missing_source_files'
    | 'missing_scummvm_files'
  message: string
}

const SESSION_PREFIX = 'mga.browserPlaySession.'
const SOURCE_PREFERENCE_PREFIX = 'mga.browserPlaySource.'
const JSDOS_EXECUTABLE_PREFERENCE_PREFIX = 'mga.browserPlayJsdosExecutable.'

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

export function browserPlayProfileForRuntime(runtime: BrowserPlayRuntime): BrowserSourceProfile {
  switch (runtime) {
    case 'emulatorjs':
      return 'browser.emulatorjs'
    case 'jsdos':
      return 'browser.jsdos'
    case 'scummvm':
      return 'browser.scummvm'
  }
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

export function buildPlayFileUrl(gameId: string, fileId: string, profile?: string): string {
  const query = new URLSearchParams({ file_id: fileId })
  if (profile) query.set('profile', profile)
  return `/api/games/${encodeURIComponent(gameId)}/play?${query.toString()}`
}

function sourcePreferenceKey(gameId: string, runtime: BrowserPlayRuntime): string {
  return `${SOURCE_PREFERENCE_PREFIX}${gameId}.${runtime}`
}

function jsdosExecutablePreferenceKey(gameId: string, sourceGameId: string): string {
  return `${JSDOS_EXECUTABLE_PREFERENCE_PREFIX}${gameId}.${sourceGameId}`
}

export function readBrowserPlaySourcePreference(
  gameId: string,
  runtime: BrowserPlayRuntime,
): string | null {
  if (typeof window === 'undefined') return null
  try {
    const value = window.localStorage.getItem(sourcePreferenceKey(gameId, runtime))
    return value && value.trim().length > 0 ? value : null
  } catch {
    return null
  }
}

export function writeBrowserPlaySourcePreference(
  gameId: string,
  runtime: BrowserPlayRuntime,
  sourceGameId: string,
) {
  if (typeof window === 'undefined') return
  try {
    window.localStorage.setItem(sourcePreferenceKey(gameId, runtime), sourceGameId)
  } catch {
    // Ignore localStorage write failures and keep the in-memory selection only.
  }
}

export function clearBrowserPlaySourcePreference(gameId: string, runtime: BrowserPlayRuntime) {
  if (typeof window === 'undefined') return
  try {
    window.localStorage.removeItem(sourcePreferenceKey(gameId, runtime))
  } catch {
    // Ignore localStorage remove failures.
  }
}

export function readBrowserPlayJsdosExecutablePreference(
  gameId: string,
  sourceGameId: string,
): string | null {
  if (typeof window === 'undefined') return null
  try {
    const value = window.localStorage.getItem(jsdosExecutablePreferenceKey(gameId, sourceGameId))
    return value && value.trim().length > 0 ? value : null
  } catch {
    return null
  }
}

export function writeBrowserPlayJsdosExecutablePreference(
  gameId: string,
  sourceGameId: string,
  executablePath: string,
) {
  if (typeof window === 'undefined') return
  try {
    window.localStorage.setItem(jsdosExecutablePreferenceKey(gameId, sourceGameId), executablePath)
  } catch {
    // Ignore localStorage write failures and keep the in-memory selection only.
  }
}

export function clearBrowserPlayJsdosExecutablePreference(gameId: string, sourceGameId: string) {
  if (typeof window === 'undefined') return
  try {
    window.localStorage.removeItem(jsdosExecutablePreferenceKey(gameId, sourceGameId))
  } catch {
    // Ignore localStorage remove failures.
  }
}

export function browserPlaySourceLabel(selection: BrowserPlaySelection): string {
  return selection.sourceGame.raw_title || selection.sourceGame.external_id
}

export function browserPlaySourceContext(selection: BrowserPlaySelection): string {
  const parts = [selection.sourceGame.plugin_id]
  if (selection.sourceGame.platform && selection.sourceGame.platform !== 'unknown') {
    parts.push(selection.sourceGame.platform)
  }
  if (selection.rootFile?.path) {
    parts.push(selection.rootFile.path)
  } else if (selection.sourceGame.root_path) {
    parts.push(selection.sourceGame.root_path)
  }
  return parts.join(' · ')
}

export function browserPlaySourceOptionLabel(selection: BrowserPlaySelection): string {
  const label = browserPlaySourceLabel(selection)
  const context = browserPlaySourceContext(selection)
  return context ? `${label} - ${context}` : label
}

export function listBrowserPlaySelections(game: GameDetailResponse): BrowserPlaySelection[] {
  const runtime = getBrowserPlayRuntime(game.platform)
  if (!runtime) return []
  const profile = browserPlayProfileForRuntime(runtime)

  const selections: BrowserPlaySelection[] = []

  for (const sourceGame of game.source_games) {
    const deliveryProfile = sourceGame.delivery?.profiles?.find((candidate) => candidate.profile === profile)
    if (!deliveryProfile || deliveryProfile.mode === 'unavailable') continue

    const rootFileId = deliveryProfile.root_file_id ?? sourceGame.play?.root_file_id
    const rootFile = rootFileId
      ? sourceGame.files.find((file) => file.id === rootFileId) ?? null
      : null

    selections.push({ runtime, profile, sourceGame, deliveryProfile, rootFile })
  }

  return selections
}

export function selectBrowserPlaySelection(
  game: GameDetailResponse,
  preferredSourceGameId?: string | null,
): BrowserPlaySelection | null {
  const selections = listBrowserPlaySelections(game)
  if (selections.length === 0) {
    return null
  }

  if (preferredSourceGameId) {
    const preferred = selections.find((selection) => selection.sourceGame.id === preferredSourceGameId)
    if (preferred) {
      return preferred
    }
  }

  return selections[0] ?? null
}

export function getBrowserPlaySelectionIssue(
  game: GameDetailResponse,
  selection: BrowserPlaySelection | null,
): BrowserPlaySelectionIssue | null {
  if (!getBrowserPlayRuntime(game.platform)) {
    return {
      code: 'unsupported_platform',
      message: `Browser Play is not enabled for ${game.platform}.`,
    }
  }

  if (listBrowserPlaySelections(game).length === 0) {
    return {
      code: 'missing_launch_source',
      message: 'No launchable source file was found for this game yet.',
    }
  }

  if (!selection) {
    return {
      code: 'missing_source_game',
      message: 'The selected browser-play source record could not be resolved from the game detail payload.',
    }
  }

  if (selection.runtime === 'emulatorjs') {
    if (!selection.rootFile) {
      return {
        code: 'missing_root_file',
        message: `EmulatorJS needs a root launch file for "${selection.sourceGame.raw_title || game.title}".`,
      }
    }
    if (!getEmulatorJsCore(game.platform)) {
      return {
        code: 'missing_runtime_core',
        message: `No EmulatorJS core is mapped for ${game.platform}.`,
      }
    }
    return null
  }

  if (selection.runtime === 'jsdos') {
    if (!selection.rootFile) {
      return {
        code: 'missing_root_file',
        message: `js-dos needs a root launch file for "${selection.sourceGame.raw_title || game.title}".`,
      }
    }
    if (selection.sourceGame.files.length === 0) {
      return {
        code: 'missing_source_files',
        message: `No source files were attached to "${selection.sourceGame.raw_title || game.title}".`,
      }
    }
    return null
  }

  if (selection.sourceGame.files.length === 0) {
    return {
      code: 'missing_scummvm_files',
      message: `ScummVM needs source files before "${selection.sourceGame.raw_title || game.title}" can launch.`,
    }
  }

  return null
}

function buildSourceSessionFiles(
  game: GameDetailResponse,
  sourceGame: SourceGameDetailDTO,
  profile: BrowserSourceProfile,
): BrowserPlaySessionFile[] {
  return sourceGame.files.map((file) => ({
    id: file.id,
    path: file.path,
    role: file.role,
    size: file.size,
    fileKind: file.file_kind,
    url: buildPlayFileUrl(game.id, file.id, profile),
  }))
}

function normalizePlayPath(path: string): string {
  return path.replaceAll('\\', '/').replace(/^\.\/+/, '').replace(/^\/+/, '')
}

function isJsdosExecutablePath(path: string): boolean {
  const normalized = normalizePlayPath(path).toLowerCase()
  return normalized.endsWith('.exe') || normalized.endsWith('.com') || normalized.endsWith('.bat')
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

function inferScummvmEnginePlugin(files: Array<{ path: string }>): string | null {
  const names = new Set(
    files.map((file) => normalizePlayPath(file.path).split('/').pop()?.toLowerCase() ?? '').filter(Boolean),
  )
  const hasAny = (...candidates: string[]) => candidates.some((candidate) => names.has(candidate))
  const hasPrefix = (prefix: string, suffix = '') =>
    Array.from(names).some((name) => name.startsWith(prefix) && name.endsWith(suffix))
  const hasExtension = (extension: string) => Array.from(names).some((name) => name.endsWith(extension))

  if (names.has('resource.map') && hasAny('resource.000', 'resource.001')) {
    return 'libsci.so'
  }
  if (
    names.has('monster.sou') ||
    (names.has('atlantis.000') && names.has('atlantis.001')) ||
    names.has('comi.la0') ||
    hasPrefix('monkey.')
  ) {
    return 'libscumm.so'
  }
  if ((names.has('english.smp') && names.has('english.idx')) || (names.has('dw.scn') && hasExtension('.scn'))) {
    return 'libtinsel.so'
  }
  if (names.has('hd.blb') && hasAny('a.blb', 'c.blb', 't.blb')) {
    return 'libneverhood.so'
  }
  if (hasAny('intro.stk', 'gobnew.lic')) {
    return 'libgob.so'
  }
  if (hasAny('dragon.res', 'dragon.sph')) {
    return 'libdragons.so'
  }
  if ((hasPrefix('bb', '.000') || hasPrefix('game', '.vnm')) && hasAny('bbloogie.000', 'bbtennis.000')) {
    return 'libbbvs.so'
  }
  if (names.has('sky.dnr') && names.has('sky.dsk')) {
    return 'libsky.so'
  }
  if (names.has('queen.1') && names.has('queen.1c')) {
    return 'libqueen.so'
  }
  if (names.has('toon.dat')) {
    return 'libtoon.so'
  }
  if (names.has('touche.dat')) {
    return 'libtouche.so'
  }
  if (names.has('drascula.dat')) {
    return 'libdrascula.so'
  }
  if (names.has('lure.dat')) {
    return 'liblure.so'
  }

  return null
}

export function listBrowserPlayJsdosExecutables(files: Array<{ path: string }>): string[] {
  return files
    .filter((file) => isJsdosExecutablePath(file.path))
    .sort((left, right) => normalizePlayPath(left.path).localeCompare(normalizePlayPath(right.path)))
    .map((file) => file.path)
}

export function browserPlayJsdosExecutableLabel(
  executablePath: string,
  files: Array<{ path: string }>,
): string {
  const launchDirectoryPath = commonDirectoryPath(files.map((file) => file.path))
  const normalizedPath = normalizePlayPath(executablePath)
  const normalizedLaunchDirectoryPath = normalizePlayPath(launchDirectoryPath)
  if (
    normalizedLaunchDirectoryPath &&
    normalizedPath.startsWith(`${normalizedLaunchDirectoryPath}/`)
  ) {
    return normalizedPath.slice(normalizedLaunchDirectoryPath.length + 1)
  }
  return normalizedPath
}

type BuildBrowserPlaySessionOptions = {
  jsdosExecutablePath?: string | null
}

export function buildBrowserPlaySession(
  game: GameDetailResponse,
  selection: BrowserPlaySelection,
  options?: BuildBrowserPlaySessionOptions,
): BrowserPlaySession | null {
  if (selection.runtime === 'emulatorjs') {
    const core = getEmulatorJsCore(game.platform)
    if (!selection.rootFile || !core) return null

    return {
      runtime: 'emulatorjs',
      title: game.title,
      sourceGameId: selection.sourceGame.id,
      gameName: game.title,
      gameUrl: buildPlayFileUrl(game.id, selection.rootFile.id, selection.profile),
      core,
    }
  }

  if (selection.runtime === 'jsdos') {
    if (!selection.rootFile) return null

    const rootPath = selection.rootFile.path.toLowerCase()
    const isBundle = rootPath.endsWith('.jsdos') || rootPath.endsWith('.zip')
    const launchDirectoryPath = commonDirectoryPath(selection.sourceGame.files.map((file) => file.path))
    const availableExecutables = listBrowserPlayJsdosExecutables(selection.sourceGame.files)
    const normalizedAvailableExecutables = new Set(availableExecutables.map((path) => normalizePlayPath(path)))
    const preferredExecutablePath =
      options?.jsdosExecutablePath && normalizedAvailableExecutables.has(normalizePlayPath(options.jsdosExecutablePath))
        ? options.jsdosExecutablePath
        : null
    const executablePath =
      preferredExecutablePath ??
      (normalizedAvailableExecutables.has(normalizePlayPath(selection.rootFile.path))
        ? selection.rootFile.path
        : (availableExecutables[0] ?? selection.rootFile.path))

    return {
      runtime: 'jsdos',
      title: game.title,
      sourceGameId: selection.sourceGame.id,
      rootFilePath: executablePath,
      executablePath,
      launchDirectoryPath,
      files: buildSourceSessionFiles(game, selection.sourceGame, selection.profile),
      bundleUrl: isBundle ? buildPlayFileUrl(game.id, selection.rootFile.id, selection.profile) : undefined,
    }
  }

  return {
    runtime: 'scummvm',
    title: game.title,
    sourceGameId: selection.sourceGame.id,
    launchDirectoryPath: commonDirectoryPath(selection.sourceGame.files.map((file) => file.path)),
    files: buildSourceSessionFiles(game, selection.sourceGame, selection.profile),
    enginePluginFileName: inferScummvmEnginePlugin(selection.sourceGame.files) ?? undefined,
  }
}

export function browserPlaySelectionRequiresPrepare(selection: BrowserPlaySelection): boolean {
  return selection.deliveryProfile.mode === 'materialized'
}

export function browserPlaySelectionIsReady(selection: BrowserPlaySelection): boolean {
  return selection.deliveryProfile.mode === 'direct' || selection.deliveryProfile.ready === true
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
