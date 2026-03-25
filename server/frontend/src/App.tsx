import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import { BrowserRouter, Navigate, Route, Routes } from 'react-router-dom'
import { ThemeProvider } from '@/theme/ThemeProvider'
import { SearchProvider } from '@/hooks/useSearchContext'
import { ErrorBoundary } from '@/components/ErrorBoundary'
import { AppLayout } from '@/layouts/AppLayout'
import { HomePage } from '@/pages/HomePage'
import { AboutPage } from '@/pages/AboutPage'
import { LibraryPage } from '@/pages/LibraryPage'
import { PlayablePage } from '@/pages/PlayablePage'
import { XCloudPage } from '@/pages/XCloudPage'
import { SettingsPage } from '@/pages/SettingsPage'

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
      <ThemeProvider>
        <SearchProvider>
          <BrowserRouter>
            <ErrorBoundary>
              <Routes>
                <Route path="/" element={<AppLayout />}>
                  <Route index element={<HomePage />} />
                  <Route path="library" element={<LibraryPage />} />
                  <Route path="playable" element={<PlayablePage />} />
                  <Route path="xcloud" element={<XCloudPage />} />
                  <Route path="settings" element={<SettingsPage />} />
                  <Route path="about" element={<AboutPage />} />
                  <Route path="*" element={<Navigate to="/" replace />} />
                </Route>
              </Routes>
            </ErrorBoundary>
          </BrowserRouter>
        </SearchProvider>
      </ThemeProvider>
    </QueryClientProvider>
  )
}
