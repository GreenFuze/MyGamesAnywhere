import { useQuery } from '@tanstack/react-query'
import { getIntegrationGames, getIntegrationEnrichedGames } from '@/api/client'
import { Badge } from '@/components/ui/badge'
import { Loader2 } from 'lucide-react'

interface IntegrationGamesListProps {
  integrationId: string
  type: 'source' | 'metadata'
  expanded: boolean
}

export function IntegrationGamesList({ integrationId, type, expanded }: IntegrationGamesListProps) {
  const fetchFn = type === 'source' ? getIntegrationGames : getIntegrationEnrichedGames

  const { data: games, isLoading, error } = useQuery({
    queryKey: ['integration-games', integrationId, type],
    queryFn: () => fetchFn(integrationId),
    enabled: expanded,
    staleTime: 60_000,
  })

  if (!expanded) return null

  // Loading state.
  if (isLoading) {
    return (
      <div className="flex items-center justify-center py-4">
        <Loader2 size={16} className="animate-spin text-mga-muted" />
        <span className="text-xs text-mga-muted ml-2">Loading games...</span>
      </div>
    )
  }

  // Error state.
  if (error) {
    return (
      <p className="text-xs text-red-400 py-2">
        Failed to load games: {error instanceof Error ? error.message : 'Unknown error'}
      </p>
    )
  }

  // Empty state.
  if (!games || games.length === 0) {
    return (
      <p className="text-xs text-mga-muted py-2 text-center">No games found</p>
    )
  }

  return (
    <div className="space-y-1">
      {/* Scrollable games list */}
      <div className="max-h-[300px] overflow-y-auto space-y-0.5 border border-mga-border/50 rounded-mga bg-mga-bg/50 p-2">
        {games.map((game) => (
          <div
            key={game.id}
            className="flex items-center justify-between py-1 px-1.5 rounded text-xs hover:bg-mga-elevated/50"
          >
            <span className="text-mga-text truncate flex-1 mr-2">{game.title}</span>
            {game.platform && (
              <Badge variant="muted" className="text-[10px] shrink-0">
                {game.platform}
              </Badge>
            )}
          </div>
        ))}
      </div>

      {/* Footer with count + link */}
      <div className="flex items-center justify-between px-1">
        <span className="text-[10px] text-mga-muted">{games.length} games</span>
        <a
          href="/library"
          className="text-[10px] text-mga-accent hover:underline"
        >
          View in Library
        </a>
      </div>
    </div>
  )
}
