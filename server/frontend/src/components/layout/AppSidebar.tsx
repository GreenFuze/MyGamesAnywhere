import { useEffect, useMemo, useState } from 'react'
import {
  ChevronDown,
  ChevronRight,
  PlayCircle,
} from 'lucide-react'
import { useLocation, useNavigate } from 'react-router-dom'
import {
  getFrontendConfig,
  setFrontendConfig,
  type FrontendConfig,
  type GameDetailResponse,
} from '@/api/client'
import { CoverImage } from '@/components/ui/cover-image'
import { PlatformIcon } from '@/components/ui/platform-icon'
import { useLibraryData } from '@/hooks/useLibraryData'
import { useRecentPlayed } from '@/hooks/useRecentPlayed'
import { buildGameRouteState } from '@/lib/gameNavigation'
import {
  isActionable,
  isPlayable,
  platformLabel,
  primarySourcePlugin,
  selectCoverUrl,
  sourceLabel,
} from '@/lib/gameUtils'

type SidebarGroup = {
  platform: string
  label: string
  games: GameDetailResponse[]
}

type DesktopSidebarPrefs = {
  collapsedPlatforms: Record<string, boolean>
}

const SIDEBAR_STORAGE_KEY = 'mga.desktopSidebarPrefs'
const DEFAULT_SIDEBAR_PREFS: DesktopSidebarPrefs = {
  collapsedPlatforms: {},
}

function extractSidebarPrefs(raw: unknown): DesktopSidebarPrefs | null {
  if (!raw || typeof raw !== 'object') return null
  const source = raw as Record<string, unknown>
  if (!source.collapsedPlatforms || typeof source.collapsedPlatforms !== 'object') {
    return DEFAULT_SIDEBAR_PREFS
  }

  const next: Record<string, boolean> = {}
  for (const [key, value] of Object.entries(source.collapsedPlatforms as Record<string, unknown>)) {
    if (typeof value === 'boolean') {
      next[key] = value
    }
  }

  return { collapsedPlatforms: next }
}

function readLocalSidebarPrefs(): DesktopSidebarPrefs {
  try {
    const raw = localStorage.getItem(SIDEBAR_STORAGE_KEY)
    if (!raw) return DEFAULT_SIDEBAR_PREFS
    return extractSidebarPrefs(JSON.parse(raw)) ?? DEFAULT_SIDEBAR_PREFS
  } catch {
    return DEFAULT_SIDEBAR_PREFS
  }
}

function writeLocalSidebarPrefs(prefs: DesktopSidebarPrefs) {
  try {
    localStorage.setItem(SIDEBAR_STORAGE_KEY, JSON.stringify(prefs))
  } catch {
    /* ignore */
  }
}

function statusHint(game: GameDetailResponse): string | null {
  if (game.xcloud_available) return 'xCloud'
  if (game.is_game_pass) return 'Game Pass'
  if (isPlayable(game)) return 'Browser Play'
  return null
}

