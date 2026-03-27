export type GameOriginLabel = 'Home' | 'Library' | 'Play'

export type GameRouteState = {
  from: string
  scrollY: number
  originLabel: GameOriginLabel
}

export type ReturnRouteState = {
  restoreScroll?: boolean
}

const STORAGE_PREFIX = 'mga.returnScroll.'

function storageKey(route: string): string {
  return `${STORAGE_PREFIX}${route}`
}

export function inferOriginLabel(pathname: string): GameOriginLabel {
  if (pathname === '/' || pathname.startsWith('/?')) return 'Home'
  return pathname.startsWith('/play') ? 'Play' : 'Library'
}

export function buildGameRouteState(
  pathname: string,
  search: string,
  scrollY = window.scrollY,
): GameRouteState {
  const from = `${pathname}${search}`
  const state: GameRouteState = {
    from,
    scrollY: Math.max(0, Math.floor(scrollY)),
    originLabel: inferOriginLabel(pathname),
  }

  try {
    sessionStorage.setItem(storageKey(from), String(state.scrollY))
  } catch {
    /* ignore storage errors */
  }

  return state
}

export function readGameRouteState(state: unknown): GameRouteState | null {
  if (!state || typeof state !== 'object') return null

  const candidate = state as Partial<GameRouteState>
  if (typeof candidate.from !== 'string' || candidate.from.length === 0) {
    return null
  }

  const originLabel =
    candidate.originLabel === 'Home' ||
    candidate.originLabel === 'Play' ||
    candidate.originLabel === 'Library'
      ? candidate.originLabel
      : inferOriginLabel(candidate.from)

  return {
    from: candidate.from,
    scrollY:
      typeof candidate.scrollY === 'number' && Number.isFinite(candidate.scrollY)
        ? candidate.scrollY
        : 0,
    originLabel,
  }
}

export function shouldRestoreRouteScroll(state: unknown): boolean {
  if (!state || typeof state !== 'object') return false
  return (state as ReturnRouteState).restoreScroll === true
}

export function consumeStoredRouteScroll(pathname: string, search: string): number | null {
  try {
    const key = storageKey(`${pathname}${search}`)
    const raw = sessionStorage.getItem(key)
    if (raw === null) return null

    sessionStorage.removeItem(key)
    const parsed = Number(raw)
    return Number.isFinite(parsed) && parsed >= 0 ? parsed : null
  } catch {
    return null
  }
}
