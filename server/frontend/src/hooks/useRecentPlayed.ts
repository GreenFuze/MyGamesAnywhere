import { useCallback, useEffect, useMemo, useState } from 'react'
import {
  getFrontendConfig,
  setFrontendConfig,
  type FrontendConfig,
  type RecentPlayedEntry,
} from '@/api/client'

const STORAGE_KEY = 'mga.recentPlayed'
const MAX_RECENT_PLAYED = 6

function isRecentPlayedEntry(value: unknown): value is RecentPlayedEntry {
  if (!value || typeof value !== 'object') return false
  const candidate = value as Partial<RecentPlayedEntry>
  return (
    typeof candidate.gameId === 'string' &&
    candidate.gameId.length > 0 &&
    typeof candidate.title === 'string' &&
    candidate.title.length > 0 &&
    typeof candidate.platform === 'string' &&
    candidate.platform.length > 0 &&
    typeof candidate.launchUrl === 'string' &&
    candidate.launchUrl.length > 0 &&
    (candidate.launchKind === 'xcloud' || candidate.launchKind === 'browser') &&
    typeof candidate.launchedAt === 'string' &&
    candidate.launchedAt.length > 0
  )
}

function normalizeEntries(raw: unknown): RecentPlayedEntry[] {
  if (!Array.isArray(raw)) return []
  return raw
    .filter(isRecentPlayedEntry)
    .sort((a, b) => b.launchedAt.localeCompare(a.launchedAt))
    .slice(0, MAX_RECENT_PLAYED)
}

function readLocal(): RecentPlayedEntry[] {
  try {
    return normalizeEntries(JSON.parse(localStorage.getItem(STORAGE_KEY) ?? '[]'))
  } catch {
    return []
  }
}

function writeLocal(entries: RecentPlayedEntry[]) {
  try {
    localStorage.setItem(STORAGE_KEY, JSON.stringify(entries))
  } catch {
    /* ignore */
  }
}

function mergeEntries(
  existing: RecentPlayedEntry[],
  nextEntry: RecentPlayedEntry,
): RecentPlayedEntry[] {
  return [nextEntry, ...existing.filter((entry) => entry.gameId !== nextEntry.gameId)].slice(
    0,
    MAX_RECENT_PLAYED,
  )
}

function removeEntry(existing: RecentPlayedEntry[], gameId: string): RecentPlayedEntry[] {
  return existing.filter((entry) => entry.gameId !== gameId)
}

export function useRecentPlayed() {
  const [recentPlayed, setRecentPlayed] = useState<RecentPlayedEntry[]>(() => readLocal())

  useEffect(() => {
    let cancelled = false
    ;(async () => {
      try {
        const remote = await getFrontendConfig()
        if (cancelled) return
        const entries = normalizeEntries(remote.recentPlayed)
        setRecentPlayed(entries)
        writeLocal(entries)
      } catch {
        /* keep local values */
      }
    })()

    return () => {
      cancelled = true
    }
  }, [])

  const recordLaunch = useCallback(
    (entry: Omit<RecentPlayedEntry, 'launchedAt'> & { launchedAt?: string }) => {
      const nextEntry: RecentPlayedEntry = {
        ...entry,
        launchedAt: entry.launchedAt ?? new Date().toISOString(),
      }

      setRecentPlayed((prev) => {
        const next = mergeEntries(prev, nextEntry)
        writeLocal(next)
        return next
      })

      void (async () => {
        try {
          const remote = await getFrontendConfig()
          const existing = normalizeEntries(remote.recentPlayed)
          const next: FrontendConfig = {
            ...remote,
            recentPlayed: mergeEntries(existing, nextEntry),
          }
          await setFrontendConfig(next)
        } catch {
          /* local-only fallback */
        }
      })()
    },
    [],
  )

  const removeRecentPlayed = useCallback((gameId: string) => {
    setRecentPlayed((prev) => {
      const next = removeEntry(prev, gameId)
      writeLocal(next)
      return next
    })

    void (async () => {
      try {
        const remote = await getFrontendConfig()
        const existing = normalizeEntries(remote.recentPlayed)
        const next: FrontendConfig = {
          ...remote,
          recentPlayed: removeEntry(existing, gameId),
        }
        await setFrontendConfig(next)
      } catch {
        /* local-only fallback */
      }
    })()
  }, [])

  return useMemo(
    () => ({
      recentPlayed,
      recordLaunch,
      removeRecentPlayed,
    }),
    [recentPlayed, recordLaunch, removeRecentPlayed],
  )
}
