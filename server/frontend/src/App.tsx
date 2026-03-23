import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import { BrowserRouter, Navigate, Route, Routes } from 'react-router-dom'
import { ThemeProvider } from '@/theme/ThemeProvider'
import { ErrorBoundary } from '@/components/ErrorBoundary'
import { AppLayout } from '@/layouts/AppLayout'
import { HomePage } from '@/pages/HomePage'
import { PlaceholderPage } from '@/pages/PlaceholderPage'
import { AboutPage } from '@/pages/AboutPage'

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
        <BrowserRouter>
          <ErrorBoundary>
            <Routes>
              <Route path="/" element={<AppLayout />}>
                <Route index element={<HomePage />} />
                <Route
                  path="library"
                  element={
                    <PlaceholderPage
                      title="Library"
                      body="Grid and list views are planned for Phase 2."
                    />
                  }
                />
                <Route
                  path="playable"
                  element={
                    <PlaceholderPage
                      title="Playable"
                      body="Browser-emulatable games filter — Phase 2."
                    />
                  }
                />
                <Route
                  path="settings"
                  element={
                    <PlaceholderPage
                      title="Settings"
                      body="Use the theme selector in the top bar; more settings later."
                    />
                  }
                />
                <Route path="about" element={<AboutPage />} />
                <Route path="*" element={<Navigate to="/" replace />} />
              </Route>
            </Routes>
          </ErrorBoundary>
        </BrowserRouter>
      </ThemeProvider>
    </QueryClientProvider>
  )
}
