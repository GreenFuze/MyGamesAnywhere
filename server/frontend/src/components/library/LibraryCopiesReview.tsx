import { useMemo, useState } from 'react'
import { Link } from 'react-router-dom'
import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query'
import {
  Check,
  ChevronDown,
  CircleSlash2,
  Combine,
  Gamepad2,
  Loader2,
  Puzzle,
  RefreshCw,
  RotateCcw,
  Split,
} from 'lucide-react'
import {
  clearSourceGameCanonicalPin,
  getDuplicateGames,
  markManualReviewCandidateDLC,
  markManualReviewCandidateNotAGame,
  mergeSourceGameCanonical,
  splitSourceGameCanonical,
  type DuplicateGameGroup,
  type DuplicateGameMode,
  type DuplicateGameSource,
} from '@/api/client'
import { Button } from '@/components/ui/button'
import { CoverImage } from '@/components/ui/cover-image'
import { platformLabel, selectCoverUrl } from '@/lib/gameUtils'

const REVIEW_MODES: Array<{ id: DuplicateGameMode; label: string; description: string }> = [
  {
    id: 'loose',
    label: 'Similar titles',
    description: 'Games whose names look alike but may be different versions or unrelated.',
  },
  {
    id: 'strict',
    label: 'Games already together',
    description: 'Copies MGA currently shows as one game.',
  },
]

type ReviewAction =
  | { kind: 'same_edition'; source: DuplicateGameSource; reference: DuplicateGameSource }
  | { kind: 'different_edition'; source: DuplicateGameSource }
  | { kind: 'unrelated'; source: DuplicateGameSource }
  | { kind: 'add_on'; source: DuplicateGameSource }
  | { kind: 'not_a_game'; source: DuplicateGameSource }
  | { kind: 'undo'; source: DuplicateGameSource }

function gameTitle(source: DuplicateGameSource): string {
  return source.game?.title || source.canonical_title || source.source.raw_title || source.source.external_id
}

function decisionLabel(source: DuplicateGameSource): string {
  const note = source.source.canonical_pin?.note
  if (note === 'same_edition') return 'Same edition'
  if (note === 'different_edition') return 'Different edition'
  if (note === 'unrelated') return 'Unrelated game'
  if (source.source.canonical_pin?.mode === 'merge') return 'Kept together'
  if (source.source.canonical_pin?.mode === 'split') return 'Kept separate'
  return ''
}

class CopyReviewGroup {
  constructor(
    readonly group: DuplicateGameGroup,
    readonly selectedReferenceId?: string,
  ) {}

  get reference(): DuplicateGameSource {
    return (
      this.group.sources.find((source) => source.source.id === this.selectedReferenceId)
      ?? this.group.sources.find((source) => !source.source.canonical_pin)
      ?? this.group.sources[0]
    )
  }

  isReference(source: DuplicateGameSource): boolean {
    return source.source.id === this.reference?.source.id
  }

  get reviewed(): boolean {
    return this.group.sources.filter((source) => !source.source.canonical_pin).length <= 1
  }
}

