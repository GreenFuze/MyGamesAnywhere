import { QueryClient, useMutation, useQueryClient, type InfiniteData } from '@tanstack/react-query'
import {
  clearGameFavorite,
  setGameFavorite,
  type GameDetailResponse,
  type ListGamesResponse,
} from '@/api/client'

function updateGameListEntry(list: ListGamesResponse | undefined, updated: GameDetailResponse): ListGamesResponse | undefined {
  if (!list) return list
  let changed = false
  const games = list.games.map((game) => {
    if (game.id !== updated.id) return game
    changed = true
    return { ...game, favorite: updated.favorite }
  })
  return changed ? { ...list, games } : list
}

export function applyUpdatedGameToCaches(
  queryClient: QueryClient,
  updated: GameDetailResponse,
) {
  queryClient.setQueryData(['game', updated.id], updated)
  queryClient.setQueriesData<InfiniteData<ListGamesResponse>>(
    { queryKey: ['games', 'paged'] },
    (current) => current
      ? { ...current, pages: current.pages.map((page) => updateGameListEntry(page, updated) ?? page) }
      : current,
  )
}

export function useGameFavoriteAction() {
  const queryClient = useQueryClient()

  const mutation = useMutation({
    mutationFn: async ({
      gameId,
      favorite,
    }: {
      gameId: string
      favorite: boolean
    }) => (favorite ? setGameFavorite(gameId) : clearGameFavorite(gameId)),
    onSuccess: (updated) => {
      applyUpdatedGameToCaches(queryClient, updated)
    },
  })

  return {
    setFavorite: mutation.mutateAsync,
    isPendingFor: (gameId: string) =>
      mutation.isPending && mutation.variables?.gameId === gameId,
  }
}
