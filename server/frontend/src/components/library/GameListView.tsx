import { Link } from 'react-router-dom'
import { Heart, Save, Trophy } from 'lucide-react'
import type { GameDetailResponse } from '@/api/client'
import { GameSplitActionButton } from '@/components/library/GameSplitActionButton'
import { BrandIcon } from '@/components/ui/brand-icon'
import { CoverImage } from '@/components/ui/cover-image'
import { StatusBadge } from '@/components/ui/status-badge'
import { useGameCardActions } from '@/hooks/useGameCardActions'
import { selectCoverUrl } from '@/lib/gameUtils'
import { GamePresentation } from '@/lib/gamePresentation'

interface GameListViewProps {
  games: GameDetailResponse[]
  selectedIds: Set<string>
  onToggleSelected: (game: GameDetailResponse) => void
}

function GameListRow({
  game,
  selected,
  onToggleSelected,
}: {
  game: GameDetailResponse
  selected: boolean
  onToggleSelected: (game: GameDetailResponse) => void
}) {
  const coverUrl = selectCoverUrl(game.media, game.cover_override)
  const presentation = new GamePresentation(game)
  const { primaryAction, alternateActions } = useGameCardActions(game)

  return (
    <article className="grid gap-3 rounded-mga border border-mga-border bg-mga-surface p-3 transition-colors hover:border-mga-accent/40 sm:grid-cols-[auto_4.25rem_minmax(0,1.35fr)_minmax(11rem,0.8fr)_minmax(10rem,0.75fr)_auto] sm:items-center">
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

      <Link
        to={`/game/${encodeURIComponent(game.id)}`}
        className="block w-20 overflow-hidden rounded-mga border border-mga-border bg-mga-bg sm:w-full"
        aria-label={`View details for ${game.title}`}
      >
        <CoverImage
          src={coverUrl}
          alt={game.title}
          fit="contain"
          variant="compact"
          className="aspect-[3/4] w-full"
        />
      </Link>

      <div className="min-w-0">
        <Link
          to={`/game/${encodeURIComponent(game.id)}`}
          className="inline-flex max-w-full text-base font-semibold text-mga-text hover:text-mga-accent"
        >
          <span className="truncate [overflow-wrap:anywhere]">{game.title || 'Untitled game'}</span>
        </Link>
        <div className="mt-1 flex flex-wrap items-center gap-2 text-xs text-mga-muted">
          <span>{presentation.platform}</span>
          {presentation.content.badgeLabel ? (
            <>
              <span aria-hidden="true">·</span>
              <span>{presentation.content.badgeLabel}</span>
            </>
          ) : null}
          {game.release_date ? (
            <>
              <span aria-hidden="true">·</span>
              <span>{game.release_date.substring(0, 4)}</span>
            </>
          ) : null}
        </div>
        <div className="mt-2 flex flex-wrap items-center gap-1.5">
          {game.favorite ? (
            <span
              className="inline-flex h-6 items-center gap-1 rounded-full border border-rose-400/25 bg-rose-500/10 px-2 text-xs text-rose-200"
              title="Favorite"
            >
              <Heart size={12} fill="currentColor" />
              Favorite
            </span>
          ) : null}
          {presentation.availability ? <StatusBadge kind={presentation.availability} /> : null}
          {presentation.achievementLabel ? (
            <span
              className="inline-flex h-6 items-center gap-1 rounded-full border border-yellow-400/25 bg-yellow-500/10 px-2 text-xs text-yellow-100"
              title={presentation.achievementLabel}
            >
              <Trophy size={12} />
              {game.achievement_summary?.unlocked_count}/{game.achievement_summary?.total_count}
            </span>
          ) : null}
          {presentation.saveLabel ? (
            <span
              className="inline-flex h-6 items-center gap-1 rounded-full border border-sky-400/25 bg-sky-500/10 px-2 text-xs text-sky-100"
              title={presentation.saveLabel}
            >
              <Save size={12} />
              Saves
            </span>
          ) : null}
        </div>
      </div>

      <div className="min-w-0 text-sm">
        <p className="font-medium text-mga-text">{presentation.copyCountLabel}</p>
        <div className="mt-1.5 flex flex-wrap gap-1.5">
          {presentation.sources.slice(0, 2).map((source) => (
            <span
              key={source.key}
              className="inline-flex max-w-full items-center gap-1 rounded-full border border-mga-border bg-mga-bg px-2 py-1 text-xs text-mga-muted"
              title={`Found in ${source.label}`}
            >
              <BrandIcon brand={source.pluginId} className="h-3 w-3 shrink-0" />
              <span className="truncate">{source.label}</span>
            </span>
          ))}
          {presentation.sources.length > 2 ? (
            <span className="self-center text-xs text-mga-muted">+{presentation.sources.length - 2}</span>
          ) : null}
        </div>
      </div>

      <div className="min-w-0 text-sm text-mga-muted">
        <p className="text-mga-text">{presentation.foundInLabel}</p>
        <p className="mt-1 text-xs">
          {presentation.availability ? 'Ready options are in Play' : 'View details to get this ready'}
        </p>
      </div>

      <GameSplitActionButton
        gameTitle={game.title}
        primaryAction={primaryAction}
        alternateActions={alternateActions}
        appearance="surface"
        onSelect={(action, event) => {
          event.preventDefault()
          event.stopPropagation()
          if (!action.disabled) action.onSelect()
        }}
      />
    </article>
  )
}

export function GameListView({ games, selectedIds, onToggleSelected }: GameListViewProps) {
  return (
    <div className="space-y-2">
      <div className="hidden grid-cols-[auto_4.25rem_minmax(0,1.35fr)_minmax(11rem,0.8fr)_minmax(10rem,0.75fr)_auto] gap-3 px-3 text-xs font-medium uppercase tracking-wide text-mga-muted sm:grid">
        <span />
        <span>Cover</span>
        <span>Game</span>
        <span>Copies</span>
        <span>Available</span>
        <span>Play</span>
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
