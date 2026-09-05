/**
 * The words for the server's own settings.
 *
 * The API reports keys and values because more than one frontend reads it. The
 * translation into something a person can act on lives here, in one place, so
 * a setting is not described two different ways on two different screens.
 */

export type NetworkReach = 'this-computer' | 'whole-network' | 'one-address'

export type ReachDescription = {
  reach: NetworkReach
  headline: string
  detail: string
}

const LOOPBACK = new Set(['127.0.0.1', 'localhost', '::1'])
const EVERY_ADDRESS = new Set(['0.0.0.0', '::'])

/** Who can currently reach this server, from the address it listens on. */
export function describeReach(listenIP: string): ReachDescription {
  const address = (listenIP ?? '').trim()
  if (EVERY_ADDRESS.has(address)) {
    return {
      reach: 'whole-network',
      headline: 'Any device on your network',
      detail: 'A TV, a phone or another PC on the same network can reach MGA using this computer’s address.',
    }
  }
  if (LOOPBACK.has(address.toLowerCase())) {
    return {
      reach: 'this-computer',
      headline: 'Only this computer',
      detail: 'Nothing else on your network can reach MGA, so an app on another device cannot connect to it.',
    }
  }
  return {
    reach: 'one-address',
    headline: `Only through ${address || 'an address that is not set'}`,
    detail: 'MGA answers on that one address. Devices that reach this computer another way will not find it.',
  }
}

/** How long the server has been running, in the roughest useful unit. */
export function formatUptime(seconds: number): string {
  if (!Number.isFinite(seconds) || seconds < 0) return 'Unknown'
  if (seconds < 60) return 'Less than a minute'
  const minutes = Math.floor(seconds / 60)
  if (minutes < 60) return `${minutes} minute${minutes === 1 ? '' : 's'}`
  const hours = Math.floor(minutes / 60)
  if (hours < 24) return `${hours} hour${hours === 1 ? '' : 's'}`
  const days = Math.floor(hours / 24)
  return `${days} day${days === 1 ? '' : 's'}`
}

const LABELS: Record<string, string> = {
  LISTEN_IP: 'Address MGA answers on',
  PORT: 'Port',
  APP_INSTALL_TYPE: 'How MGA was installed',
  DB_PATH: 'Your library',
  MEDIA_ROOT: 'Downloaded pictures',
  SOURCE_CACHE_ROOT: 'Game files kept ready to send',
  PLUGINS_DIR: 'Provider plugins',
  FRONTEND_DIST: 'This web page',
  UPDATES_DIR: 'Downloaded updates',
  LOG_FILE: 'Server log',
  LOG_MAX_SIZE_MB: 'Log size before it starts a new one',
  LOG_MAX_BACKUPS: 'Old logs kept',
  MEDIA_DOWNLOAD_CONCURRENCY: 'Pictures downloaded at once',
  UPDATE_MANIFEST_URL: 'Where updates are looked for',
}

/** An unknown key is shown as itself rather than hidden or renamed. */
export function settingLabel(key: string): string {
  return LABELS[key] ?? key
}

const STORAGE_ORDER = ['DB_PATH', 'MEDIA_ROOT', 'SOURCE_CACHE_ROOT', 'PLUGINS_DIR', 'UPDATES_DIR']

/** The storage locations worth showing, in the order a person would look. */
export function orderStorage<T extends { key: string }>(locations: T[]): T[] {
  const rank = (key: string) => {
    const index = STORAGE_ORDER.indexOf(key)
    return index === -1 ? STORAGE_ORDER.length : index
  }
  return [...locations].sort((a, b) => rank(a.key) - rank(b.key))
}

/**
 * Settings that are not network settings and not a storage path: the small
 * numbers and URLs worth showing once, without repeating a path already listed
 * above them.
 */
export function otherSettings<T extends { key: string }>(settings: T[]): T[] {
  const shown = new Set([...STORAGE_ORDER, 'LISTEN_IP', 'PORT', 'FRONTEND_DIST', 'LOG_FILE'])
  return settings.filter((setting) => !shown.has(setting.key))
}
