import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import { BrowserRouter, Navigate, Route, Routes } from 'react-router-dom'
import { ThemeProvider } from '@/theme/ThemeProvider'
import { SearchProvider } from '@/hooks/useSearchContext'
import { SSEProvider } from '@/hooks/useSSE'
import { ErrorBoundary } from '@/components/ErrorBoundary'
import { AppNotifications } from '@/components/notifications/AppNotifications'
import { AppQueryInvalidation } from '@/components/notifications/AppQueryInvalidation'
import { ToastProvider } from '@/components/ui/toast'
import { AppLayout } from '@/layouts/AppLayout'
import { HomePage } from '@/pages/HomePage'
import { AboutPage } from '@/pages/AboutPage'
import { AchievementsPage } from '@/pages/AchievementsPage'
import { LibraryPage } from '@/pages/LibraryPage'
import { PlayPage } from '@/pages/PlayPage'
import { SettingsPage } from '@/pages/SettingsPage'
import { GameDetailPage } from '@/pages/GameDetailPage'
import { GamePlayerPage } from '@/pages/GamePlayerPage'

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
        <ToastProvider>
          <ThemeProvider>
            <SearchProvider>
              <BrowserRouter>
                <AppNotifications />
                <AppQueryInvalidation />
                <ErrorBoundary>
                  <Routes>
                    <Route path="/" element={<AppLayout />}>
                      <Route index element={<HomePage />} />
                      <Route path="play" element={<PlayPage />} />
                      <Route path="play/section/:sectionId" element={<PlayPage />} />
                      <Route path="library" element={<LibraryPage />} />
                      <Route path="library/section/:sectionId" element={<LibraryPage />} />
                      <Route path="achievements" element={<AchievementsPage />} />
                      <Route path="playable" element={<Navigate to="/play" replace />} />
                      <Route path="xcloud" element={<Navigate to="/play" replace />} />
                      <Route path="settings" element={<SettingsPage />} />
                      <Route path="about" element={<AboutPage />} />
                    </Route>
                    <Route path="/game/:id/play" element={<GamePlayerPage />} />
                    <Route path="/game/:id" element={<GameDetailPage />} />
                    <Route path="*" element={<Navigate to="/" replace />} />
                  </Routes>
                </ErrorBoundary>
              </BrowserRouter>
            </SearchProvider>
          </ThemeProvider>
        </ToastProvider>
      </SSEProvider>
    </QueryClientProvider>
  )
}
