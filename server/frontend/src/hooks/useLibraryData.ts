import { useQuery } from '@tanstack/react-query'
import { listGames } from '@/api/client'

/**
 * Fetches all games in a single request (page_size=0).
 * TanStack Query caches the result with 30s stale time.
 */
export function useLibraryData() {
  return useQuery({
    queryKey: ['games', 'all'],
    queryFn: () => listGames({ page: 0, page_size: 0 }),
    staleTime: 30_000,
    select: (data) => data.games,
  })
}
