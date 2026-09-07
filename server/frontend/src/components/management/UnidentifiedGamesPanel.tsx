import { useState } from 'react'
import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query'
import { HelpCircle, RefreshCw } from 'lucide-react'
import { listManualReviewCandidates, redetectActiveManualReviewCandidates } from '@/api/client'
import { Button } from '@/components/ui/button'
import { ActionError } from '@/components/management/ManagementActions'
import { SectionCard, formatCount } from '@/components/management/ManagementPrimitives'
import { platformLabel } from '@/lib/displayText'

/**
 * Games MGA found and could not name.
 *
 * These exist as real files on a real drive, and until now they appeared
 * nowhere at all: a game with no metadata match never becomes a library entry,
 * and nothing said so. On one library that hid 75 games, 48 of them an entire
 * arcade collection, and the only symptom was a library that felt small.
 *
 * The scheduled run retries them on its own now, so this is not a chore list —
 * most of these resolve themselves. It is here so that "where are my games" has
 * an answer on the screen rather than in a database.
 */
export function UnidentifiedGamesPanel({ isAdmin }: { isAdmin: boolean }) {
  const queryClient = useQueryClient()
  const [notice, setNotice] = useState<string | null>(null)

  const candidates = useQuery({
    queryKey: ['management', 'review-candidates', 'active'],
    queryFn: () => listManualReviewCandidates('active', 200),
    enabled: isAdmin,
  })

  const retry = useMutation({
    mutationFn: redetectActiveManualReviewCandidates,
    onMutate: () => setNotice(null),
    onSuccess: async (result) => {
      // Report what happened rather than that something happened. "Try again"
      // that says "done" teaches nobody whether it is worth pressing twice.
      setNotice(
        result.matched > 0
          ? `Identified ${formatCount(result.matched)} of ${formatCount(result.attempted)}. The rest are still unnamed.`
          : `Tried ${formatCount(result.attempted)} and identified none. The sources that name games may be unreachable right now.`,
      )
      await queryClient.invalidateQueries({ queryKey: ['management', 'review-candidates', 'active'] })
      await queryClient.invalidateQueries({ queryKey: ['management', 'library'] })
    },
  })

  if (!isAdmin) {
    return null
  }

  const games = candidates.data ?? []
  if (candidates.isPending || (games.length === 0 && !notice)) {
    // Nothing unnamed is the normal state, and a panel announcing zero problems
    // is just something else to read.
    return null
  }

  return (
    <SectionCard
      title="Games MGA could not name"
      description="These are on your drives, but no source could tell MGA what they are, so they are not in the library yet."
    >
      <div className="space-y-3">
        <div className="flex flex-wrap items-center justify-between gap-3">
          <p className="text-xs leading-5 text-mga-muted">
            {games.length > 0
              ? `${formatCount(games.length)} game${games.length === 1 ? '' : 's'} waiting to be named. MGA retries these after every scan; this button asks it to try now.`
              : 'Nothing is waiting to be named.'}
          </p>
          <Button variant="outline" size="sm" disabled={retry.isPending} onClick={() => retry.mutate()}>
            <RefreshCw className="h-3.5 w-3.5" /> {retry.isPending ? 'Trying…' : 'Try again now'}
          </Button>
        </div>

        {notice && <p className="text-xs leading-5 text-emerald-300">{notice}</p>}
        <ActionError error={candidates.error ?? retry.error} />

        {games.length > 0 && (
          <ul className="divide-y divide-mga-border/60 rounded-lg border border-mga-border">
            {games.slice(0, 50).map((game) => (
              <li key={game.id} className="flex items-start gap-2 px-3 py-2">
                <HelpCircle className="mt-0.5 h-3.5 w-3.5 shrink-0 text-mga-muted" />
                <div className="min-w-0">
                  <p className="truncate text-xs text-mga-text">{game.current_title || game.raw_title}</p>
                  <p className="mt-0.5 text-[0.68rem] text-mga-muted">
                    {[game.integration_label, platformLabel(game.platform)].filter(Boolean).join(' · ')}
                  </p>
                </div>
              </li>
            ))}
          </ul>
        )}
        {games.length > 50 && (
          <p className="text-[0.68rem] text-mga-muted">
            Showing the first 50 of {formatCount(games.length)}.
          </p>
        )}
      </div>
    </SectionCard>
  )
}
