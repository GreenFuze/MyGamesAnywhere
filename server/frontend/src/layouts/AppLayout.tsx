import { useEffect, useRef } from 'react'
import { NavLink, Outlet, useLocation } from 'react-router-dom'
import { THEME_IDS, THEME_LABELS, type ThemeId } from '@/theme/presets'
import { useTheme } from '@/theme/ThemeProvider'
import { cn } from '@/lib/utils'

const nav = [
  { to: '/', label: 'Home' },
  { to: '/library', label: 'Library' },
  { to: '/playable', label: 'Playable' },
  { to: '/settings', label: 'Settings' },
  { to: '/about', label: 'About' },
]

export function AppLayout() {
  const { themeId, setThemeId } = useTheme()
  const searchRef = useRef<HTMLInputElement>(null)
  const loc = useLocation()

  useEffect(() => {
    const onKey = (e: KeyboardEvent) => {
      if ((e.ctrlKey || e.metaKey) && e.key === 'k') {
        e.preventDefault()
        searchRef.current?.focus()
      }
    }
    window.addEventListener('keydown', onKey)
    return () => window.removeEventListener('keydown', onKey)
  }, [])

  return (
    <div className="flex min-h-screen bg-mga-bg font-mga text-mga-text">
      <aside className="hidden w-52 shrink-0 border-r border-mga-border bg-mga-surface md:block">
        <NavLink
          to="/"
          end
          className={({ isActive }) =>
            cn(
              'flex h-14 items-center gap-2 border-b border-mga-border px-3 transition-colors focus:outline-none focus-visible:ring-2 focus-visible:ring-mga-accent',
              isActive ? 'bg-mga-elevated/60' : 'hover:bg-mga-elevated/40',
            )
          }
          aria-label="Home"
          title="Home"
        >
          <img src="/logo.png" alt="" width={32} height={32} className="h-8 w-8 shrink-0 object-contain" />
          <span className="truncate text-sm font-semibold tracking-tight">MGA</span>
        </NavLink>
        <nav className="flex flex-col gap-0.5 p-2">
          {nav.map(({ to, label }) => (
            <NavLink
              key={to}
              to={to}
              end={to === '/'}
              className={({ isActive }) =>
                cn(
                  'rounded-mga px-3 py-2 text-sm transition-colors',
                  isActive
                    ? 'bg-mga-elevated text-mga-accent'
                    : 'text-mga-muted hover:bg-mga-elevated hover:text-mga-text',
                )
              }
            >
              {label}
            </NavLink>
          ))}
        </nav>
      </aside>

      <div className="flex min-w-0 flex-1 flex-col">
        <header className="flex h-14 shrink-0 items-center gap-3 border-b border-mga-border bg-mga-surface px-3 md:px-4">
          <NavLink
            to="/"
            end
            className={({ isActive }) =>
              cn(
                'shrink-0 rounded-mga p-1 transition-colors focus:outline-none focus-visible:ring-2 focus-visible:ring-mga-accent md:hidden',
                isActive ? 'bg-mga-elevated' : 'hover:bg-mga-elevated',
              )
            }
            aria-label="Home"
            title="Home"
          >
            <img src="/logo.png" alt="" width={32} height={32} className="h-8 w-8 object-contain" />
          </NavLink>
          <div className="min-w-0 flex-1">
            <input
              ref={searchRef}
              type="search"
              placeholder="Search… (Ctrl+K)"
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
          <button
            type="button"
            className="rounded-mga p-2 text-mga-muted hover:bg-mga-elevated hover:text-mga-text"
            title="Notifications (coming soon)"
            aria-label="Notifications"
          >
            🔔
          </button>
        </header>

        <main className="flex-1 overflow-auto p-4 md:p-6">
          <div key={loc.pathname} className="mx-auto max-w-5xl">
            <Outlet />
          </div>
        </main>
      </div>
    </div>
  )
}
