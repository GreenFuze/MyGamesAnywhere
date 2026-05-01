import { useEffect, useMemo, useState } from 'react'
import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query'
import { AlertCircle, ArrowRightLeft, ExternalLink, FileSearch, Loader2, RefreshCw, Search, Trash2 } from 'lucide-react'
import { useNavigate, useSearchParams } from 'react-router-dom'
import {
  ApiError,
  applyManualReviewCandidate,
  deleteManualReviewCandidateFiles,
  getManualReviewCandidate,
  listManualReviewCandidates,
  markManualReviewCandidateNotAGame,
  redetectActiveManualReviewCandidates,
  redetectManualReviewCandidate,
  searchManualReviewCandidate,
  unarchiveManualReviewCandidate,
  type ManualReviewCandidateDetail,
  type ManualReviewCandidateSummary,
  type ManualReviewScope,
  type ManualReviewSearchResult,
} from '@/api/client'
import { BrandBadge } from '@/components/ui/brand-icon'
import { Badge } from '@/components/ui/badge'
import { Button } from '@/components/ui/button'
import { CoverImage } from '@/components/ui/cover-image'
import { Dialog } from '@/components/ui/dialog'
import { Input } from '@/components/ui/input'
import { PlatformIcon } from '@/components/ui/platform-icon'
import { brandLabel } from '@/lib/brands'
import { pluginLabel } from '@/lib/gameUtils'

const CANDIDATE_LIMIT = 200

function safeText(value: string | null | undefined): string {
  return typeof value === 'string' ? value : ''
}

function safeList<T>(value: T[] | null | undefined): T[] {
  return Array.isArray(value) ? value : []
}

function formatDateTime(value: string | undefined): string {
  if (!value) return 'Unknown'
  const date = new Date(value)
  if (Number.isNaN(date.getTime())) return value
  return new Intl.DateTimeFormat(undefined, { dateStyle: 'medium', timeStyle: 'short' }).format(date)
}

function formatBytes(bytes: number): string {
  if (!Number.isFinite(bytes) || bytes <= 0) return '0 B'
  const units = ['B', 'KB', 'MB', 'GB', 'TB']
  const exponent = Math.min(Math.floor(Math.log(bytes) / Math.log(1024)), units.length - 1)
  const amount = bytes / 1024 ** exponent
  return `${amount.toFixed(amount >= 10 || exponent === 0 ? 0 : 1)} ${units[exponent]}`
}

function humanizeValue(value: string | undefined): string {
  if (!value) return 'Unknown'
  return value
    .split(/[_-]+/g)
    .filter(Boolean)
    .map((part) => (part.length <= 3 ? part.toUpperCase() : part.charAt(0).toUpperCase() + part.slice(1)))
    .join(' ')
}

function reviewReasonLabel(reason: string): string {
  switch (reason) {
    case 'no_metadata_matches':
      return 'No metadata matches'
    case 'no_resolved_title':
      return 'No resolved title'
    case 'unknown_platform':
      return 'Unknown platform'
    case 'unknown_grouping':
      return 'Unknown grouping'
    default:
      return humanizeValue(reason)
  }
}

function reviewStateLabel(state: ManualReviewCandidateDetail['review_state']): string {
  switch (state) {
    case 'matched':
      return 'Match Applied'
    case 'not_a_game':
      return 'Archived as Not a Game'
    default:
      return 'Pending Review'
  }
}

function reviewStateVariant(state: ManualReviewCandidateDetail['review_state']): 'accent' | 'muted' | 'default' {
  switch (state) {
    case 'matched':
      return 'muted'
    case 'not_a_game':
      return 'default'
    default:
      return 'accent'
  }
}

function statusTone(status: string): 'accent' | 'muted' | 'default' {
  if (status === 'success') return 'accent'
  if (status === 'error') return 'default'
  return 'muted'
}

function preferredSearchQuery(candidate: ManualReviewCandidateDetail | undefined): string {
  if (!candidate) return ''
  return safeText(candidate.current_title).trim() || safeText(candidate.raw_title).trim()
}

function selectInitialCandidate(
  candidates: ManualReviewCandidateSummary[],
  legacy: { canonicalGameId?: string | null; title?: string | null; platform?: string | null; source?: string | null },
): ManualReviewCandidateSummary | null {
  if (candidates.length === 0) return null
  if (legacy.canonicalGameId) {
    const byCanonical = candidates.find((candidate) => candidate.canonical_game_id === legacy.canonicalGameId)
    if (byCanonical) return byCanonical
  }
  const normalizedTitle = legacy.title?.trim().toLowerCase()
  const normalizedPlatform = legacy.platform?.trim().toLowerCase()
  const normalizedSource = legacy.source?.trim().toLowerCase()
  const exact = candidates.find((candidate) => {
    const current = safeText(candidate.current_title).trim().toLowerCase()
    const raw = safeText(candidate.raw_title).trim().toLowerCase()
    const platformMatches = !normalizedPlatform || safeText(candidate.platform).trim().toLowerCase() === normalizedPlatform
    const sourceMatches = !normalizedSource || safeText(candidate.plugin_id).trim().toLowerCase() === normalizedSource
    const titleMatches = normalizedTitle && (current === normalizedTitle || raw === normalizedTitle)
    return Boolean(titleMatches && platformMatches && sourceMatches)
  })
  if (exact) return exact
  return candidates[0]
}

