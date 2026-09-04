import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import { lazy, Suspense, type ReactNode } from 'react'
import { BrowserRouter, Navigate, Route, Routes, useLocation } from 'react-router'
import { ThemeProvider } from '@/theme/ThemeProvider'
import { ProfileProvider, useProfiles } from '@/hooks/useProfiles'
import { SSEProvider } from '@/hooks/useSSE'
import { ErrorBoundary } from '@/components/ErrorBoundary'
import { AppNotifications } from '@/components/notifications/AppNotifications'
import { AppQueryInvalidation } from '@/components/notifications/AppQueryInvalidation'
import { ToastProvider } from '@/components/ui/toast'
import { ManagementShell } from '@/layouts/ManagementShell'
import {
  APP_DESTINATIONS,
  APP_ROUTES,
  isCredentialSetupPath,
} from '@/lib/navigationRoutes'

const CredentialSetupPage = lazy(() => import('@/pages/CredentialSetupPage').then((module) => ({ default: module.CredentialSetupPage })))
const OverviewPage = lazy(() => import('@/pages/management/OverviewPage').then((module) => ({ default: module.OverviewPage })))
const ProfilesPage = lazy(() => import('@/pages/management/ProfilesPage').then((module) => ({ default: module.ProfilesPage })))
const LibraryManagementPage = lazy(() => import('@/pages/management/LibraryManagementPage').then((module) => ({ default: module.LibraryManagementPage })))
const CatalogPage = lazy(() => import('@/pages/management/CatalogPage').then((module) => ({ default: module.CatalogPage })))
const SourcesPage = lazy(() => import('@/pages/management/SourcesPage').then((module) => ({ default: module.SourcesPage })))
const ArtifactsPage = lazy(() => import('@/pages/management/ArtifactsPage').then((module) => ({ default: module.ArtifactsPage })))
const AchievementsManagementPage = lazy(() => import('@/pages/management/AchievementsManagementPage').then((module) => ({ default: module.AchievementsManagementPage })))
const SystemPage = lazy(() => import('@/pages/management/SystemPage').then((module) => ({ default: module.SystemPage })))

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
          <BrowserRouter>
            <ErrorBoundary>
              <ProfileAwareRoutes />
            </ErrorBoundary>
          </BrowserRouter>
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
          <Route path={APP_ROUTES.root} element={<ManagementShell />}>
            <Route index element={<Navigate to={APP_DESTINATIONS.overview} replace />} />
            <Route path={APP_ROUTES.overview} element={<OverviewPage />} />
            <Route path={APP_ROUTES.profiles} element={<ProfilesPage />} />
            <Route path={APP_ROUTES.library} element={<LibraryManagementPage />} />
            <Route path={APP_ROUTES.catalog} element={<CatalogPage />} />
            <Route path={APP_ROUTES.sources} element={<SourcesPage />} />
            <Route path={APP_ROUTES.artifacts} element={<ArtifactsPage />} />
            <Route path={APP_ROUTES.achievements} element={<AchievementsManagementPage />} />
            <Route path={APP_ROUTES.system} element={<SystemPage />} />
            {/* Paths from the retired player shell that named a management
                surface still lead to it, so existing bookmarks do not break.
                Play, game-launch and browser-runtime paths are gone entirely
                and fall through to the catch-all below. */}
            <Route path={APP_ROUTES.librarySection} element={<Navigate to={APP_DESTINATIONS.library} replace />} />
            <Route path={APP_ROUTES.libraryReview} element={<Navigate to={APP_DESTINATIONS.library} replace />} />
            <Route path={APP_ROUTES.settings} element={<Navigate to={APP_DESTINATIONS.system} replace />} />
            <Route path={APP_ROUTES.about} element={<Navigate to={APP_DESTINATIONS.system} replace />} />
            <Route path={APP_ROUTES.stats} element={<Navigate to={APP_DESTINATIONS.overview} replace />} />
          </Route>
          <Route path={APP_ROUTES.fallback} element={<Navigate to={APP_DESTINATIONS.overview} replace />} />
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
