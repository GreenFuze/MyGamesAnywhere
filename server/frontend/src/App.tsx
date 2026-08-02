import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import type { ReactNode } from 'react'
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
import { AboutPage } from '@/pages/AboutPage'
import { AchievementsPage } from '@/pages/AchievementsPage'
import { LibraryPage } from '@/pages/LibraryPage'
import { LibraryReviewPage } from '@/pages/LibraryReviewPage'
import { PlayPage } from '@/pages/PlayPage'
import { StatsPage } from '@/pages/StatsPage'
import { SettingsPage } from '@/pages/SettingsPage'
import { GameDetailPage } from '@/pages/GameDetailPage'
import { GameMediaPage } from '@/pages/GameMediaPage'
import { GamePlayerPage } from '@/pages/GamePlayerPage'
import { CredentialSetupPage } from '@/pages/CredentialSetupPage'
import {
  APP_DESTINATIONS,
  APP_ROUTES,
  isCredentialSetupPath,
} from '@/lib/navigationRoutes'

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
    return <Routes><Route path={APP_ROUTES.credentialSetup} element={<CredentialSetupPage />} /></Routes>
  }
  return (
    <ProfileProvider>
      <ProfileScopedToastProvider>
        <AppNotifications />
        <AppQueryInvalidation />
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
      </ProfileScopedToastProvider>
    </ProfileProvider>
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
