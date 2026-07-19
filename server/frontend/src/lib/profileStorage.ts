const SELECTED_PROFILE_KEY = 'mga.selectedProfileId'
const PROFILE_STORAGE_VERSION = 2

export function selectedProfileIdFromStorage(): string {
  if (typeof window === 'undefined') return ''
  try {
    return window.localStorage.getItem(SELECTED_PROFILE_KEY)?.trim() ?? ''
  } catch {
    return ''
  }
}

export function profileStorageKey(base: string, profileId = selectedProfileIdFromStorage()): string {
  const owner = profileId.trim()
  if (!owner) return `mga.profile.v${PROFILE_STORAGE_VERSION}.unselected.${base}`
  return `mga.profile.v${PROFILE_STORAGE_VERSION}.${encodeURIComponent(owner)}.${base}`
}

// Ambiguous v1 player-owned browser data is discarded rather than attributed
// to whichever profile happens to be selected after this upgrade.
export function discardAmbiguousLegacyProfileStorage(): void {
  if (typeof window === 'undefined') return
  const exactLocal = ['mga.libraryPrefs', 'mga.libraryPrefs.library', 'mga.libraryPrefs.play', 'mga.recentPlayed']
  const exactSession = [
    'mga.activeScanJobId',
    'mga.bulkReclassifyQueue',
    'mga.settings.duplicates.markedForDeletion',
  ]
  const localPrefixes = [
    'mga.browserPlaySource.',
    'mga.browserPlayJsdosExecutable.',
    'mga.browserPlay.source.',
    'mga.browserPlay.jsdosExecutable.',
  ]
  const sessionPrefixes = ['mga.returnScroll.', 'mga.browserPlaySession.']
  try {
    for (const key of exactLocal) window.localStorage.removeItem(key)
    for (let index = window.localStorage.length - 1; index >= 0; index -= 1) {
      const key = window.localStorage.key(index)
      if (key && localPrefixes.some((prefix) => key.startsWith(prefix))) window.localStorage.removeItem(key)
    }
  } catch {
    // Best effort when storage is unavailable.
  }
  try {
    for (const key of exactSession) window.sessionStorage.removeItem(key)
    for (let index = window.sessionStorage.length - 1; index >= 0; index -= 1) {
      const key = window.sessionStorage.key(index)
      if (key && sessionPrefixes.some((prefix) => key.startsWith(prefix))) window.sessionStorage.removeItem(key)
    }
  } catch {
    // Best effort when storage is unavailable.
  }
}
