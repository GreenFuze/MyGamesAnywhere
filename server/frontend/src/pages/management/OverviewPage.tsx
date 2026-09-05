import { useQuery } from '@tanstack/react-query'
import { AlertTriangle, Database, Gamepad2, PackageCheck, PlugZap } from 'lucide-react'
import { Link } from 'react-router'
import { getIntegrationStatus, getStats, listCatalogOffers, listRuntimeArtifacts } from '@/api/client'
import { MetricCard, PageIntro, QueryFeedback, SectionCard, StatusPill, formatCount, formatDate } from '@/components/management/ManagementPrimitives'

export function OverviewPage() {
  const stats = useQuery({ queryKey: ['management', 'stats'], queryFn: getStats })
  const sources = useQuery({ queryKey: ['management', 'source-status'], queryFn: getIntegrationStatus })
  const offers = useQuery({ queryKey: ['management', 'catalog-offers'], queryFn: listCatalogOffers })
  const artifacts = useQuery({ queryKey: ['management', 'runtime-artifacts'], queryFn: listRuntimeArtifacts })
  const pending = stats.isPending || sources.isPending || offers.isPending || artifacts.isPending
  const firstError = stats.error ?? sources.error ?? offers.error ?? artifacts.error
  const staleOffers = offers.data?.filter((offer) => offer.stale_at).length ?? 0
  const sourceProblems = sources.data?.filter((source) => source.status !== 'ok').length ?? 0
  const blockedArtifacts = artifacts.data?.filter((artifact) => artifact.compliance_state !== 'approved').length ?? 0

  return (
    <div className="mga-page-enter space-y-7">
      <PageIntro eyebrow="Overview" title="How everything is doing" />
      <QueryFeedback pending={pending} error={firstError} empty={false} emptyTitle="" emptyDescription="" />
      {!pending && !firstError && (
        <>
          <div className="grid gap-4 sm:grid-cols-2 xl:grid-cols-4">
            <MetricCard label="Games" value={formatCount(stats.data?.canonical_game_count)} detail={`Found across ${formatCount(sources.data?.length)} connected sources`} tone="good" icon={<Gamepad2 className="h-4 w-4" />} />
            <MetricCard label="Ways to play" value={formatCount(offers.data?.length)} detail={(offers.data?.length ?? 0) === 0 ? 'Nothing tracked yet' : staleOffers ? `${staleOffers} not checked recently` : 'All checked recently'} tone={staleOffers ? 'attention' : 'good'} icon={<Database className="h-4 w-4" />} />
            <MetricCard label="Connected sources" value={formatCount(sources.data?.length)} detail={sourceProblems ? `${sourceProblems} need attention` : 'All working normally'} tone={sourceProblems ? 'attention' : 'good'} icon={<PlugZap className="h-4 w-4" />} />
            <MetricCard label="Emulators and runtimes" value={formatCount(artifacts.data?.length)} detail={(artifacts.data?.length ?? 0) === 0 ? 'None added yet' : blockedArtifacts ? `${blockedArtifacts} cannot be sent yet` : 'All cleared to send'} tone={blockedArtifacts ? 'attention' : 'good'} icon={<PackageCheck className="h-4 w-4" />} />
          </div>

          <div className="grid gap-5 xl:grid-cols-[1.45fr_1fr]">
            <SectionCard title="Needs attention" description="Anything that could stop a connected app from seeing your games.">
              <div className="space-y-3">
                {sourceProblems + staleOffers + blockedArtifacts === 0 ? (
                  <div className="rounded-lg border border-emerald-400/20 bg-emerald-400/5 p-4">
                    <div className="flex items-center gap-2 text-sm font-semibold text-emerald-300"><PackageCheck className="h-4 w-4" /> Nothing needs your attention</div>
                    <p className="mt-1 text-xs leading-5 text-mga-muted">Your sources are working, and everything has been checked recently.</p>
                  </div>
                ) : null}
                {/* Naming the problem is the whole job of this card. Telling
                    someone "2 things to look at" and leaving them to find which
                    two is a worse answer than saying nothing. */}
                {(sources.data ?? []).filter((source) => source.status !== 'ok').map((source) => (
                  <AttentionItem
                    key={source.integration_id}
                    to="/sources"
                    title={source.label}
                    detail={sourceProblemText(source.status, source.message)}
                  />
                ))}
                {staleOffers > 0 && (
                  <AttentionItem
                    to="/catalog"
                    title={`${staleOffers} ${staleOffers === 1 ? 'game has' : 'games have'} not been checked recently`}
                    detail="What is shown for them may be out of date."
                  />
                )}
                {blockedArtifacts > 0 && (
                  <AttentionItem
                    to="/artifacts"
                    title={`${blockedArtifacts} ${blockedArtifacts === 1 ? 'emulator or runtime cannot' : 'emulators or runtimes cannot'} be sent`}
                    detail="Their licence or checksum has not been confirmed."
                  />
                )}
                {offers.data?.filter((offer) => offer.availability === 'leaving_soon').slice(0, 3).map((offer) => (
                  <div key={offer.id} className="flex items-center justify-between gap-4 rounded-lg border border-amber-400/20 bg-amber-400/5 p-3">
                    <div><p className="text-sm font-medium text-mga-text">{offer.provider} · {offer.platform}</p><p className="mt-1 text-xs text-mga-muted">Last checked {formatDate(offer.observed_at)}</p></div>
                    <StatusPill label="Leaving soon" tone="attention" />
                  </div>
                ))}
              </div>
            </SectionCard>

            <SectionCard title="Jump to" description="Where you probably want to go next.">
              <div className="grid grid-cols-2 gap-2 text-xs">
                <QuickLink to="/library" label="Browse your games" />
                <QuickLink to="/system" label="Manage app access" />
                <QuickLink to="/catalog" label="See what you can play" />
                <QuickLink to="/artifacts" label="Check emulators" />
              </div>
            </SectionCard>
          </div>
        </>
      )}
    </div>
  )
}

/** One named problem, and somewhere to go about it. */
function AttentionItem({ to, title, detail }: { to: string; title: string; detail: string }) {
  return (
    <Link to={to} className="flex items-center gap-3 rounded-lg border border-amber-400/20 bg-amber-400/5 p-4 transition hover:border-amber-400/40 focus:outline-none focus:ring-2 focus:ring-amber-400/40">
      <AlertTriangle className="h-5 w-5 shrink-0 text-amber-300" />
      <div className="min-w-0">
        <p className="truncate text-sm font-semibold text-mga-text">{title}</p>
        <p className="mt-1 text-xs leading-5 text-mga-muted">{detail}</p>
      </div>
    </Link>
  )
}

/** Says what is wrong in words the reader can act on. The provider's own
 *  message is kept when there is one, because "invalid API key" tells someone
 *  exactly what to go and fix, where "error" does not. */
function sourceProblemText(status: string, message?: string): string {
  if (status === 'oauth_required') return 'Sign in again to reconnect this source.'
  const detail = message?.trim()
  if (detail) return detail.charAt(0).toUpperCase() + detail.slice(1)
  return 'This source could not be reached.'
}

function QuickLink({ to, label }: { to: string; label: string }) {
  return <Link to={to} className="rounded-lg border border-mga-border bg-mga-elevated/60 px-3 py-2.5 text-mga-muted transition hover:border-mga-accent/30 hover:text-mga-text focus:outline-none focus:ring-2 focus:ring-mga-accent/40">{label} →</Link>
}