export function AppSidebar() {
  const location = useLocation()
  const navigate = useNavigate()
  const { data: allGames = [] } = useLibraryData()
  const { recentPlayed } = useRecentPlayed()
  const [sidebarPrefs, setSidebarPrefsState] = useState<DesktopSidebarPrefs>(() =>
    readLocalSidebarPrefs(),
  )
  const [sidebarQuery, setSidebarQuery] = useState('')

  const playableGroups = useMemo<SidebarGroup[]>(() => {
    const grouped = new Map<string, GameDetailResponse[]>()
    const query = sidebarQuery.trim().toLowerCase()

    for (const game of allGames.filter(isActionable)) {
      if (query.length > 0 && !game.title.toLowerCase().includes(query)) {
        continue
      }
      const bucket = grouped.get(game.platform) ?? []
      bucket.push(game)
      grouped.set(game.platform, bucket)
    }

    return Array.from(grouped.entries())
      .map(([platform, games]) => ({
        platform,
        label: platformLabel(platform),
        games: [...games].sort((a, b) => a.title.localeCompare(b.title)),
      }))
      .sort((a, b) => b.games.length - a.games.length || a.label.localeCompare(b.label))
  }, [allGames, sidebarQuery])

  const openGame = (game: GameDetailResponse) => {
    navigate(`/game/${encodeURIComponent(game.id)}`, {
      state: buildGameRouteState(location.pathname, location.search),
    })
  }

  const openBrowserPlayer = (gameId: string) => {
    navigate(`/game/${encodeURIComponent(gameId)}/play`, {
      state: buildGameRouteState(location.pathname, location.search),
    })
  }

  const recentPlayedEntries = useMemo(() => {
    return recentPlayed
      .map((entry) => {
        const game = allGames.find((candidate) => candidate.id === entry.gameId)
        const launchUrl =
          entry.launchKind === 'xcloud'
            ? game?.xcloud_url ?? entry.launchUrl
            : entry.launchUrl
        if (!launchUrl) return null
        return {
          ...entry,
          game,
          launchUrl,
        }
      })
      .filter((entry): entry is NonNullable<typeof entry> => entry !== null)
      .slice(0, 4)
  }, [allGames, recentPlayed])

  useEffect(() => {
    let cancelled = false
    ;(async () => {
      try {
        const remote = await getFrontendConfig()
        if (cancelled) return

        const prefs = extractSidebarPrefs(remote.desktopSidebarPrefs) ?? readLocalSidebarPrefs()
        setSidebarPrefsState(prefs)
        writeLocalSidebarPrefs(prefs)
      } catch {
        /* keep local values */
      }
    })()

    return () => {
      cancelled = true
    }
  }, [])

  const setSidebarPrefs = (patch: Partial<DesktopSidebarPrefs>) => {
    setSidebarPrefsState((prev) => {
      const next = { ...prev, ...patch }
      writeLocalSidebarPrefs(next)
      return next
    })

    void (async () => {
      try {
        const remote = await getFrontendConfig()
        const current = extractSidebarPrefs(remote.desktopSidebarPrefs) ?? DEFAULT_SIDEBAR_PREFS
        const next: FrontendConfig = {
          ...remote,
          desktopSidebarPrefs: { ...current, ...patch },
        }
        await setFrontendConfig(next)
      } catch {
        /* local-only fallback */
      }
    })()
  }

  const setPlatformCollapsed = (platform: string, collapsed: boolean) => {
    setSidebarPrefs({
      collapsedPlatforms: {
        ...sidebarPrefs.collapsedPlatforms,
        [platform]: collapsed,
      },
    })
  }

  return (
    <aside className="hidden lg:block">
      <div className="sticky top-[7.75rem] space-y-5 rounded-[1.25rem] border border-mga-border bg-mga-surface p-4 shadow-lg shadow-black/10">
        <section className="space-y-3">
          {recentPlayedEntries.length > 0 && (
            <div className="space-y-2 rounded-mga border border-mga-border bg-mga-bg/65 p-3">
              <div className="flex items-center gap-2 text-mga-accent">
                <PlayCircle size={16} />
                <span className="text-sm font-semibold uppercase tracking-wide">Recent Played</span>
              </div>
              <div className="space-y-2">
                {recentPlayedEntries.map((entry) => {
                  const title = entry.game?.title ?? entry.title
                  const coverUrl = selectCoverUrl(entry.game?.media) ?? entry.coverUrl ?? null
                  const primarySource = entry.game ? primarySourcePlugin(entry.game) : null
                  const hint = entry.game
                    ? [platformLabel(entry.game.platform), primarySource ? sourceLabel(primarySource) : null]
                        .filter(Boolean)
                        .join(' • ')
                    : platformLabel(entry.platform)

                  const content = (
                    <>
                      <div className="h-12 w-8 shrink-0 overflow-hidden rounded-sm border border-mga-border bg-mga-surface">
                        <CoverImage
                          src={coverUrl}
                          alt={title}
                          fit="contain"
                          variant="compact"
                          className="h-full w-full text-sm"
                        />
                      </div>
                      <div className="min-w-0 flex-1">
                        <p className="line-clamp-1 text-sm font-medium text-mga-text">{title}</p>
                        <p className="line-clamp-1 text-xs text-mga-muted">{hint}</p>
                      </div>
                      <span className="shrink-0 text-xs font-medium text-mga-accent">Resume</span>
                    </>
                  )

                  return entry.launchKind === 'browser' ? (
                    <button
                      key={entry.gameId}
                      type="button"
                      onClick={() => openBrowserPlayer(entry.gameId)}
                      className="flex w-full items-center gap-3 rounded-mga border border-mga-border bg-mga-surface/80 px-2 py-2 text-left transition-colors hover:bg-mga-elevated/70"
                    >
                      {content}
                    </button>
                  ) : (
                    <a
                      key={entry.gameId}
                      href={entry.launchUrl}
                      target="_blank"
                      rel="noreferrer"
                      className="flex items-center gap-3 rounded-mga border border-mga-border bg-mga-surface/80 px-2 py-2 text-left transition-colors hover:bg-mga-elevated/70"
                    >
                      {content}
                    </a>
                  )
                })}
              </div>
            </div>
          )}

          <div className="flex items-center gap-2 text-mga-accent">
            <PlayCircle size={16} />
            <span className="text-sm font-semibold uppercase tracking-wide">Playable Games</span>
          </div>

          <input
            type="search"
            value={sidebarQuery}
            onChange={(event) => setSidebarQuery(event.target.value)}
            placeholder="Filter playable games..."
            className="w-full rounded-mga border border-mga-border bg-mga-bg px-3 py-2 text-sm text-mga-text placeholder:text-mga-muted focus:outline-none focus:ring-2 focus:ring-mga-accent"
            aria-label="Filter playable games"
          />

          {playableGroups.length > 0 ? (
              <div className="space-y-2">
                {playableGroups.map((group) => {
                  const isCollapsed = sidebarPrefs.collapsedPlatforms[group.platform] ?? true

                  return (
                    <div key={group.platform} className="rounded-mga border border-mga-border bg-mga-bg/70">
                      <button
                        type="button"
                        onClick={() => setPlatformCollapsed(group.platform, !isCollapsed)}
                        className="flex w-full items-center justify-between gap-2 px-3 py-2 text-left"
                      >
                        <div className="flex min-w-0 items-center gap-2">
                          {isCollapsed ? (
                            <ChevronRight size={14} className="shrink-0 text-mga-muted" />
                          ) : (
                            <ChevronDown size={14} className="shrink-0 text-mga-muted" />
                          )}
                          <PlatformIcon platform={group.platform} showLabel />
                        </div>
                        <span className="text-xs text-mga-muted">{group.games.length}</span>
                      </button>

                      {!isCollapsed && (
                        <div className="space-y-1 border-t border-mga-border px-2 py-2">
                          {group.games.map((game) => {
                            const primarySource = primarySourcePlugin(game)
                            const hintParts = [
                              primarySource ? sourceLabel(primarySource) : null,
                              statusHint(game),
                            ].filter(Boolean)

                            return (
                              <button
                                key={game.id}
                                type="button"
                                onClick={() => openGame(game)}
                                className="flex w-full items-center gap-3 rounded-mga px-2 py-2 text-left transition-colors hover:bg-mga-elevated/70"
                              >
                                <div className="h-14 w-10 shrink-0 overflow-hidden rounded-sm border border-mga-border bg-mga-surface">
                                  <CoverImage
                                    src={selectCoverUrl(game.media)}
                                    alt={game.title}
                                    className="h-full w-full text-sm"
                                  />
                                </div>
                                <div className="min-w-0 flex-1">
                                  <p className="line-clamp-2 text-sm font-medium text-mga-text">
                                    {game.title}
                                  </p>
                                  {hintParts.length > 0 && (
                                    <p className="line-clamp-1 text-xs text-mga-muted">
                                      {hintParts.join(' • ')}
                                    </p>
                                  )}
                                </div>
                              </button>
                            )
                          })}
                        </div>
                      )}
                    </div>
                  )
                })}
              </div>
            ) : (
              <div className="rounded-mga border border-dashed border-mga-border bg-mga-bg/50 px-3 py-4 text-sm text-mga-muted">
                {sidebarQuery.trim().length > 0
                  ? 'No playable games match that filter.'
                  : 'No playable games are available yet.'}
              </div>
          )}
        </section>
      </div>
    </aside>
  )
}
