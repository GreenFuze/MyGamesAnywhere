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
    ]

    return () => {
      for (const unsub of unsubs) unsub()
    }
  }, [queryClient, subscribe])

  return null
}
