import { useCallback, useEffect, useMemo, useState } from 'react'
import {
  getFrontendConfig,
  setFrontendConfig,
  type LibraryPrefs,
} from '@/api/client'

// ---------------------------------------------------------------------------
// Constants
// ---------------------------------------------------------------------------

const STORAGE_KEY = 'mga.libraryPrefs'

const DEFAULTS: LibraryPrefs = {
  viewMode: 'grid',
  sortBy: 'title',
  sortDir: 'asc',
}

// ---------------------------------------------------------------------------
// Local storage helpers
// ---------------------------------------------------------------------------

function readLocal(): Partial<LibraryPrefs> {
  try {
    const raw = localStorage.getItem(STORAGE_KEY)
    if (raw) return JSON.parse(raw) as Partial<LibraryPrefs>
  } catch {
    /* private mode or corrupt data */
  }
  return {}
}

function writeLocal(prefs: LibraryPrefs) {
  try {
    localStorage.setItem(STORAGE_KEY, JSON.stringify(prefs))
  } catch {
    /* ignore */
  }
}

// ---------------------------------------------------------------------------
// Hook — mirrors ThemeProvider pattern
// ---------------------------------------------------------------------------

export function useLibraryPrefs() {
  const [prefs, setPrefsState] = useState<LibraryPrefs>(() => ({
    ...DEFAULTS,
    ...readLocal(),
  }))

  // On mount: fetch server config and merge library prefs over local state
  useEffect(() => {
    let cancelled = false
    ;(async () => {
      try {
        const remote = await getFrontendConfig()
        if (cancelled) return

        const merged: LibraryPrefs = { ...DEFAULTS }

        // Extract library-related keys from remote config
        if (remote.viewMode === 'grid' || remote.viewMode === 'list') {
          merged.viewMode = remote.viewMode
        }
        if (typeof remote.sortBy === 'string') {
          merged.sortBy = remote.sortBy as LibraryPrefs['sortBy']
        }
        if (remote.sortDir === 'asc' || remote.sortDir === 'desc') {
          merged.sortDir = remote.sortDir
        }

        setPrefsState(merged)
        writeLocal(merged)
      } catch {
        /* keep local values */
      }
    })()
    return () => { cancelled = true }
  }, [])

  // Persist a partial update: optimistic local, then read-then-write server
  const patchPrefs = useCallback((patch: Partial<LibraryPrefs>) => {
    setPrefsState((prev) => {
      const next = { ...prev, ...patch }
      writeLocal(next)
      return next
    })

    // Async server sync (read-then-write to avoid clobbering themeId)
    void (async () => {
      try {
        const remote = await getFrontendConfig()
        await setFrontendConfig({ ...remote, ...patch })
      } catch {
        /* local-only fallback */
      }
    })()
  }, [])

  // Typed convenience setters
  const setViewMode = useCallback(
    (mode: LibraryPrefs['viewMode']) => patchPrefs({ viewMode: mode }),
    [patchPrefs],
  )

  const setSortBy = useCallback(
    (sortBy: LibraryPrefs['sortBy']) => patchPrefs({ sortBy }),
    [patchPrefs],
  )

  const setSortDir = useCallback(
    (sortDir: LibraryPrefs['sortDir']) => patchPrefs({ sortDir }),
    [patchPrefs],
  )

  return useMemo(
    () => ({ prefs, setViewMode, setSortBy, setSortDir }),
    [prefs, setViewMode, setSortBy, setSortDir],
  )
}
