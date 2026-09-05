import { useMemo } from 'react'
import { useIsFetching, useQueryClient } from '@tanstack/react-query'
import { NavLink, Outlet, useLocation } from 'react-router'
import {
  Boxes,
  ChartNoAxesCombined,
  CircleUserRound,
  Database,
  Library,
  PackageCheck,
  PlugZap,
  RefreshCw,
  ServerCog,
  Trophy,
  UsersRound,
} from 'lucide-react'
import { Button } from '@/components/ui/button'
import { profileAvatarFor, useProfiles } from '@/hooks/useProfiles'
import { MANAGEMENT_DESTINATIONS } from '@/lib/navigationRoutes'
import { cn } from '@/lib/utils'

const icons = {
  overview: ChartNoAxesCombined,
  profiles: UsersRound,
  library: Library,
  catalog: Database,
  sources: PlugZap,
  artifacts: PackageCheck,
  achievements: Trophy,
  system: ServerCog,
} as const

export function ManagementShell() {
  const { currentProfile, clearProfile } = useProfiles()
  const queryClient = useQueryClient()
  const managementQueriesFetching = useIsFetching({ queryKey: ['management'] }) > 0
  const location = useLocation()

  const active = useMemo(() => MANAGEMENT_DESTINATIONS.find((item) => location.pathname === item.path || location.pathname.startsWith(`${item.path}/`)) ?? MANAGEMENT_DESTINATIONS[0], [location.pathname])
  const AvatarIcon = profileAvatarFor(currentProfile?.avatar_key).Icon

  return (
    <div className="min-h-screen bg-[radial-gradient(circle_at_top_right,rgba(61,184,255,0.08),transparent_34rem),linear-gradient(180deg,var(--mga-bg),color-mix(in_srgb,var(--mga-bg)_92%,black))] text-mga-text">
      <a href="#management-content" className="fixed left-3 top-3 z-50 -translate-y-20 rounded-md bg-mga-accent px-3 py-2 text-sm font-semibold text-black transition-transform focus:translate-y-0">Skip to content</a>
      <aside className="fixed inset-y-0 left-0 z-30 hidden w-64 border-r border-mga-border/80 bg-mga-surface/90 backdrop-blur-xl lg:flex lg:flex-col">
        <Brand />
        <Navigation className="flex-1 px-3 py-5" />
      </aside>

      <div className="lg:pl-64">
        <header className="sticky top-0 z-20 border-b border-mga-border/80 bg-mga-bg/88 backdrop-blur-xl">
          <div className="flex min-h-[4.6rem] items-center justify-between gap-4 px-4 sm:px-6 lg:px-8">
            <div className="min-w-0">
              <div className="lg:hidden"><Brand compact /></div>
              <div className="hidden lg:block">
                <p className="truncate text-sm font-semibold text-mga-text">{active.label}</p>
                <p className="mt-0.5 truncate text-xs text-mga-muted">{active.description}</p>
              </div>
            </div>
            <div className="flex items-center gap-2 sm:gap-3">
              <Button variant="ghost" size="icon" aria-label="Refresh management data" onClick={() => void queryClient.invalidateQueries()} disabled={managementQueriesFetching}>
                <RefreshCw className={cn('h-4 w-4', managementQueriesFetching && 'animate-spin')} />
              </Button>
              <button type="button" onClick={() => void clearProfile()} className="group flex min-h-11 items-center gap-2 rounded-lg border border-mga-border bg-mga-surface px-2.5 py-1.5 text-left transition hover:border-mga-accent/45 focus:outline-none focus:ring-2 focus:ring-mga-accent/50" aria-label={`Switch profile. Current profile ${currentProfile?.display_name}`}>
                <span className="grid h-8 w-8 place-items-center rounded-md bg-gradient-to-br from-mga-accent/25 to-violet-400/15 text-mga-accent"><AvatarIcon className="h-4 w-4" /></span>
                <span className="hidden min-w-0 sm:block">
                  <span className="block max-w-32 truncate text-xs font-semibold text-mga-text">{currentProfile?.display_name}</span>
                  <span className="block text-[0.65rem] uppercase tracking-wider text-mga-muted">{currentProfile?.role === 'admin_player' ? 'Administrator' : 'Profile'}</span>
                </span>
                <CircleUserRound className="h-4 w-4 text-mga-muted group-hover:text-mga-accent" />
              </button>
            </div>
          </div>
          <Navigation mobile className="lg:hidden" />
        </header>

        <main id="management-content" className="mx-auto w-full max-w-[1600px] px-4 py-7 sm:px-6 lg:px-8 lg:py-9" tabIndex={-1}>
          <Outlet />
        </main>
      </div>
    </div>
  )
}

function Brand({ compact = false }: { compact?: boolean }) {
  return (
    <div className={cn('flex items-center gap-3', compact ? 'py-2' : 'border-b border-mga-border/70 px-5 py-5')}>
      <span className="relative grid h-10 w-10 place-items-center overflow-hidden rounded-xl border border-mga-accent/30 bg-mga-accent/10 text-mga-accent shadow-[0_0_28px_rgba(61,184,255,0.12)]">
        <Boxes className="h-5 w-5" />
      </span>
      <span>
        <span className="block text-sm font-bold tracking-[0.18em] text-mga-text">MGA</span>
        <span className="block text-[0.62rem] uppercase tracking-[0.2em] text-mga-muted">My Games Anywhere</span>
      </span>
    </div>
  )
}

function Navigation({ mobile = false, className }: { mobile?: boolean; className?: string }) {
  return (
    <nav aria-label="Management console" className={cn(className, mobile && 'mga-hidden-scrollbar flex gap-1 overflow-x-auto border-t border-mga-border/60 px-3 py-2')}>
      <div className={cn(!mobile && 'space-y-1')}>
        {MANAGEMENT_DESTINATIONS.map((item) => {
          const Icon = icons[item.id]
          return (
            <NavLink key={item.id} to={item.path} className={({ isActive }) => cn(
              'group flex min-h-11 items-center gap-3 rounded-lg border border-transparent text-sm transition focus:outline-none focus:ring-2 focus:ring-mga-accent/45',
              mobile ? 'shrink-0 px-3' : 'w-full px-3',
              isActive ? 'border-mga-accent/20 bg-mga-accent/10 text-mga-text shadow-[inset_3px_0_0_var(--mga-accent)]' : 'text-mga-muted hover:bg-mga-elevated/65 hover:text-mga-text',
            )}>
              <Icon className="h-[1.05rem] w-[1.05rem] shrink-0" />
              <span className={cn('font-medium', mobile && 'text-xs')}>{item.label}</span>
            </NavLink>
          )
        })}
      </div>
    </nav>
  )
}
