import { useCallback, useEffect, useMemo, useState } from 'react'
import {
  getFrontendConfig,
  setFrontendConfig,
  type LibraryPrefs,
} from '@/api/client'
import { defaultSections, sanitizeSections } from '@/lib/collectionSections'

// ---------------------------------------------------------------------------
// Constants
// ---------------------------------------------------------------------------

type LibraryPrefsPage = 'library' | 'play'

const DEFAULTS: LibraryPrefs = {
  viewMode: 'shelf',
  sortBy: 'title',
  sortDir: 'asc',
  sections: defaultSections(),
  expandedSectionId: null,
}

// ---------------------------------------------------------------------------
// Local storage helpers
// ---------------------------------------------------------------------------

function storageKey(page: LibraryPrefsPage): string {
  return `mga.libraryPrefs.${page}`
}

function readLegacyLocal(): Partial<LibraryPrefs> {
  try {
    const raw = localStorage.getItem('mga.libraryPrefs')
    if (raw) return JSON.parse(raw) as Partial<LibraryPrefs>
  } catch {
    /* private mode or corrupt data */
  }
  return {}
}

function readLocal(page: LibraryPrefsPage): Partial<LibraryPrefs> {
  try {
    const raw = localStorage.getItem(storageKey(page))
    if (raw) return JSON.parse(raw) as Partial<LibraryPrefs>
  } catch {
    /* private mode or corrupt data */
  }
  return readLegacyLocal()
}

function writeLocal(page: LibraryPrefsPage, prefs: LibraryPrefs) {
  try {
    localStorage.setItem(storageKey(page), JSON.stringify(prefs))
  } catch {
    /* ignore */
  }
}

function extractPrefs(raw: unknown): LibraryPrefs | null {
  if (!raw || typeof raw !== 'object') return null
  const source = raw as Record<string, unknown>
  const next: LibraryPrefs = { ...DEFAULTS }
  let found = false

  if (
    source.viewMode === 'shelf' ||
    source.viewMode === 'accordion' ||
    source.viewMode === 'grid' ||
    source.viewMode === 'list' ||
    source.viewMode === 'timeline'
  ) {
    if (source.viewMode === 'accordion') {
      next.viewMode = 'shelf'
    } else {
      next.viewMode = source.viewMode === 'list' ? 'grid' : source.viewMode
    }
    found = true
  }
  if (typeof source.sortBy === 'string') {
    next.sortBy = source.sortBy as LibraryPrefs['sortBy']
    found = true
  }
  if (source.sortDir === 'asc' || source.sortDir === 'desc') {
    next.sortDir = source.sortDir
    found = true
  }
  if (Array.isArray(source.sections)) {
    next.sections = sanitizeSections(source.sections)
    found = true
  }
  if (typeof source.expandedSectionId === 'string' || source.expandedSectionId === null) {
    next.expandedSectionId = source.expandedSectionId
    found = true
  }

  return found ? next : null
}

// ---------------------------------------------------------------------------
// Hook — mirrors ThemeProvider pattern
// ---------------------------------------------------------------------------

export function useLibraryPrefs(page: LibraryPrefsPage) {
  const [prefs, setPrefsState] = useState<LibraryPrefs>(() => ({
    ...DEFAULTS,
    ...readLocal(page),
  }))

  // On mount: fetch server config and merge library prefs over local state
  useEffect(() => {
    let cancelled = false
    ;(async () => {
      try {
        const remote = await getFrontendConfig()
        if (cancelled) return

        const pageKey = page === 'play' ? 'playPrefs' : 'libraryPrefs'
        const scoped = extractPrefs(remote[pageKey])
        const legacy = extractPrefs(remote)
        const merged = scoped ?? legacy ?? { ...DEFAULTS }

        if (!scoped && legacy) {
          void setFrontendConfig({ ...remote, [pageKey]: legacy })
        }

        setPrefsState(merged)
        writeLocal(page, merged)
      } catch {
        /* keep local values */
      }
    })()
    return () => { cancelled = true }
  }, [page])

  // Persist a partial update: optimistic local, then read-then-write server
  const patchPrefs = useCallback((patch: Partial<LibraryPrefs>) => {
    setPrefsState((prev) => {
      const next = { ...prev, ...patch }
      writeLocal(page, next)
      return next
    })

    // Async server sync (read-then-write to avoid clobbering themeId)
    void (async () => {
      try {
        const remote = await getFrontendConfig()
        const pageKey = page === 'play' ? 'playPrefs' : 'libraryPrefs'
        const current = extractPrefs(remote[pageKey]) ?? extractPrefs(remote) ?? DEFAULTS
        await setFrontendConfig({ ...remote, [pageKey]: { ...current, ...patch } })
      } catch {
        /* local-only fallback */
      }
    })()
  }, [page])

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

  const setSections = useCallback(
    (sections: LibraryPrefs['sections']) => patchPrefs({ sections: sanitizeSections(sections) }),
    [patchPrefs],
  )

  const setExpandedSectionId = useCallback(
    (expandedSectionId: string | null) => patchPrefs({ expandedSectionId }),
    [patchPrefs],
  )

  return useMemo(
    () => ({
      prefs,
      setViewMode,
      setSortBy,
      setSortDir,
      setSections,
      setExpandedSectionId,
    }),
    [prefs, setViewMode, setSortBy, setSortDir, setSections, setExpandedSectionId],
  )
}
