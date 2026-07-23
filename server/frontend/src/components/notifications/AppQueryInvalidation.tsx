import { useEffect } from 'react'
import { useQueryClient } from '@tanstack/react-query'
import { useSSE } from '@/hooks/useSSE'

export function AppQueryInvalidation() {
  const queryClient = useQueryClient()
  const { subscribe } = useSSE()

  useEffect(() => {
    const refreshLibrarySlices = () => {
      queryClient.invalidateQueries({ queryKey: ['games'] })
      queryClient.invalidateQueries({ queryKey: ['stats'] })
      queryClient.invalidateQueries({ queryKey: ['integration-games'] })
    }
    const refreshAchievements = () => {
      queryClient.invalidateQueries({ queryKey: ['achievements-dashboard'] })
      queryClient.invalidateQueries({ queryKey: ['achievements-explorer'] })
      queryClient.invalidateQueries({ queryKey: ['stats'] })
      queryClient.invalidateQueries({ queryKey: ['games'] })
    }

    const unsubs = [
      subscribe('scan_integration_complete', refreshLibrarySlices),
      subscribe('scan_complete', () => {
        refreshLibrarySlices()
        queryClient.invalidateQueries({ queryKey: ['scan-reports'] })
      }),
      subscribe('scan_error', () => {
        queryClient.invalidateQueries({ queryKey: ['scan-reports'] })
      }),
      subscribe('save_sync_migration_completed', () => {
        queryClient.invalidateQueries({ queryKey: ['frontend-config'] })
      }),
      subscribe('achievement_refresh_completed', refreshAchievements),
      subscribe('achievement_refresh_failed', refreshAchievements),
      subscribe('achievement_refresh_warning', refreshAchievements),
      subscribe('installation_validation_finished', () => {
        queryClient.invalidateQueries({ queryKey: ['devices'] })
        queryClient.invalidateQueries({ queryKey: ['installation-validation-schedule'] })
        queryClient.invalidateQueries({ queryKey: ['installed-games'] })
        queryClient.invalidateQueries({ queryKey: ['game-detail'] })
      }),
      subscribe('update_available', () => {
        queryClient.invalidateQueries({ queryKey: ['update-status'] })
      }),
    ]

    return () => {
      for (const unsub of unsubs) unsub()
    }
  }, [queryClient, subscribe])

  return null
}