function mutationErrorMessage(error: unknown, fallback: string): string {
  if (error instanceof ApiError) return error.responseText || error.message
  if (error instanceof Error && error.message) return error.message
  return fallback
}

function EmptyState({ title, description }: { title: string; description: string }) {
  return (
    <div className="rounded-mga border border-dashed border-mga-border bg-mga-surface p-8 text-center">
      <h3 className="text-base font-semibold text-mga-text">{title}</h3>
      <p className="mt-2 text-sm leading-6 text-mga-muted">{description}</p>
    </div>
  )
}

function ErrorState({ message }: { message: string }) {
  return <div className="rounded-mga border border-red-500/30 bg-red-500/10 p-6 text-sm text-red-300">{message}</div>
}

export function UndetectedGamesTab() {
  const queryClient = useQueryClient()
  const navigate = useNavigate()
  const [searchParams, setSearchParams] = useSearchParams()
  const [candidateFilter, setCandidateFilter] = useState('')
  const [searchQuery, setSearchQuery] = useState('')
  const [submittedQuery, setSubmittedQuery] = useState('')
  const [seededCandidateId, setSeededCandidateId] = useState<string | null>(null)
  const [redetectNotice, setRedetectNotice] = useState<string | null>(null)
  const [deleteDialogOpen, setDeleteDialogOpen] = useState(false)

  const scope: ManualReviewScope = searchParams.get('scope') === 'archive' ? 'archive' : 'active'
  const activeCandidateId = searchParams.get('candidate_id')?.trim() ?? ''
  const legacyGameId = searchParams.get('reclassify_game_id')
  const legacyTitle = searchParams.get('reclassify_title')
  const legacyPlatform = searchParams.get('reclassify_platform')
  const legacySource = searchParams.get('reclassify_source')

  const candidatesQuery = useQuery({
    queryKey: ['manual-review-candidates', scope],
    queryFn: () => listManualReviewCandidates(scope, CANDIDATE_LIMIT),
  })

  const candidateQuery = useQuery({
    queryKey: ['manual-review-candidate', activeCandidateId],
    queryFn: () => getManualReviewCandidate(activeCandidateId),
    enabled: activeCandidateId.length > 0,
  })

  const searchResultsQuery = useQuery({
    queryKey: ['manual-review-search', activeCandidateId, submittedQuery],
    queryFn: () => searchManualReviewCandidate(activeCandidateId, submittedQuery),
    enabled:
      activeCandidateId.length > 0 &&
      submittedQuery.trim().length > 0 &&
      candidateQuery.data?.review_state !== 'not_a_game',
  })

  const filteredCandidates = useMemo(() => {
    const items = candidatesQuery.data ?? []
    const filter = candidateFilter.trim().toLowerCase()
    if (!filter) return items
    return items.filter((candidate) => {
      const haystack = [
        safeText(candidate.current_title),
        safeText(candidate.raw_title),
        safeText(candidate.integration_label),
        safeText(candidate.platform),
        safeText(candidate.plugin_id),
        safeText(candidate.root_path),
      ]
        .filter(Boolean)
        .join(' ')
        .toLowerCase()
      return haystack.includes(filter)
    })
  }, [candidateFilter, candidatesQuery.data])

  const selectedInScope =
    filteredCandidates.some((candidate) => candidate.id === activeCandidateId) ||
    (candidatesQuery.data ?? []).some((candidate) => candidate.id === activeCandidateId)

  const selectCandidate = (candidateId: string) => {
    const next = new URLSearchParams(searchParams)
    next.set('tab', 'undetected')
    next.set('scope', scope)
    next.set('candidate_id', candidateId)
    setSearchParams(next, { replace: true })
  }

  const setScope = (nextScope: ManualReviewScope) => {
    const next = new URLSearchParams(searchParams)
    next.set('tab', 'undetected')
    next.set('scope', nextScope)
    next.delete('candidate_id')
    setSearchParams(next, { replace: true })
  }

  const focusNextCandidateInScope = (candidateId: string): boolean => {
    const items = candidatesQuery.data ?? []
    const index = items.findIndex((candidate) => candidate.id === candidateId)
    if (index < 0) return false
    const nextCandidate = items[index + 1] ?? items[index - 1] ?? null
    const next = new URLSearchParams(searchParams)
    next.set('tab', 'undetected')
    next.set('scope', scope)
    if (nextCandidate) next.set('candidate_id', nextCandidate.id)
    else next.delete('candidate_id')
    setSearchParams(next, { replace: true })
    return true
  }

  const invalidateReviewQueries = (candidateId?: string) => {
    void queryClient.invalidateQueries({ queryKey: ['manual-review-candidates'] })
    if (candidateId) void queryClient.invalidateQueries({ queryKey: ['manual-review-candidate', candidateId] })
    void queryClient.invalidateQueries({ queryKey: ['manual-review-search'] })
    void queryClient.invalidateQueries({ queryKey: ['games'] })
    void queryClient.invalidateQueries({ queryKey: ['stats'] })
    void queryClient.invalidateQueries({ queryKey: ['integration-games'] })
  }

  const handleMutationSuccess = (updated: ManualReviewCandidateDetail) => {
    queryClient.setQueryData(['manual-review-candidate', updated.id], updated)
    invalidateReviewQueries(updated.id)
    const leavesCurrentScope =
      (scope === 'active' && updated.review_state !== 'pending') ||
      (scope === 'archive' && updated.review_state !== 'not_a_game')
    if (leavesCurrentScope && focusNextCandidateInScope(updated.id)) return
    selectCandidate(updated.id)
  }

  const applyMutation = useMutation({
    mutationFn: ({ candidateId, result }: { candidateId: string; result: ManualReviewSearchResult }) =>
      applyManualReviewCandidate(candidateId, result),
    onMutate: () => setRedetectNotice(null),
    onSuccess: handleMutationSuccess,
  })

  const notAGameMutation = useMutation({
    mutationFn: (candidateId: string) => markManualReviewCandidateNotAGame(candidateId),
    onMutate: () => setRedetectNotice(null),
    onSuccess: handleMutationSuccess,
  })

  const unarchiveMutation = useMutation({
    mutationFn: (candidateId: string) => unarchiveManualReviewCandidate(candidateId),
    onMutate: () => setRedetectNotice(null),
    onSuccess: handleMutationSuccess,
  })

  const redetectMutation = useMutation({
    mutationFn: (candidateId: string) => redetectManualReviewCandidate(candidateId),
    onMutate: () => setRedetectNotice(null),
    onSuccess: (response) => {
      const matchText =
        response.result.status === 'matched'
          ? `Re-detect matched this candidate with ${response.result.match_count} resolver match${response.result.match_count === 1 ? '' : 'es'}.`
          : response.result.status === 'pending'
            ? `Re-detect found ${response.result.match_count} resolver match${response.result.match_count === 1 ? '' : 'es'}, but the candidate still needs review.`
            : `Re-detect finished with no automatic match from ${response.result.provider_count} provider${response.result.provider_count === 1 ? '' : 's'}.`
      setRedetectNotice(matchText)
      queryClient.setQueryData(['manual-review-candidate', response.candidate.id], response.candidate)
      invalidateReviewQueries(response.candidate.id)
      if (scope === 'active' && response.result.status === 'matched' && focusNextCandidateInScope(response.candidate.id)) return
      selectCandidate(response.candidate.id)
    },
  })

  const batchRedetectMutation = useMutation({
    mutationFn: redetectActiveManualReviewCandidates,
    onMutate: () => setRedetectNotice(null),
    onSuccess: (result) => {
      setRedetectNotice(
        `Re-detect checked ${result.attempted} candidate${result.attempted === 1 ? '' : 's'}: ${result.matched} matched, ${result.unidentified} unchanged.`,
      )
      invalidateReviewQueries(activeCandidateId || undefined)
    },
  })

  const deleteFilesMutation = useMutation({
    mutationFn: (candidateId: string) => deleteManualReviewCandidateFiles(candidateId),
    onMutate: () => setRedetectNotice(null),
    onSuccess: (result) => {
      const deletedCandidateId = result.deleted_candidate_id
      invalidateReviewQueries(deletedCandidateId)
      void queryClient.removeQueries({ queryKey: ['manual-review-candidate', deletedCandidateId], exact: true })
      if (focusNextCandidateInScope(deletedCandidateId)) return
      const next = new URLSearchParams(searchParams)
      next.set('tab', 'undetected')
      next.set('scope', scope)
      next.delete('candidate_id')
      setSearchParams(next, { replace: true })
    },
  })

  useEffect(() => {
    if (activeCandidateId || !candidatesQuery.data || candidatesQuery.data.length === 0) return
    const nextCandidate = selectInitialCandidate(candidatesQuery.data, {
      canonicalGameId: scope === 'active' ? legacyGameId : null,
      title: scope === 'active' ? legacyTitle : null,
      platform: scope === 'active' ? legacyPlatform : null,
      source: scope === 'active' ? legacySource : null,
    })
    if (!nextCandidate) return
    selectCandidate(nextCandidate.id)
  }, [activeCandidateId, candidatesQuery.data, legacyGameId, legacyPlatform, legacySource, legacyTitle, scope])

  useEffect(() => {
    const nextSeed = preferredSearchQuery(candidateQuery.data)
    if (!candidateQuery.data || seededCandidateId === candidateQuery.data.id) return
    setSearchQuery(nextSeed)
    setSubmittedQuery(nextSeed)
    setSeededCandidateId(candidateQuery.data.id)
  }, [candidateQuery.data, seededCandidateId])

  const activeManualMatch = safeList(candidateQuery.data?.resolver_matches).find((match) => match.manual_selection)
  const canRedetectSelected = scope === 'active' && candidateQuery.data?.review_state === 'pending'
  const mutationError =
    applyMutation.error ??
    notAGameMutation.error ??
    unarchiveMutation.error ??
    redetectMutation.error ??
    batchRedetectMutation.error ??
    deleteFilesMutation.error
  const applyBusyKey = applyMutation.isPending && applyMutation.variables
    ? `${applyMutation.variables.result.provider_plugin_id}:${applyMutation.variables.result.external_id}`
    : null
  const candidateTitle =
    (candidateQuery.data && (safeText(candidateQuery.data.current_title) || safeText(candidateQuery.data.raw_title))) ||
    'Unknown title'
  const candidateIntegrationLabel =
    (candidateQuery.data &&
      (safeText(candidateQuery.data.integration_label) || safeText(candidateQuery.data.integration_id))) ||
    ''
  const candidateReviewReasons = safeList(candidateQuery.data?.review_reasons)
  const candidateFiles = safeList(candidateQuery.data?.files)
  const candidateResolverMatches = safeList(candidateQuery.data?.resolver_matches)

  const submitSearch = () => {
    const nextQuery = searchQuery.trim()
    if (!nextQuery) return
    setSubmittedQuery(nextQuery)
  }

  return (
    <div className="space-y-6">
      <section className="rounded-mga border border-mga-accent/30 bg-mga-accent/10 p-4">
        <div className="flex flex-wrap items-start justify-between gap-3">
          <div className="space-y-2">
            <div className="flex items-center gap-2 text-mga-accent">
              <ArrowRightLeft size={16} />
              <span className="text-sm font-semibold uppercase tracking-wide">Undetected Games</span>
            </div>
            <div>
              <h2 className="text-lg font-semibold text-mga-text">Manual review workflow</h2>
              <p className="mt-1 max-w-3xl text-sm leading-6 text-mga-muted">
                Review unresolved source records inline, apply metadata matches, archive not-a-game items, and reopen archived decisions.
              </p>
            </div>
          </div>
          <div className="flex flex-wrap gap-2">
            <Button type="button" size="sm" variant={scope === 'active' ? 'default' : 'outline'} onClick={() => setScope('active')}>
              Active Queue
            </Button>
            <Button type="button" size="sm" variant={scope === 'archive' ? 'default' : 'outline'} onClick={() => setScope('archive')}>
              Archive
            </Button>
            {scope === 'active' ? (
              <Button
                type="button"
                size="sm"
                variant="outline"
                onClick={() => batchRedetectMutation.mutate()}
                disabled={batchRedetectMutation.isPending || (candidatesQuery.data ?? []).length === 0}
              >
                {batchRedetectMutation.isPending ? <Loader2 size={16} className="animate-spin" /> : <RefreshCw size={16} />}
                Re-Detect Active Queue
              </Button>
            ) : null}
            {legacyGameId && legacyTitle && scope === 'active' ? (
              <>
                <Badge variant="muted">{legacyGameId}</Badge>
                {legacyPlatform ? <Badge variant="platform"><PlatformIcon platform={legacyPlatform} showLabel /></Badge> : null}
                {legacySource ? <BrandBadge brand={legacySource} label={brandLabel(legacySource, pluginLabel(legacySource))} /> : null}
              </>
            ) : null}
          </div>
        </div>
      </section>

      <section className="grid gap-6 xl:grid-cols-[minmax(320px,360px)_1fr]">
        <div className="space-y-4 rounded-mga border border-mga-border bg-mga-surface p-4">
          <div className="flex items-start justify-between gap-3">
            <div>
              <h3 className="text-sm font-semibold uppercase tracking-wide text-mga-text">
                {scope === 'archive' ? 'Archived Candidates' : 'Review Candidates'}
              </h3>
              <p className="mt-1 text-sm text-mga-muted">
                {scope === 'archive'
                  ? 'Archived not-a-game decisions stay here until you reopen them.'
                  : 'Applying a match removes a record from the active review queue.'}
              </p>
            </div>
            <Badge variant="muted">{(candidatesQuery.data ?? []).length}</Badge>
          </div>

          <Input
            value={candidateFilter}
            onChange={(event) => setCandidateFilter(event.target.value)}
            placeholder="Filter by title, source, platform, or path"
          />

          <div className="space-y-3">
            {candidatesQuery.isPending ? (
              <div className="rounded-mga border border-dashed border-mga-border p-4 text-sm text-mga-muted">
                Loading review candidates...
              </div>
            ) : null}

            {candidatesQuery.isError ? (
              <div className="rounded-mga border border-red-500/30 bg-red-500/10 p-4 text-sm text-red-300">
                {candidatesQuery.error instanceof ApiError
                  ? candidatesQuery.error.responseText || candidatesQuery.error.message
                  : 'Failed to load review candidates.'}
              </div>
            ) : null}

            {!candidatesQuery.isPending && !candidatesQuery.isError && filteredCandidates.length === 0 ? (
              <div className="rounded-mga border border-dashed border-mga-border p-4 text-sm text-mga-muted">
                {(candidatesQuery.data ?? []).length === 0
                  ? scope === 'archive'
                    ? 'No archived not-a-game items were found.'
                    : 'No active manual-review candidates were generated from the current queue heuristics.'
                  : 'No candidates match the current filter.'}
              </div>
            ) : null}

            {filteredCandidates.map((candidate) => {
              const isActive = candidate.id === activeCandidateId
              const currentTitle = safeText(candidate.current_title) || safeText(candidate.raw_title) || 'Unknown title'
              const rawTitle = safeText(candidate.raw_title)
              const reviewReasons = safeList(candidate.review_reasons)
              return (
                <button
                  key={candidate.id}
                  type="button"
                  onClick={() => selectCandidate(candidate.id)}
                  className={`w-full rounded-mga border p-3 text-left transition-colors ${
                    isActive
                      ? 'border-mga-accent bg-mga-accent/10'
                      : 'border-mga-border bg-mga-bg hover:bg-mga-elevated'
                  }`}
                >
                  <div className="flex items-start justify-between gap-3">
                    <div className="space-y-2">
                      <div>
                        <p className="font-semibold text-mga-text">{currentTitle}</p>
                        {rawTitle && currentTitle !== rawTitle ? (
                          <p className="text-xs text-mga-muted">Raw title: {rawTitle}</p>
                        ) : null}
                      </div>
                      <div className="flex flex-wrap gap-2">
                        <Badge variant="platform">
                          <PlatformIcon platform={candidate.platform} showLabel />
                        </Badge>
                        <BrandBadge
                          brand={candidate.plugin_id}
                          label={brandLabel(candidate.plugin_id, pluginLabel(candidate.plugin_id))}
                        />
                        {candidate.integration_label ? <Badge variant="muted">{candidate.integration_label}</Badge> : null}
                        <Badge variant={reviewStateVariant(candidate.review_state)}>{reviewStateLabel(candidate.review_state)}</Badge>
                      </div>
                    </div>

                    <div className="text-right text-xs text-mga-muted">
                      <div>{candidate.file_count} files</div>
                      <div>{candidate.resolver_match_count} resolver matches</div>
                    </div>
                  </div>

                  <div className="mt-3 flex flex-wrap gap-2">
                    {reviewReasons.length > 0 ? (
                      reviewReasons.map((reason) => (
                        <Badge key={`${candidate.id}:${reason}`} variant="accent">
                          {reviewReasonLabel(reason)}
                        </Badge>
                      ))
                    ) : (
                      <Badge variant="muted">{candidate.review_state === 'not_a_game' ? 'Archived' : 'Direct entry'}</Badge>
                    )}
                  </div>
                </button>
              )
            })}
          </div>
        </div>

        <div className="space-y-4">
          {!activeCandidateId ? (
            <EmptyState
              title={scope === 'archive' ? 'Choose an archived item' : 'Choose a review candidate'}
              description={
                scope === 'archive'
                  ? 'Select an archived record to inspect it or move it back into the active review queue.'
                  : 'Select a candidate from the left to inspect source evidence, search metadata providers, and apply a match.'
              }
            />
          ) : candidateQuery.isPending ? (
            <EmptyState title="Loading candidate" description="Pulling source evidence for the selected record." />
          ) : candidateQuery.isError ? (
            <ErrorState
              message={
                candidateQuery.error instanceof ApiError
                  ? candidateQuery.error.responseText || candidateQuery.error.message
                  : 'Failed to load the selected review candidate.'
              }
            />
          ) : !candidateQuery.data ? (
            <ErrorState message="The selected review candidate could not be found." />
          ) : (
            <>
              {mutationError ? (
                <ErrorState message={mutationErrorMessage(mutationError, 'Manual review update failed.')} />
              ) : null}

              {redetectNotice ? (
                <div className="rounded-mga border border-mga-accent/30 bg-mga-accent/10 p-4 text-sm text-mga-text">
                  {redetectNotice}
                </div>
              ) : null}

              <section className="rounded-mga border border-mga-border bg-mga-surface p-4">
                <div className="flex flex-wrap items-start justify-between gap-4">
                  <div className="space-y-3">
                    <div className="flex flex-wrap items-center gap-2">
                      <h3 className="text-xl font-semibold text-mga-text">{candidateTitle}</h3>
                      <Badge variant={reviewStateVariant(candidateQuery.data.review_state)}>
                        {reviewStateLabel(candidateQuery.data.review_state)}
                      </Badge>
                      {!selectedInScope ? <Badge variant="muted">Direct reclassify target</Badge> : null}
                    </div>
                    <p className="text-sm text-mga-muted">
                      {candidateIntegrationLabel}
                      {' · '}
                      {brandLabel(candidateQuery.data.plugin_id, pluginLabel(candidateQuery.data.plugin_id))}
                    </p>
                    <div className="flex flex-wrap gap-2">
                      <Badge variant="platform">
                        <PlatformIcon platform={candidateQuery.data.platform} showLabel />
                      </Badge>
                      <Badge variant="muted">{humanizeValue(candidateQuery.data.kind)}</Badge>
                      <Badge variant="muted">{humanizeValue(candidateQuery.data.group_kind)}</Badge>
                      <Badge variant="muted">{candidateQuery.data.file_count} files</Badge>
                      <Badge variant="muted">{candidateQuery.data.resolver_match_count} resolver matches</Badge>
                    </div>
                    <div className="flex flex-wrap gap-2">
                      {candidateReviewReasons.length > 0 ? (
                        candidateReviewReasons.map((reason) => (
                          <Badge key={`${candidateQuery.data.id}:${reason}`} variant="accent">
                            {reviewReasonLabel(reason)}
                          </Badge>
                        ))
                      ) : (
                        <Badge variant="muted">
                          {candidateQuery.data.review_state === 'matched'
                            ? 'Manual decision already applied'
                            : candidateQuery.data.review_state === 'not_a_game'
                              ? 'Archived from review'
                              : 'Opened from direct reclassify'}
                        </Badge>
                      )}
                    </div>
                  </div>

                  <div className="space-y-2 text-sm text-mga-muted">
                    <div>
                      <span className="font-medium text-mga-text">Candidate ID:</span> {candidateQuery.data.id}
                    </div>
                    <div>
                      <span className="font-medium text-mga-text">Created:</span> {formatDateTime(candidateQuery.data.created_at)}
                    </div>
                    <div>
                      <span className="font-medium text-mga-text">Last seen:</span> {formatDateTime(candidateQuery.data.last_seen_at)}
                    </div>
                    {candidateQuery.data.root_path ? (
                      <div className="max-w-xl break-all">
                        <span className="font-medium text-mga-text">Root path:</span> {candidateQuery.data.root_path}
                      </div>
                    ) : null}
                    {candidateQuery.data.url ? (
                      <a
                        href={candidateQuery.data.url}
                        target="_blank"
                        rel="noreferrer"
                        className="inline-flex items-center gap-1 text-mga-accent hover:underline"
                      >
                        Open source record
                        <ExternalLink size={14} />
                      </a>
                    ) : null}
                  </div>
                </div>

                <div className="mt-4 flex flex-wrap gap-3">
                  {candidateQuery.data.canonical_game_id ? (
                    <Button type="button" variant="outline" onClick={() => navigate(`/game/${encodeURIComponent(candidateQuery.data.canonical_game_id ?? '')}`)}>
                      Open Game
                    </Button>
                  ) : null}
                  {canRedetectSelected ? (
                    <Button
                      type="button"
                      variant="outline"
                      onClick={() => redetectMutation.mutate(candidateQuery.data.id)}
                      disabled={redetectMutation.isPending}
                    >
                      {redetectMutation.isPending ? <Loader2 size={16} className="animate-spin" /> : <RefreshCw size={16} />}
                      Try Re-Detect
                    </Button>
                  ) : null}
                  {candidateQuery.data.review_state === 'not_a_game' ? (
                    <Button
                      type="button"
                      variant="outline"
                      onClick={() => unarchiveMutation.mutate(candidateQuery.data.id)}
                      disabled={unarchiveMutation.isPending}
                    >
                      {unarchiveMutation.isPending ? <Loader2 size={16} className="animate-spin" /> : null}
                      Unarchive
                    </Button>
                  ) : (
                    <Button
                      type="button"
                      variant="outline"
                      onClick={() => notAGameMutation.mutate(candidateQuery.data.id)}
                      disabled={notAGameMutation.isPending}
                    >
                      {notAGameMutation.isPending ? <Loader2 size={16} className="animate-spin" /> : null}
                      Mark as Not a Game
                    </Button>
                  )}
                  <Button
                    type="button"
                    variant="outline"
                    className="border-red-500/40 bg-red-500/10 text-red-300 hover:bg-red-500/20"
                    onClick={() => setDeleteDialogOpen(true)}
                    disabled={candidateFiles.length === 0 || deleteFilesMutation.isPending}
                    title={candidateFiles.length === 0 ? 'No source files were recorded for this candidate.' : 'Delete candidate files'}
                  >
                    {deleteFilesMutation.isPending ? <Loader2 size={16} className="animate-spin" /> : <Trash2 size={16} />}
                    Delete Candidate Files
                  </Button>
                </div>
              </section>

              <section className="rounded-mga border border-mga-border bg-mga-surface p-4">
                {candidateQuery.data.review_state === 'not_a_game' ? (
                  <div className="rounded-mga border border-dashed border-mga-border p-4 text-sm text-mga-muted">
                    Archived records stay out of the active queue. Unarchive this item to search providers or apply a metadata match again.
                  </div>
                ) : (
                  <>
                    <div className="flex flex-wrap items-end gap-3">
                      <div className="min-w-[240px] flex-1">
                        <Input
                          label="Metadata search"
                          value={searchQuery}
                          onChange={(event) => setSearchQuery(event.target.value)}
                          placeholder="Search configured metadata providers"
                        />
                      </div>
                      <Button
                        type="button"
                        variant="outline"
                        onClick={submitSearch}
                        disabled={searchQuery.trim().length === 0 || searchResultsQuery.isFetching}
                      >
                        {searchResultsQuery.isFetching ? <Loader2 size={16} className="animate-spin" /> : <Search size={16} />}
                        Search Providers
                      </Button>
                    </div>

                    {activeManualMatch ? (
                      <div className="mt-4 flex flex-wrap items-center gap-2 text-sm text-mga-muted">
                        <Badge variant="muted">Selected match</Badge>
                        <span>
                          {brandLabel(activeManualMatch.plugin_id, pluginLabel(activeManualMatch.plugin_id))}
                          {' · '}
                          {activeManualMatch.title || activeManualMatch.external_id}
                        </span>
                      </div>
                    ) : null}

                    <div className="mt-4 flex flex-wrap gap-2">
                      {(searchResultsQuery.data?.providers ?? []).map((provider) => (
                        <Badge key={`${provider.integration_id}:${provider.plugin_id}`} variant={statusTone(provider.status)}>
                          {provider.integration_label || provider.integration_id}: {humanizeValue(provider.status)}
                          {provider.result_count > 0 ? ` (${provider.result_count})` : ''}
                        </Badge>
                      ))}
                    </div>

                    {searchResultsQuery.isError ? (
                      <div className="mt-4 rounded-mga border border-red-500/30 bg-red-500/10 p-4 text-sm text-red-300">
                        {searchResultsQuery.error instanceof ApiError
                          ? searchResultsQuery.error.responseText || searchResultsQuery.error.message
                          : 'Metadata search failed.'}
                      </div>
                    ) : null}

                    {searchResultsQuery.isFetching ? (
                      <div className="mt-4 flex items-center gap-2 text-sm text-mga-muted">
                        <Loader2 size={16} className="animate-spin" />
                        Searching configured metadata providers...
                      </div>
                    ) : null}

                    {!searchResultsQuery.isFetching &&
                    !searchResultsQuery.isError &&
                    searchResultsQuery.data &&
                    safeList(searchResultsQuery.data.results).length === 0 ? (
                      <div className="mt-4 rounded-mga border border-dashed border-mga-border p-4 text-sm text-mga-muted">
                        No metadata matches were returned for “{searchResultsQuery.data.query}”.
                      </div>
                    ) : null}

                    <div className="mt-4 grid gap-4 xl:grid-cols-2">
                      {(searchResultsQuery.data?.results ?? []).map((result) => {
                        const isSelected =
                          activeManualMatch?.plugin_id === result.provider_plugin_id &&
                          activeManualMatch?.external_id === result.external_id
                        const resultKey = `${result.provider_plugin_id}:${result.external_id}`
                        return (
                          <article
                            key={`${result.provider_integration_id}:${result.provider_plugin_id}:${result.external_id}`}
                            className="overflow-hidden rounded-mga border border-mga-border bg-mga-bg"
                          >
                            <div className="grid gap-4 p-4 sm:grid-cols-[120px_1fr]">
                              <div className="w-full max-w-[120px]">
                                <CoverImage src={result.image_url ?? null} alt={result.title || result.external_id} fit="contain" variant="compact" />
                              </div>

                              <div className="space-y-3">
                                <div className="flex flex-wrap items-start justify-between gap-2">
                                  <div>
                                    <h4 className="text-base font-semibold text-mga-text">{result.title || result.external_id}</h4>
                                    <p className="text-sm text-mga-muted">
                                      {result.provider_label || result.provider_integration_id}
                                      {' · '}
                                      {brandLabel(result.provider_plugin_id, pluginLabel(result.provider_plugin_id))}
                                    </p>
                                  </div>
                                  {result.url ? (
                                    <a
                                      href={result.url}
                                      target="_blank"
                                      rel="noreferrer"
                                      className="inline-flex items-center gap-1 text-sm text-mga-accent hover:underline"
                                    >
                                      Open
                                      <ExternalLink size={14} />
                                    </a>
                                  ) : null}
                                </div>

                                <div className="flex flex-wrap gap-2">
                                  {result.platform ? <Badge variant="platform"><PlatformIcon platform={result.platform} showLabel /></Badge> : null}
                                  {result.release_date ? <Badge variant="muted">{result.release_date}</Badge> : null}
                                  {result.developer ? <Badge variant="muted">{result.developer}</Badge> : null}
                                  {result.publisher ? <Badge variant="muted">{result.publisher}</Badge> : null}
                                  {isSelected ? <Badge variant="accent">Current selection</Badge> : null}
                                </div>

                                {result.description ? <p className="text-sm leading-6 text-mga-muted">{result.description}</p> : null}
                                {result.genres?.length ? (
                                  <div className="flex flex-wrap gap-2">
                                    {result.genres.map((genre) => <Badge key={`${result.external_id}:${genre}`} variant="accent">{genre}</Badge>)}
                                  </div>
                                ) : null}

                                <Button
                                  type="button"
                                  size="sm"
                                  onClick={() => applyMutation.mutate({ candidateId: candidateQuery.data.id, result })}
                                  disabled={applyMutation.isPending}
                                >
                                  {applyBusyKey === resultKey ? <Loader2 size={16} className="animate-spin" /> : null}
                                  {isSelected ? 'Reapply Match' : 'Use This Match'}
                                </Button>
                              </div>
                            </div>
                          </article>
                        )
                      })}
                    </div>
                  </>
                )}
              </section>

              <section className="grid gap-4 lg:grid-cols-2">
                <div className="rounded-mga border border-mga-border bg-mga-surface p-4">
                  <div className="flex items-center gap-2">
                    <FileSearch size={16} className="text-mga-accent" />
                    <h4 className="text-sm font-semibold uppercase tracking-wide text-mga-text">Source Files</h4>
                  </div>
                  <div className="mt-4 space-y-3">
                    {candidateFiles.length === 0 ? (
                      <p className="text-sm text-mga-muted">No source files were recorded for this candidate.</p>
                    ) : (
                      candidateFiles.map((file) => (
                        <div key={file.id} className="rounded-mga border border-mga-border bg-mga-bg p-3">
                          <div className="flex flex-wrap items-start justify-between gap-3">
                            <div>
                              <p className="break-all text-sm font-medium text-mga-text">{file.path}</p>
                              <p className="mt-1 text-xs text-mga-muted">
                                {humanizeValue(file.role)}
                                {file.file_kind ? ` · ${humanizeValue(file.file_kind)}` : ''}
                              </p>
                            </div>
                            <Badge variant="muted">{formatBytes(file.size)}</Badge>
                          </div>
                        </div>
                      ))
                    )}
                  </div>
                </div>

                <div className="rounded-mga border border-mga-border bg-mga-surface p-4">
                  <div className="flex items-center gap-2">
                    <AlertCircle size={16} className="text-mga-accent" />
                    <h4 className="text-sm font-semibold uppercase tracking-wide text-mga-text">Existing Resolver Matches</h4>
                  </div>
                  <div className="mt-4 space-y-3">
                    {candidateResolverMatches.length === 0 ? (
                      <p className="text-sm text-mga-muted">No resolver matches are currently attached to this candidate.</p>
                    ) : (
                      candidateResolverMatches.map((match) => (
                        <div key={`${match.plugin_id}:${match.external_id}`} className="rounded-mga border border-mga-border bg-mga-bg p-3">
                          <div className="flex flex-wrap items-start justify-between gap-3">
                            <div>
                              <p className="text-sm font-medium text-mga-text">{match.title || safeText(candidateQuery.data.raw_title) || candidateTitle}</p>
                              <p className="mt-1 text-xs text-mga-muted">
                                {brandLabel(match.plugin_id, pluginLabel(match.plugin_id))}
                                {' · '}
                                {match.external_id}
                              </p>
                            </div>
                            <div className="flex flex-wrap gap-2">
                              {match.manual_selection ? <Badge variant="accent">Manual</Badge> : null}
                              {match.outvoted ? <Badge variant="muted">Outvoted</Badge> : <Badge variant="accent">Active</Badge>}
                            </div>
                          </div>
                          <div className="mt-3 flex flex-wrap gap-2">
                            {match.platform ? <Badge variant="platform"><PlatformIcon platform={match.platform} showLabel /></Badge> : null}
                            {match.release_date ? <Badge variant="muted">{match.release_date}</Badge> : null}
                            {match.developer ? <Badge variant="muted">{match.developer}</Badge> : null}
                            {match.publisher ? <Badge variant="muted">{match.publisher}</Badge> : null}
                          </div>
                        </div>
                      ))
                    )}
                  </div>
                </div>
              </section>
            </>
          )}
        </div>
      </section>
      <Dialog
        open={deleteDialogOpen}
        onClose={() => !deleteFilesMutation.isPending && setDeleteDialogOpen(false)}
        title="Delete Candidate Files"
      >
        <div className="space-y-4">
          <div className="rounded-mga border border-red-500/30 bg-red-500/10 p-4 text-sm leading-6 text-red-100">
            This permanently deletes the source files recorded for <span className="font-semibold">{candidateTitle}</span> and removes this
            candidate from MGA. This is different from marking it as not a game, which only archives the record.
          </div>
          <div className="max-h-48 space-y-2 overflow-auto rounded-mga border border-mga-border bg-mga-bg p-3">
            {candidateFiles.length === 0 ? (
              <p className="text-sm text-mga-muted">No source files were recorded for this candidate.</p>
            ) : (
              candidateFiles.map((file) => (
                <div key={`${file.path}:${file.role}`} className="flex items-start justify-between gap-3 text-sm">
                  <span className="break-all text-mga-text">{file.path}</span>
                  <span className="shrink-0 text-mga-muted">{formatBytes(file.size)}</span>
                </div>
              ))
            )}
          </div>
          <div className="flex justify-end gap-3">
            <Button
              type="button"
              variant="outline"
              onClick={() => setDeleteDialogOpen(false)}
              disabled={deleteFilesMutation.isPending}
            >
              Cancel
            </Button>
            <Button
              type="button"
              className="bg-red-600 text-white hover:bg-red-500"
              onClick={() => {
                if (!candidateQuery.data) return
                deleteFilesMutation.mutate(candidateQuery.data.id, {
                  onSuccess: () => setDeleteDialogOpen(false),
                })
              }}
              disabled={!candidateQuery.data || candidateFiles.length === 0 || deleteFilesMutation.isPending}
            >
              {deleteFilesMutation.isPending ? <Loader2 size={16} className="animate-spin" /> : <Trash2 size={16} />}
              Delete Files
            </Button>
          </div>
        </div>
      </Dialog>
    </div>
  )
}
