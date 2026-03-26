import { useQuery } from '@tanstack/react-query'
import { useLocation, useNavigate, useParams } from 'react-router-dom'
import { getGame } from '@/api/client'
import { Badge } from '@/components/ui/badge'
import { Button } from '@/components/ui/button'
import { CoverImage } from '@/components/ui/cover-image'
import { PlatformIcon } from '@/components/ui/platform-icon'
import {
  formatHLTB,
  isPlayable,
  pluginLabel,
  resolverMatchCount,
  selectCoverUrl,
  selectSourcePlugins,
} from '@/lib/gameUtils'

function readFromState(state: unknown): string | null {
  if (!state || typeof state !== 'object') return null
  const candidate = (state as { from?: unknown }).from
  return typeof candidate === 'string' ? candidate : null
}

export function GameDetailPage() {
  const navigate = useNavigate()
  const location = useLocation()
  const { id = '' } = useParams()
  const from = readFromState(location.state) ?? '/library'

  const game = useQuery({
    queryKey: ['game', id],
    queryFn: () => getGame(id),
    enabled: id.length > 0,
  })

  if (game.isPending) {
    return (
      <div className="mx-auto max-w-5xl space-y-4 p-4 md:p-6">
        <Button variant="outline" size="sm" onClick={() => navigate(from)}>
          ← Back
        </Button>
        <div className="rounded-mga border border-mga-border bg-mga-surface p-6">
          <p className="text-sm text-mga-muted">Loading game details...</p>
        </div>
      </div>
    )
  }

  if (game.isError || !game.data) {
    return (
      <div className="mx-auto max-w-5xl space-y-4 p-4 md:p-6">
        <Button variant="outline" size="sm" onClick={() => navigate(from)}>
          ← Back
        </Button>
        <div className="rounded-mga border border-red-500/30 bg-red-500/10 p-6">
          <p className="text-sm text-red-400">
            {game.isError ? game.error.message : 'Game not found.'}
          </p>
        </div>
      </div>
    )
  }

  const data = game.data
  const coverUrl = selectCoverUrl(data.media)
  const playable = isPlayable(data.platform)
  const sources = selectSourcePlugins(data)
  const hltb = formatHLTB(data.completion_time)
  const matchCount = resolverMatchCount(data)

  return (
    <div className="mx-auto max-w-5xl space-y-6 p-4 md:p-6">
      <Button variant="outline" size="sm" onClick={() => navigate(from)}>
        ← Back
      </Button>

      <section className="grid gap-6 rounded-mga border border-mga-border bg-mga-surface p-4 md:grid-cols-[240px,1fr] md:p-6">
        <div className="overflow-hidden rounded-mga border border-mga-border bg-mga-bg">
          <CoverImage src={coverUrl} alt={data.title} className="aspect-[2/3] h-full w-full" />
        </div>

        <div className="space-y-4">
          <div className="space-y-2">
            <h1 className="text-3xl font-semibold tracking-tight text-mga-text">{data.title}</h1>
            <div className="flex flex-wrap items-center gap-2">
              <Badge variant="platform">
                <PlatformIcon platform={data.platform} showLabel />
              </Badge>
              {data.xcloud_available && <Badge variant="xcloud">xCloud</Badge>}
              {data.is_game_pass && <Badge variant="gamepass">GP</Badge>}
              {playable && <Badge variant="playable">Playable</Badge>}
              {hltb && <Badge>{hltb}</Badge>}
              {matchCount > 0 && <Badge>{matchCount} matches</Badge>}
            </div>
          </div>

          {data.description && (
            <p className="text-sm leading-6 text-mga-muted">{data.description}</p>
          )}

          <div className="grid gap-3 text-sm text-mga-muted sm:grid-cols-2">
            <div>
              <p className="font-medium text-mga-text">Release Date</p>
              <p>{data.release_date ?? 'Unknown'}</p>
            </div>
            <div>
              <p className="font-medium text-mga-text">Developer</p>
              <p>{data.developer ?? 'Unknown'}</p>
            </div>
            <div>
              <p className="font-medium text-mga-text">Publisher</p>
              <p>{data.publisher ?? 'Unknown'}</p>
            </div>
            <div>
              <p className="font-medium text-mga-text">Genres</p>
              <p>{data.genres?.join(', ') || 'Unknown'}</p>
            </div>
          </div>

          {sources.length > 0 && (
            <div className="space-y-2">
              <p className="text-sm font-medium text-mga-text">Sources</p>
              <div className="flex flex-wrap gap-2">
                {sources.map((source) => (
                  <Badge key={source} variant="source">
                    {pluginLabel(source)}
                  </Badge>
                ))}
              </div>
            </div>
          )}
        </div>
      </section>

      <section className="rounded-mga border border-mga-border bg-mga-surface p-4 md:p-6">
        <h2 className="text-lg font-semibold text-mga-text">Current Snapshot</h2>
        <div className="mt-4 grid gap-3 text-sm text-mga-muted sm:grid-cols-3">
          <div>
            <p className="font-medium text-mga-text">Source Records</p>
            <p>{data.source_games.length}</p>
          </div>
          <div>
            <p className="font-medium text-mga-text">Media Items</p>
            <p>{data.media?.length ?? 0}</p>
          </div>
          <div>
            <p className="font-medium text-mga-text">Files</p>
            <p>{data.files?.length ?? 0}</p>
          </div>
        </div>
      </section>
    </div>
  )
}
