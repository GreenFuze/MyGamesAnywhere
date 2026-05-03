import { useMemo } from 'react'
import { useInfiniteQuery } from '@tanstack/react-query'
import { listGames } from '@/api/client'

export const LIBRARY_PAGE_SIZE = 100

/**
 * Fetches library games in bounded pages. Fetch-all is too expensive for
 * larger profile libraries and makes upgraded installs look empty when
 * navigation cancels the long request.
 */
export function useLibraryData() {
  const query = useInfiniteQuery({
    queryKey: ['games', 'paged', LIBRARY_PAGE_SIZE],
    initialPageParam: 0,
    queryFn: ({ pageParam }) => listGames({ page: pageParam, page_size: LIBRARY_PAGE_SIZE }),
    getNextPageParam: (lastPage) => {
      const loaded = (lastPage.page + 1) * lastPage.page_size
      return loaded < lastPage.total ? lastPage.page + 1 : undefined
    },
    staleTime: 30_000,
  })

  const games = useMemo(() => query.data?.pages.flatMap((page) => page.games) ?? [], [query.data])
  const totalCount = query.data?.pages[0]?.total ?? 0

  return {
    ...query,
    data: games,
    totalCount,
    loadedCount: games.length,
    pageSize: LIBRARY_PAGE_SIZE,
  }
}
