import { useEffect } from 'react'
import { useQueryClient } from '@tanstack/react-query'
import { useSSE } from '@/hooks/useSSE'

export function AppQueryInvalidation() {
  const queryClient = useQueryClient()
  const { subscribe } = useSSE()

  useEffect(() => {
    return subscribe('scan_complete', () => {
      queryClient.invalidateQueries({ queryKey: ['games'] })
      queryClient.invalidateQueries({ queryKey: ['stats'] })
      queryClient.invalidateQueries({ queryKey: ['integration-games'] })
      queryClient.invalidateQueries({ queryKey: ['scan-reports'] })
    })
  }, [queryClient, subscribe])

  return null
}
