import { useEffect, useRef, useState } from 'react'
import { Check, LogOut, Users } from 'lucide-react'
import { NavLink, Outlet, useLocation } from 'react-router-dom'
import { THEME_IDS, THEME_LABELS, type ThemeId } from '@/theme/presets'
import { useTheme } from '@/theme/ThemeProvider'
import { useSearch } from '@/hooks/useSearchContext'
import { ProfileAvatar, useProfiles } from '@/hooks/useProfiles'
import { cn } from '@/lib/utils'

const nav = [
  { to: '/', label: 'Home' },
  { to: '/play', label: 'Play' },
  { to: '/library', label: 'Library' },
  { to: '/achievements', label: 'Achievements' },
  { to: '/settings', label: 'Settings' },
  { to: '/about', label: 'About' },
]

export function AppLayout() {
  const { themeId, setThemeId } = useTheme()
  const { searchQuery, setSearchQuery, searchRef } = useSearch()
  const { profiles, currentProfile, selectProfile, clearProfile } = useProfiles()
  const [profileMenuOpen, setProfileMenuOpen] = useState(false)
  const profileMenuRef = useRef<HTMLDivElement | null>(null)
  const loc = useLocation()

  useEffect(() => {
    if (!profileMenuOpen) return

    function onPointerDown(event: PointerEvent) {
      if (!profileMenuRef.current?.contains(event.target as Node)) {
        setProfileMenuOpen(false)
      }
    }

    function onKeyDown(event: KeyboardEvent) {
      if (event.key === 'Escape') {
        setProfileMenuOpen(false)
      }
    }

    document.addEventListener('pointerdown', onPointerDown)
    document.addEventListener('keydown', onKeyDown)
    return () => {
      document.removeEventListener('pointerdown', onPointerDown)
      document.removeEventListener('keydown', onKeyDown)
    }
  }, [profileMenuOpen])

  const isWideRoute = ['/library', '/play', '/achievements'].some((p) =>
    loc.pathname.startsWith(p),
  )

  return (
    <div className="min-h-screen bg-mga-bg font-mga text-mga-text">
      <header className="sticky top-0 z-20 border-b border-mga-border bg-mga-surface/95 backdrop-blur">
        <div className="flex flex-wrap items-center gap-3 px-3 py-3 md:px-4">
          <NavLink
            to="/"
            end
            className="flex shrink-0 items-center gap-2 rounded-mga border border-mga-border bg-mga-bg px-2 py-1.5 transition-colors hover:bg-mga-elevated focus:outline-none focus-visible:ring-2 focus-visible:ring-mga-accent"
            aria-label="Home"
            title="Home"
          >
            <img src="/logo.png" alt="" width={32} height={32} className="h-8 w-8 object-contain" />
            <span className="hidden text-sm font-semibold tracking-tight sm:inline">MGA</span>
          </NavLink>
          <div className="min-w-[14rem] flex-1">
            <input
              ref={searchRef}
              type="search"
              placeholder="Search... (Ctrl+K)"
              value={searchQuery}
              onChange={(e) => setSearchQuery(e.target.value)}
              className="w-full max-w-md rounded-mga border border-mga-border bg-mga-bg px-3 py-1.5 text-sm text-mga-text placeholder:text-mga-muted focus:outline-none focus:ring-2 focus:ring-mga-accent"
              aria-label="Search"
            />
          </div>
          <label className="flex items-center gap-2 text-xs text-mga-muted">
            <span className="hidden sm:inline">Theme</span>
            <select
              value={themeId}
              onChange={(e) => setThemeId(e.target.value as ThemeId)}
              className="rounded-mga border border-mga-border bg-mga-bg px-2 py-1 text-mga-text"
            >
              {THEME_IDS.map((id) => (
                <option key={id} value={id}>
                  {THEME_LABELS[id]}
                </option>
              ))}
            </select>
          </label>
          {currentProfile ? (
            <div ref={profileMenuRef} className="relative shrink-0">
              <button
                type="button"
                onClick={() => setProfileMenuOpen((open) => !open)}
                className="flex items-center gap-2 rounded-mga border border-mga-border bg-mga-bg px-2 py-1.5 text-left transition-colors hover:bg-mga-elevated focus:outline-none focus-visible:ring-2 focus-visible:ring-mga-accent"
                title="Profile menu"
                aria-label="Profile menu"
                aria-expanded={profileMenuOpen}
              >
                <ProfileAvatar profile={currentProfile} className="h-7 w-7 text-xs" />
                <span className="hidden min-w-0 max-w-[10rem] truncate text-sm font-medium text-mga-text sm:inline">
                  {currentProfile.display_name}
                </span>
              </button>
              {profileMenuOpen ? (
                <div className="absolute right-0 top-[calc(100%+0.5rem)] z-50 w-72 overflow-hidden rounded-mga border border-mga-border bg-mga-surface shadow-2xl">
                  <div className="border-b border-mga-border bg-mga-bg/70 px-3 py-2">
                    <div className="flex items-center gap-2 text-xs font-bold uppercase tracking-[0.18em] text-mga-muted">
                      <Users className="h-3.5 w-3.5" />
                      Switch Player
                    </div>
                  </div>
                  <div className="max-h-72 overflow-y-auto p-1.5">
                    {profiles.map((profile) => {
                      const selected = profile.id === currentProfile.id
                      return (
                        <button
                          key={profile.id}
                          type="button"
                          onClick={() => {
                            if (!selected) {
                              selectProfile(profile.id)
                            }
                            setProfileMenuOpen(false)
                          }}
                          className={cn(
                            'flex w-full items-center gap-3 rounded-mga px-2 py-2 text-left transition-colors focus:outline-none focus-visible:ring-2 focus-visible:ring-mga-accent',
                            selected ? 'bg-mga-elevated text-mga-text' : 'text-mga-muted hover:bg-mga-elevated hover:text-mga-text',
                          )}
                        >
                          <ProfileAvatar profile={profile} className="h-9 w-9 text-xs" />
                          <span className="min-w-0 flex-1">
                            <span className="block truncate text-sm font-semibold">{profile.display_name}</span>
                            <span className="block text-xs text-mga-muted">
                              {profile.role === 'admin_player' ? 'Admin Player' : 'Player'}
                            </span>
                          </span>
                          {selected ? <Check className="h-4 w-4 text-mga-accent" /> : null}
                        </button>
                      )
                    })}
                  </div>
                  <div className="border-t border-mga-border p-1.5">
                    <button
                      type="button"
                      onClick={() => {
                        setProfileMenuOpen(false)
                        clearProfile()
                      }}
                      className="flex w-full items-center gap-3 rounded-mga px-2 py-2 text-left text-sm font-semibold text-mga-muted transition-colors hover:bg-mga-elevated hover:text-mga-text focus:outline-none focus-visible:ring-2 focus-visible:ring-mga-accent"
                    >
                      <LogOut className="h-4 w-4" />
                      Log out
                    </button>
                  </div>
                </div>
              ) : null}
            </div>
          ) : null}
        </div>
        <nav className="overflow-x-auto border-t border-mga-border/70 px-3 md:px-4">
          <div className="flex min-w-max gap-1 py-2">
            {nav
              .filter(({ to }) => to !== '/settings' || currentProfile?.role === 'admin_player')
              .map(({ to, label }) => (
              <NavLink
                key={to}
                to={to}
                end={to === '/'}
                className={({ isActive }) =>
                  cn(
                    'rounded-mga px-3 py-2 text-sm font-medium transition-colors',
                    isActive
                      ? 'bg-mga-elevated text-mga-accent'
                      : 'text-mga-muted hover:bg-mga-elevated hover:text-mga-text',
                  )
                }
              >
                {label}
              </NavLink>
            ))}
          </div>
        </nav>
      </header>

      <main className="p-4 md:p-6">
        <div className="mx-auto min-w-0 max-w-[110rem]">
          <div
            key={loc.pathname}
            className={cn(
              'mga-page-enter min-w-0',
              isWideRoute ? 'w-full' : 'mx-auto w-full max-w-5xl',
            )}
          >
            <Outlet />
          </div>
        </div>
      </main>
    </div>
  )
}
