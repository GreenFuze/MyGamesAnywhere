import { useQuery } from '@tanstack/react-query'
import { getHealth, listGames } from '@/api/client'

export function HomePage() {
  const health = useQuery({ queryKey: ['health'], queryFn: getHealth })
  const games = useQuery({ queryKey: ['games'], queryFn: listGames })

  return (
    <div className="space-y-8">
      <div className="space-y-4">
        <div className="overflow-hidden rounded-mga border border-mga-border bg-mga-surface shadow-sm shadow-black/20">
          <img
            src="/title.png"
            alt="MyGamesAnywhere"
            width={1200}
            height={400}
            className="block h-auto w-full object-contain object-center"
          />
        </div>
        <p className="mx-auto max-w-2xl text-center text-sm leading-relaxed text-mga-muted">
          The shelf that follows your library. Play anywhere. Know what you have.
        </p>
        <p className="text-center text-xs text-mga-muted/80">
          Phase 1 scaffold — library UI comes in Phase 2.
        </p>
      </div>

      <section className="rounded-mga border border-mga-border bg-mga-surface p-4">
        <h2 className="text-sm font-medium text-mga-muted">Server</h2>
        <p className="mt-2 font-mono text-sm">
          {health.isPending && 'Checking /health…'}
          {health.isError && <span className="text-red-400">Error: {health.error.message}</span>}
          {health.isSuccess && <span className="text-mga-accent">{health.data}</span>}
        </p>
      </section>

      <section className="rounded-mga border border-mga-border bg-mga-surface p-4">
        <h2 className="text-sm font-medium text-mga-muted">Library</h2>
        <p className="mt-2 font-mono text-sm">
          {games.isPending && 'Loading /api/games…'}
          {games.isError && <span className="text-red-400">Error: {games.error.message}</span>}
          {games.isSuccess && (
            <span>
              <strong className="text-mga-text">{games.data.games.length}</strong> canonical games
            </span>
          )}
        </p>
      </section>
    </div>
  )
}
