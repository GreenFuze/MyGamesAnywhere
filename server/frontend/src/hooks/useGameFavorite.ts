import { QueryClient, useMutation, useQueryClient } from '@tanstack/react-query'
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
    return updated
  })
  return changed ? { ...list, games } : list
}

export function applyUpdatedGameToCaches(
  queryClient: QueryClient,
  updated: GameDetailResponse,
) {
  queryClient.setQueryData(['game', updated.id], updated)
  queryClient.setQueryData<ListGamesResponse | undefined>(['games', 'all'], (current) =>
    updateGameListEntry(current, updated),
  )
  void queryClient.invalidateQueries({ queryKey: ['games'] })
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
