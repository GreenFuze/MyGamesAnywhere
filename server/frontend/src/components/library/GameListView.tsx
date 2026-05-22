import { Link } from 'react-router-dom'
import { ExternalLink, Heart, Trophy } from 'lucide-react'
import type { GameDetailResponse } from '@/api/client'
import { CoverImage } from '@/components/ui/cover-image'
import { StatusBadge } from '@/components/ui/status-badge'
import { BrandIcon } from '@/components/ui/brand-icon'
import {
  isPlayable,
  platformLabel,
  selectCoverUrl,
  selectSourceIntegrations,
} from '@/lib/gameUtils'

interface GameListViewProps {
  games: GameDetailResponse[]
  selectedIds: Set<string>
  onToggleSelected: (game: GameDetailResponse) => void
}

function formatBytes(value: number): string {
  if (value <= 0) return '0 B'
  const units = ['B', 'KB', 'MB', 'GB', 'TB']
  let size = value
  let unitIndex = 0
  while (size >= 1024 && unitIndex < units.length - 1) {
    size /= 1024
    unitIndex += 1
  }
  return `${size.toFixed(size >= 10 || unitIndex === 0 ? 0 : 1)} ${units[unitIndex]}`
}

function sourceFileSummary(game: GameDetailResponse): { fileCount: number; byteCount: number; pathHint: string } {
  let fileCount = 0
  let byteCount = 0
  let pathHint = ''

  for (const source of game.source_games ?? []) {
    if (!pathHint && source.root_path) pathHint = source.root_path
    for (const file of source.files ?? []) {
      fileCount += 1
      byteCount += file.size ?? 0
      if (!pathHint && file.path) pathHint = file.path
    }
  }

  return { fileCount, byteCount, pathHint }
}

function achievementText(game: GameDetailResponse): string | null {
  const summary = game.achievement_summary
  if (!summary || summary.total_count <= 0) return null
  return `${summary.unlocked_count}/${summary.total_count}`
}

function GameListRow({ game, selected, onToggleSelected }: { game: GameDetailResponse; selected: boolean; onToggleSelected: (game: GameDetailResponse) => void }) {
  const coverUrl = selectCoverUrl(game.media, game.cover_override)
  const sources = selectSourceIntegrations(game)
  const fileSummary = sourceFileSummary(game)
  const achievements = achievementText(game)
  const playable = isPlayable(game)

  return (
    <article className="grid gap-3 rounded-mga border border-mga-border bg-mga-surface p-3 transition-colors hover:border-mga-accent/40 sm:grid-cols-[auto_4.25rem_minmax(0,1.35fr)_minmax(9rem,0.65fr)_minmax(10rem,0.8fr)_auto] sm:items-center">
      <label className="flex items-center gap-2 text-sm text-mga-muted sm:block">
        <input
          type="checkbox"
          checked={selected}
          onChange={() => onToggleSelected(game)}
          className="h-4 w-4 rounded border-mga-border bg-mga-bg accent-mga-accent"
          aria-label={`Select ${game.title}`}
        />
        <span className="sm:hidden">Select</span>
      </label>

      <Link to={`/game/${encodeURIComponent(game.id)}`} className="block w-20 overflow-hidden rounded-mga border border-mga-border bg-mga-bg sm:w-full" aria-label={`Open ${game.title}`}>
        <CoverImage src={coverUrl} alt={game.title} fit="contain" variant="compact" className="aspect-[3/4] w-full" />
      </Link>

      <div className="min-w-0">
        <Link to={`/game/${encodeURIComponent(game.id)}`} className="group inline-flex max-w-full items-center gap-2 text-base font-semibold text-mga-text hover:text-mga-accent">
          <span className="truncate [overflow-wrap:anywhere]">{game.title || 'Untitled game'}</span>
          <ExternalLink size={14} className="shrink-0 opacity-0 transition-opacity group-hover:opacity-100" />
        </Link>
        <div className="mt-1 flex flex-wrap items-center gap-2 text-xs text-mga-muted">
          <span>{platformLabel(game.platform)}</span>
          <span aria-hidden="true">·</span>
          <span>{game.kind || 'unknown'}</span>
          {game.release_date ? (
            <>
              <span aria-hidden="true">·</span>
              <span>{game.release_date.substring(0, 4)}</span>
            </>
          ) : null}
        </div>
        <div className="mt-2 flex flex-wrap gap-1.5">
          {game.favorite ? (
            <span className="inline-flex h-6 items-center gap-1 rounded-full border border-rose-400/25 bg-rose-500/10 px-2 text-xs text-rose-200">
              <Heart size={12} fill="currentColor" />
              Favorite
            </span>
          ) : null}
          {playable ? <StatusBadge kind="playable" /> : null}
          {game.xcloud_available ? <StatusBadge kind="xcloud" /> : null}
          {game.is_game_pass ? <StatusBadge kind="gamepass" /> : null}
          {achievements ? (
            <span className="inline-flex h-6 items-center gap-1 rounded-full border border-yellow-400/25 bg-yellow-500/10 px-2 text-xs text-yellow-100">
              <Trophy size={12} />
              {achievements}
            </span>
          ) : null}
        </div>
      </div>

      <div className="min-w-0 text-sm">
        <p className="font-medium text-mga-text">{game.source_games.length} source{game.source_games.length === 1 ? '' : 's'}</p>
        <div className="mt-1 flex flex-wrap gap-1.5">
          {sources.slice(0, 3).map((source) => (
            <span key={source.key} className="inline-flex max-w-full items-center gap-1 rounded-full border border-mga-border bg-mga-bg px-2 py-1 text-xs text-mga-muted">
              <BrandIcon brand={source.pluginId} className="h-3 w-3 shrink-0" />
              <span className="truncate">{source.label}</span>
            </span>
          ))}
          {sources.length > 3 ? <span className="text-xs text-mga-muted">+{sources.length - 3}</span> : null}
        </div>
      </div>

      <div className="min-w-0 text-sm text-mga-muted">
        <p className="text-mga-text">{fileSummary.fileCount} file{fileSummary.fileCount === 1 ? '' : 's'} · {formatBytes(fileSummary.byteCount)}</p>
        {fileSummary.pathHint ? <p className="mt-1 truncate font-mono text-xs">{fileSummary.pathHint}</p> : null}
      </div>

      <Link
        to={`/game/${encodeURIComponent(game.id)}`}
        className="inline-flex h-9 items-center justify-center rounded-mga border border-mga-border bg-mga-bg px-3 text-sm font-medium text-mga-text hover:bg-mga-elevated"
      >
        Details
      </Link>
    </article>
  )
}

export function GameListView({ games, selectedIds, onToggleSelected }: GameListViewProps) {
  return (
    <div className="space-y-2">
      <div className="hidden grid-cols-[auto_4.25rem_minmax(0,1.35fr)_minmax(9rem,0.65fr)_minmax(10rem,0.8fr)_auto] gap-3 px-3 text-xs font-medium uppercase tracking-wide text-mga-muted sm:grid">
        <span />
        <span>Cover</span>
        <span>Game</span>
        <span>Sources</span>
        <span>Files</span>
        <span />
      </div>
      {games.map((game) => (
        <GameListRow
          key={game.id}
          game={game}
          selected={selectedIds.has(game.id)}
          onToggleSelected={onToggleSelected}
        />
      ))}
    </div>
  )
}
