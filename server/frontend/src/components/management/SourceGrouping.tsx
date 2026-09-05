import { useEffect, useState } from 'react'
import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query'
import { Split, Undo2 } from 'lucide-react'
import {
  clearSourceGameCanonicalPin, mergeSourceGameCanonical, searchCanonicalGames, splitSourceGameCanonical,
  type CanonicalSourcePinDTO,
} from '@/api/client'
import { Input } from '@/components/ui/input'
import { ActionError, FormDialog } from '@/components/management/ManagementActions'
import { platformLabel } from '@/lib/displayText'

/**
 * Putting a source in the right place when MGA guessed wrong.
 *
 * Grouping is automatic and usually right, and when it is wrong it is wrong in
 * two directions: two different games ended up as one entry, or one game is
 * sitting in two. The server has been able to fix both since the grouping
 * service landed; nothing in the console could ask it to.
 *
 * A decision made here is remembered as a pin, so a later scan does not undo
 * it. That is why every one of them can be taken back.
 */
export function SourceGroupingControls({
  canonicalGameId, sourceGameId, sourceName, pin, canSplit, allowed,
}: {
  canonicalGameId: string
  sourceGameId: string
  sourceName: string
  pin?: CanonicalSourcePinDTO
  /** Splitting the only source would move a game onto itself. */
  canSplit: boolean
  allowed: boolean
}) {
  const queryClient = useQueryClient()
  const [choosing, setChoosing] = useState(false)

  const refresh = () => {
    // Regrouping changes which games exist, so the list is as stale as the page.
    void queryClient.invalidateQueries({ queryKey: ['management', 'game', canonicalGameId] })
    void queryClient.invalidateQueries({ queryKey: ['management', 'library'] })
  }

  const split = useMutation({
    mutationFn: (decision: 'different_edition' | 'unrelated') =>
      splitSourceGameCanonical(canonicalGameId, sourceGameId, decision),
    onSuccess: refresh,
  })
  const undo = useMutation({
    mutationFn: () => clearSourceGameCanonicalPin(canonicalGameId, sourceGameId),
    onSuccess: refresh,
  })

  if (!allowed) return null

  if (pin) {
    return (
      <div className="mt-3 flex flex-wrap items-center gap-2 border-t border-mga-border pt-3">
        <p className="text-[0.68rem] text-mga-muted">
          {pin.mode === 'split'
            ? 'You said this is not the same game, and MGA keeps it apart.'
            : 'You put this here, and MGA keeps it here.'}
        </p>
        <button
          type="button"
          disabled={undo.isPending}
          onClick={() => undo.mutate()}
          className="inline-flex items-center gap-1 rounded-md border border-mga-border px-2 py-1 text-[0.68rem] text-mga-muted transition hover:text-mga-text disabled:opacity-50"
        >
          <Undo2 className="h-3 w-3" />
          {undo.isPending ? 'Undoing…' : 'Let MGA decide again'}
        </button>
        <ActionError error={undo.error} />
      </div>
    )
  }

  return (
    <div className="mt-3 flex flex-wrap items-center gap-2 border-t border-mga-border pt-3">
      <span className="text-[0.68rem] text-mga-muted">Not the same game?</span>
      {canSplit && (
        <>
          <GroupingButton
            busy={split.isPending}
            onClick={() => split.mutate('unrelated')}
            label="It is a different game"
          />
          <GroupingButton
            busy={split.isPending}
            onClick={() => split.mutate('different_edition')}
            label="It is another edition"
          />
        </>
      )}
      <GroupingButton busy={false} onClick={() => setChoosing(true)} label="It belongs to another game" />
      <ActionError error={split.error} />

      {choosing && (
        <MoveToGameDialog
          canonicalGameId={canonicalGameId}
          sourceGameId={sourceGameId}
          sourceName={sourceName}
          onClose={() => setChoosing(false)}
          onMoved={() => { setChoosing(false); refresh() }}
        />
      )}
    </div>
  )
}

function GroupingButton({ busy, onClick, label }: { busy: boolean; onClick: () => void; label: string }) {
  return (
    <button
      type="button"
      disabled={busy}
      onClick={onClick}
      className="inline-flex items-center gap-1 rounded-md border border-mga-border px-2 py-1 text-[0.68rem] text-mga-muted transition hover:text-mga-text disabled:opacity-50"
    >
      <Split className="h-3 w-3" />
      {label}
    </button>
  )
}

/** Picks the game this source should have been part of all along. */
function MoveToGameDialog({
  canonicalGameId, sourceGameId, sourceName, onClose, onMoved,
}: {
  canonicalGameId: string
  sourceGameId: string
  sourceName: string
  onClose: () => void
  onMoved: () => void
}) {
  const [typed, setTyped] = useState('')
  const [term, setTerm] = useState('')
  const [target, setTarget] = useState('')

  useEffect(() => {
    const timer = window.setTimeout(() => setTerm(typed.trim()), 250)
    return () => window.clearTimeout(timer)
  }, [typed])

  const results = useQuery({
    queryKey: ['management', 'canonical-search', term],
    queryFn: () => searchCanonicalGames({ q: term, limit: 20 }),
    enabled: term.length > 1,
  })

  const move = useMutation({
    mutationFn: () => mergeSourceGameCanonical(canonicalGameId, sourceGameId, target),
    onSuccess: onMoved,
  })

  // The game it is already part of is not somewhere it can be moved to.
  const options = (results.data?.games ?? []).filter((game) => game.id !== canonicalGameId)

  return (
    <FormDialog
      open
      onClose={onClose}
      title="Move it to another game"
      description={`Where should ${sourceName} belong instead?`}
      submitLabel="Move it"
      submitting={move.isPending}
      error={move.error}
      disabled={target === ''}
      onSubmit={() => move.mutate()}
    >
      <Input
        label="Search your games"
        value={typed}
        onChange={(event) => setTyped(event.target.value)}
        placeholder="Part of the name"
        autoFocus
      />
      {term.length > 1 && results.isPending && <p className="text-xs text-mga-muted">Looking…</p>}
      {term.length > 1 && !results.isPending && options.length === 0 && (
        <p className="text-xs text-mga-muted">Nothing else is called that.</p>
      )}
      {options.length > 0 && (
        <ul className="max-h-56 space-y-1 overflow-auto">
          {options.map((game) => (
            <li key={game.id}>
              <label className="flex cursor-pointer items-center gap-2 rounded-md border border-mga-border bg-mga-elevated/40 px-2.5 py-2">
                <input
                  type="radio"
                  name="move-target"
                  checked={target === game.id}
                  onChange={() => setTarget(game.id)}
                  className="h-3.5 w-3.5 accent-[color:var(--mga-accent,#7c5cff)]"
                />
                <span className="min-w-0">
                  <span className="block truncate text-xs text-mga-text">{game.title}</span>
                  <span className="block text-[0.66rem] text-mga-muted">
                    {platformLabel(game.platform)} · {game.source_count} source{game.source_count === 1 ? '' : 's'}
                  </span>
                </span>
              </label>
            </li>
          ))}
        </ul>
      )}
      <p className="text-[0.68rem] leading-5 text-mga-muted">
        MGA remembers this, so the next scan will not put it back. You can undo it afterwards.
      </p>
    </FormDialog>
  )
}
