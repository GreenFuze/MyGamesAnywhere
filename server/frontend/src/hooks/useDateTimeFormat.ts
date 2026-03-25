import { useCallback, useEffect, useMemo, useState } from 'react'
import { getFrontendConfig, setFrontendConfig } from '@/api/client'

// ---------------------------------------------------------------------------
// Types
// ---------------------------------------------------------------------------

export type DateFormat = 'd/M/yyyy' | 'M/d/yyyy'
export type TimeFormat = '12h' | '24h'

export type DateTimePrefs = {
  dateFormat: DateFormat
  timeFormat: TimeFormat
}

// ---------------------------------------------------------------------------
// Constants
// ---------------------------------------------------------------------------

const STORAGE_KEY = 'mga.dateTimeFormat'

const DEFAULTS: DateTimePrefs = {
  dateFormat: 'M/d/yyyy',
  timeFormat: '12h',
}

// ---------------------------------------------------------------------------
// Formatting utility
// ---------------------------------------------------------------------------

/**
 * Format an ISO date string using the given date/time preferences.
 * Returns a human-readable string like "3/24/2026 2:30 PM" or "24/3/2026 14:30".
 */
export function formatDateTime(iso: string, prefs: DateTimePrefs): string {
  const d = new Date(iso)
  if (isNaN(d.getTime())) return iso

  // Date part.
  const day = d.getDate()
  const month = d.getMonth() + 1
  const year = d.getFullYear()

  const datePart =
    prefs.dateFormat === 'd/M/yyyy'
      ? `${day}/${month}/${year}`
      : `${month}/${day}/${year}`

  // Time part.
  const hours = d.getHours()
  const minutes = d.getMinutes().toString().padStart(2, '0')

  let timePart: string
  if (prefs.timeFormat === '24h') {
    timePart = `${hours.toString().padStart(2, '0')}:${minutes}`
  } else {
    const h12 = hours % 12 || 12
    const ampm = hours < 12 ? 'AM' : 'PM'
    timePart = `${h12}:${minutes} ${ampm}`
  }

  return `${datePart} ${timePart}`
}

// ---------------------------------------------------------------------------
// Local storage helpers
// ---------------------------------------------------------------------------

function readLocal(): Partial<DateTimePrefs> {
  try {
    const raw = localStorage.getItem(STORAGE_KEY)
    if (raw) return JSON.parse(raw) as Partial<DateTimePrefs>
  } catch {
    /* private mode or corrupt data */
  }
  return {}
}

function writeLocal(prefs: DateTimePrefs) {
  try {
    localStorage.setItem(STORAGE_KEY, JSON.stringify(prefs))
  } catch {
    /* ignore */
  }
}

// ---------------------------------------------------------------------------
// Hook — mirrors useLibraryPrefs pattern
// ---------------------------------------------------------------------------

export function useDateTimeFormat() {
  const [prefs, setPrefsState] = useState<DateTimePrefs>(() => ({
    ...DEFAULTS,
    ...readLocal(),
  }))

  // On mount: fetch server config and merge date/time prefs over local state.
  useEffect(() => {
    let cancelled = false
    ;(async () => {
      try {
        const remote = await getFrontendConfig()
        if (cancelled) return

        const merged: DateTimePrefs = { ...DEFAULTS }

        if (remote.dateFormat === 'd/M/yyyy' || remote.dateFormat === 'M/d/yyyy') {
          merged.dateFormat = remote.dateFormat
        }
        if (remote.timeFormat === '12h' || remote.timeFormat === '24h') {
          merged.timeFormat = remote.timeFormat
        }

        setPrefsState(merged)
        writeLocal(merged)
      } catch {
        /* keep local values */
      }
    })()
    return () => { cancelled = true }
  }, [])

  // Persist a partial update: optimistic local, then read-then-write server.
  const patchPrefs = useCallback((patch: Partial<DateTimePrefs>) => {
    setPrefsState((prev) => {
      const next = { ...prev, ...patch }
      writeLocal(next)
      return next
    })

    // Async server sync (read-then-write to avoid clobbering other keys).
    void (async () => {
      try {
        const remote = await getFrontendConfig()
        await setFrontendConfig({ ...remote, ...patch })
      } catch {
        /* local-only fallback */
      }
    })()
  }, [])

  // Convenience setters.
  const setDateFormat = useCallback(
    (fmt: DateFormat) => patchPrefs({ dateFormat: fmt }),
    [patchPrefs],
  )

  const setTimeFormat = useCallback(
    (fmt: TimeFormat) => patchPrefs({ timeFormat: fmt }),
    [patchPrefs],
  )

  // Bound formatter that uses current prefs.
  const format = useCallback(
    (iso: string) => formatDateTime(iso, prefs),
    [prefs],
  )

  return useMemo(
    () => ({ prefs, setDateFormat, setTimeFormat, format }),
    [prefs, setDateFormat, setTimeFormat, format],
  )
}