export function LibraryCopiesReview() {
  const queryClient = useQueryClient()
  const [mode, setMode] = useState<DuplicateGameMode>('loose')
  const [scope, setScope] = useState<'pending' | 'reviewed'>('pending')
  const [referenceByGroup, setReferenceByGroup] = useState<Record<string, string>>({})
  const [notice, setNotice] = useState('')

  const duplicates = useQuery({
    queryKey: ['duplicate-games', mode],
    queryFn: () => getDuplicateGames(mode),
    refetchOnWindowFocus: false,
  })

  const allGroups = useMemo(
    () => (duplicates.data?.groups ?? []).map((group) => new CopyReviewGroup(group, referenceByGroup[group.id])),
    [duplicates.data?.groups, referenceByGroup],
  )
  const groups = useMemo(
    () => allGroups.filter((group) => (scope === 'reviewed' ? group.reviewed : !group.reviewed)),
    [allGroups, scope],
  )
  const pendingCount = allGroups.filter((group) => !group.reviewed).length
  const reviewedCount = allGroups.length - pendingCount

  const refreshReview = async () => {
    await Promise.all([
      queryClient.invalidateQueries({ queryKey: ['duplicate-games'] }),
      queryClient.invalidateQueries({ queryKey: ['games'] }),
      queryClient.invalidateQueries({ queryKey: ['game'] }),
      queryClient.invalidateQueries({ queryKey: ['manual-review-candidates'] }),
      queryClient.invalidateQueries({ queryKey: ['stats'] }),
      queryClient.invalidateQueries({ queryKey: ['library-statistics'] }),
    ])
  }

  const decisionMutation = useMutation({
    mutationFn: async (action: ReviewAction) => {
      const source = action.source
      switch (action.kind) {
        case 'same_edition':
          return mergeSourceGameCanonical(
            source.canonical_game_id,
            source.source.id,
            action.reference.canonical_game_id,
            'same_edition',
          )
        case 'different_edition':
          return splitSourceGameCanonical(source.canonical_game_id, source.source.id, 'different_edition')
        case 'unrelated':
          return splitSourceGameCanonical(source.canonical_game_id, source.source.id, 'unrelated')
        case 'add_on':
          return markManualReviewCandidateDLC(source.source.id)
        case 'not_a_game':
          return markManualReviewCandidateNotAGame(source.source.id)
        case 'undo':
          return clearSourceGameCanonicalPin(source.canonical_game_id, source.source.id)
      }
    },
    onSuccess: async (_result, action) => {
      const title = gameTitle(action.source)
      const messages: Record<ReviewAction['kind'], string> = {
        same_edition: `${title} will stay with the reference copy.`,
        different_edition: `${title} will stay as its own edition.`,
        unrelated: `${title} will stay as a separate game.`,
        add_on: `${title} was marked as an add-on and moved to Hidden.`,
        not_a_game: `${title} was hidden because it is not a game.`,
        undo: `The saved decision for ${title} was removed.`,
      }
      setNotice(messages[action.kind])
      await refreshReview()
    },
  })

  const error = duplicates.error ?? decisionMutation.error

  return (
    <div className="space-y-5">
      <section className="rounded-mga border border-mga-border bg-mga-surface p-5">
        <div className="flex flex-wrap items-start justify-between gap-4">
          <div>
            <h2 className="text-lg font-semibold text-mga-text">Copies and versions</h2>
            <p className="mt-1 max-w-3xl text-sm leading-6 text-mga-muted">
              Tell MGA which copies belong together. Your files and store entries are left untouched.
            </p>
          </div>
          <Button variant="outline" size="sm" onClick={() => void duplicates.refetch()} disabled={duplicates.isFetching}>
            {duplicates.isFetching ? <Loader2 size={16} className="animate-spin" /> : <RefreshCw size={16} />}
            Refresh
          </Button>
        </div>

        <div className="mt-5 grid gap-2 sm:grid-cols-2">
          {REVIEW_MODES.map((item) => (
            <button
              key={item.id}
              type="button"
              onClick={() => setMode(item.id)}
              className={`rounded-mga border px-4 py-3 text-left transition-colors ${
                mode === item.id
                  ? 'border-mga-accent bg-mga-accent/10 text-mga-text'
                  : 'border-mga-border bg-mga-bg text-mga-muted hover:border-mga-accent/50 hover:text-mga-text'
              }`}
            >
              <span className="block text-sm font-semibold">{item.label}</span>
              <span className="mt-1 block text-xs leading-5">{item.description}</span>
            </button>
          ))}
        </div>
        <div className="mt-4 flex flex-wrap gap-2">
          <Button type="button" size="sm" variant={scope === 'pending' ? 'default' : 'outline'} onClick={() => setScope('pending')}>
            To review · {pendingCount}
          </Button>
          <Button type="button" size="sm" variant={scope === 'reviewed' ? 'default' : 'outline'} onClick={() => setScope('reviewed')}>
            Reviewed · {reviewedCount}
          </Button>
        </div>
      </section>

      {notice ? (
        <div className="rounded-mga border border-emerald-500/30 bg-emerald-500/10 px-4 py-3 text-sm text-emerald-100">
          {notice}
        </div>
      ) : null}

      {error ? (
        <div className="rounded-mga border border-red-500/30 bg-red-500/10 px-4 py-3 text-sm text-red-200">
          {error instanceof Error ? error.message : 'MGA could not save that library decision.'}
        </div>
      ) : null}

      {duplicates.isPending ? (
        <div className="flex items-center gap-2 rounded-mga border border-mga-border bg-mga-surface px-4 py-4 text-sm text-mga-muted">
          <Loader2 size={16} className="animate-spin" />
          Looking for games to review...
        </div>
      ) : groups.length === 0 ? (
        <div className="rounded-mga border border-mga-border bg-mga-surface px-4 py-10 text-center">
          <Check className="mx-auto h-8 w-8 text-emerald-300" />
          <p className="mt-3 font-semibold text-mga-text">
            {scope === 'reviewed' ? 'No reviewed groups here yet' : 'Nothing needs your attention'}
          </p>
          <p className="mt-1 text-sm text-mga-muted">
            {scope === 'reviewed'
              ? 'Saved choices will appear here so you can review or undo them.'
              : 'MGA is keeping uncertain games conservatively separated.'}
          </p>
        </div>
      ) : (
        <div className="space-y-4">
          {groups.map((model) => (
            <section key={model.group.id} className="rounded-mga border border-mga-border bg-mga-surface p-4">
              <div>
                <h3 className="text-base font-semibold text-mga-text">
                  {model.group.representative_title || model.group.normalized_title}
                </h3>
                <p className="mt-1 text-sm text-mga-muted">
                  Choose one copy as the reference, then describe each of the others.
                </p>
              </div>

              <div className="mt-4 grid gap-3 xl:grid-cols-2">
                {model.group.sources.map((source) => {
                  const reference = model.reference
                  const isReference = model.isReference(source)
                  const savedDecision = decisionLabel(source)
                  const pending = decisionMutation.isPending
                  return (
                    <article key={source.source.id} className="rounded-mga border border-mga-border bg-mga-bg p-4">
                      <div className="flex gap-3">
                        <Link
                          to={`/game/${encodeURIComponent(source.canonical_game_id)}`}
                          className="h-24 w-16 shrink-0 overflow-hidden rounded-mga border border-mga-border bg-mga-surface"
                          title={`Open ${gameTitle(source)}`}
                        >
                          <CoverImage
                            src={selectCoverUrl(source.game?.media, source.game?.cover_override)}
                            alt={gameTitle(source)}
                            fit="contain"
                            variant="compact"
                            className="h-full w-full"
                          />
                        </Link>
                        <div className="min-w-0 flex-1">
                          <div className="flex flex-wrap items-start justify-between gap-2">
                            <div>
                              <Link
                                to={`/game/${encodeURIComponent(source.canonical_game_id)}`}
                                className="font-semibold text-mga-text hover:text-mga-accent"
                              >
                                {gameTitle(source)}
                              </Link>
                              <p className="mt-1 text-xs text-mga-muted">
                                {source.source.integration_label || 'Library'} · {platformLabel(source.source.platform || source.game?.platform || 'unknown')}
                              </p>
                            </div>
                            {savedDecision ? (
                              <span className="rounded-full bg-mga-accent/10 px-2 py-1 text-xs font-medium text-mga-accent">
                                {savedDecision}
                              </span>
                            ) : null}
                          </div>

                          <button
                            type="button"
                            onClick={() => setReferenceByGroup((current) => ({ ...current, [model.group.id]: source.source.id }))}
                            className={`mt-3 inline-flex items-center gap-1.5 rounded-full border px-2.5 py-1 text-xs transition-colors ${
                              isReference
                                ? 'border-emerald-400/40 bg-emerald-500/10 text-emerald-200'
                                : 'border-mga-border text-mga-muted hover:border-mga-accent/50 hover:text-mga-text'
                            }`}
                          >
                            <Gamepad2 size={13} />
                            {isReference ? 'Reference copy' : 'Use as reference'}
                          </button>
                        </div>
                      </div>

                      <div className="mt-4 flex flex-wrap gap-2">
                        {!isReference ? (
                          <Button
                            type="button"
                            size="sm"
                            onClick={() => decisionMutation.mutate({ kind: 'same_edition', source, reference })}
                            disabled={pending}
                            title={`Show this with ${gameTitle(reference)}`}
                          >
                            <Combine size={14} />
                            Same edition
                          </Button>
                        ) : null}
                        <Button
                          type="button"
                          variant="outline"
                          size="sm"
                          onClick={() => decisionMutation.mutate({ kind: 'different_edition', source })}
                          disabled={pending}
                          title="Keep this as another edition of the same title"
                        >
                          <Split size={14} />
                          Different edition
                        </Button>
                        <Button
                          type="button"
                          variant="outline"
                          size="sm"
                          onClick={() => decisionMutation.mutate({ kind: 'unrelated', source })}
                          disabled={pending}
                          title="This is not the same game despite the similar name"
                        >
                          <CircleSlash2 size={14} />
                          Unrelated
                        </Button>
                        {source.source.canonical_pin ? (
                          <Button
                            type="button"
                            variant="ghost"
                            size="sm"
                            onClick={() => decisionMutation.mutate({ kind: 'undo', source })}
                            disabled={pending}
                            title="Remove your saved choice and let MGA identify it again"
                          >
                            <RotateCcw size={14} />
                            Undo
                          </Button>
                        ) : null}
                      </div>

                      <details className="group mt-3 rounded-mga border border-mga-border/70 bg-mga-surface/50">
                        <summary className="flex cursor-pointer list-none items-center justify-between gap-2 px-3 py-2 text-xs font-medium text-mga-muted hover:text-mga-text">
                          This item is not a standalone game
                          <ChevronDown size={14} className="transition-transform group-open:rotate-180" />
                        </summary>
                        <div className="flex flex-wrap gap-2 border-t border-mga-border/70 p-3">
                          <Button
                            type="button"
                            variant="outline"
                            size="sm"
                            onClick={() => decisionMutation.mutate({ kind: 'add_on', source })}
                            disabled={pending}
                            title="Move this item to Hidden as downloadable content or an add-on"
                          >
                            <Puzzle size={14} />
                            Add-on
                          </Button>
                          <Button
                            type="button"
                            variant="outline"
                            size="sm"
                            onClick={() => decisionMutation.mutate({ kind: 'not_a_game', source })}
                            disabled={pending}
                            title="Move this item to Hidden without deleting any files"
                          >
                            <CircleSlash2 size={14} />
                            Not a game
                          </Button>
                          <p className="basis-full text-xs leading-5 text-mga-muted">
                            These choices only hide the item from the game library. You can restore it from Games to identify → Hidden.
                          </p>
                        </div>
                      </details>
                    </article>
                  )
                })}
              </div>
            </section>
          ))}
        </div>
      )}
    </div>
  )
}
