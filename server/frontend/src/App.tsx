import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import { lazy, Suspense, type ReactNode } from 'react'
import { BrowserRouter, Navigate, Route, Routes, useLocation } from 'react-router'
import { ThemeProvider } from '@/theme/ThemeProvider'
import { SearchProvider } from '@/hooks/useSearchContext'
import { ProfileProvider, useProfiles } from '@/hooks/useProfiles'
import { SSEProvider } from '@/hooks/useSSE'
import { ErrorBoundary } from '@/components/ErrorBoundary'
import { AppNotifications } from '@/components/notifications/AppNotifications'
import { AppQueryInvalidation } from '@/components/notifications/AppQueryInvalidation'
import { ToastProvider } from '@/components/ui/toast'
import { AppLayout } from '@/layouts/AppLayout'
import {
  APP_DESTINATIONS,
  APP_ROUTES,
  isCredentialSetupPath,
} from '@/lib/navigationRoutes'

const AboutPage = lazy(() => import('@/pages/AboutPage').then((module) => ({ default: module.AboutPage })))
const AchievementsPage = lazy(() => import('@/pages/AchievementsPage').then((module) => ({ default: module.AchievementsPage })))
const LibraryPage = lazy(() => import('@/pages/LibraryPage').then((module) => ({ default: module.LibraryPage })))
const LibraryReviewPage = lazy(() => import('@/pages/LibraryReviewPage').then((module) => ({ default: module.LibraryReviewPage })))
const PlayPage = lazy(() => import('@/pages/PlayPage').then((module) => ({ default: module.PlayPage })))
const StatsPage = lazy(() => import('@/pages/StatsPage').then((module) => ({ default: module.StatsPage })))
const SettingsPage = lazy(() => import('@/pages/SettingsPage').then((module) => ({ default: module.SettingsPage })))
const GameDetailPage = lazy(() => import('@/pages/GameDetailPage').then((module) => ({ default: module.GameDetailPage })))
const GameMediaPage = lazy(() => import('@/pages/GameMediaPage').then((module) => ({ default: module.GameMediaPage })))
const GamePlayerPage = lazy(() => import('@/pages/GamePlayerPage').then((module) => ({ default: module.GamePlayerPage })))
const CredentialSetupPage = lazy(() => import('@/pages/CredentialSetupPage').then((module) => ({ default: module.CredentialSetupPage })))

const queryClient = new QueryClient({
  defaultOptions: {
    queries: {
      staleTime: 30_000,
      retry: 1,
    },
  },
})

export function App() {
  return (
    <QueryClientProvider client={queryClient}>
      <SSEProvider>
        <ThemeProvider>
          <SearchProvider>
            <BrowserRouter>
              <ErrorBoundary>
                <ProfileAwareRoutes />
              </ErrorBoundary>
            </BrowserRouter>
          </SearchProvider>
        </ThemeProvider>
      </SSEProvider>
    </QueryClientProvider>
  )
}

function ProfileAwareRoutes() {
  const location = useLocation()
  if (isCredentialSetupPath(location.pathname)) {
    return <Suspense fallback={<RouteLoading />}><Routes><Route path={APP_ROUTES.credentialSetup} element={<CredentialSetupPage />} /></Routes></Suspense>
  }
  return (
    <ProfileProvider>
      <ProfileScopedToastProvider>
        <AppNotifications />
        <AppQueryInvalidation />
        <Suspense fallback={<RouteLoading />}>
        <Routes>
          <Route path={APP_ROUTES.root} element={<AppLayout />}>
            <Route index element={<Navigate to={APP_DESTINATIONS.play} replace />} />
            <Route path={APP_ROUTES.play} element={<PlayPage />} />
            <Route path={APP_ROUTES.playSection} element={<PlayPage />} />
            <Route path={APP_ROUTES.library} element={<LibraryPage />} />
            <Route path={APP_ROUTES.librarySection} element={<LibraryPage />} />
            <Route path={APP_ROUTES.libraryReview} element={<LibraryReviewPage />} />
            <Route path={APP_ROUTES.achievements} element={<AchievementsPage />} />
            <Route path={APP_ROUTES.stats} element={<Navigate to={APP_DESTINATIONS.statsLibrary} replace />} />
            <Route path={APP_ROUTES.statsLibrary} element={<StatsPage />} />
            <Route path={APP_ROUTES.statsGamer} element={<StatsPage />} />
            <Route path={APP_ROUTES.playableLegacy} element={<Navigate to={APP_DESTINATIONS.play} replace />} />
            <Route path={APP_ROUTES.xcloudLegacy} element={<Navigate to={APP_DESTINATIONS.play} replace />} />
            <Route path={APP_ROUTES.settings} element={<SettingsPage />} />
            <Route path={APP_ROUTES.about} element={<AboutPage />} />
          </Route>
          <Route path={APP_ROUTES.gamePlay} element={<GamePlayerPage />} />
          <Route path={APP_ROUTES.gameMedia} element={<GameMediaPage />} />
          <Route path={APP_ROUTES.gameDetail} element={<GameDetailPage />} />
          <Route path={APP_ROUTES.fallback} element={<Navigate to={APP_DESTINATIONS.play} replace />} />
        </Routes>
        </Suspense>
      </ProfileScopedToastProvider>
    </ProfileProvider>
  )
}

function RouteLoading() {
  return (
    <div className="flex min-h-[40vh] items-center justify-center text-sm text-mga-muted" role="status">
      Loading…
    </div>
  )
}

function ProfileScopedToastProvider({ children }: { children: ReactNode }) {
  const { currentProfile } = useProfiles()
  if (!currentProfile) {
    throw new Error('Notification history requires an active profile')
  }
  return (
    <ToastProvider key={currentProfile.id} historyScope={currentProfile.id}>
      {children}
    </ToastProvider>
  )
}
