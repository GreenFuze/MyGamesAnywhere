import { NavLink, Outlet, useLocation } from 'react-router-dom'
import { THEME_IDS, THEME_LABELS, type ThemeId } from '@/theme/presets'
import { useTheme } from '@/theme/ThemeProvider'
import { useSearch } from '@/hooks/useSearchContext'
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
  const loc = useLocation()

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
        </div>
        <nav className="overflow-x-auto border-t border-mga-border/70 px-3 md:px-4">
          <div className="flex min-w-max gap-1 py-2">
            {nav.map(({ to, label }) => (
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
