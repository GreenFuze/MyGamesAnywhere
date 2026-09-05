import { useQuery } from '@tanstack/react-query'
import { Clock3, Database, RefreshCw } from 'lucide-react'
import { listCatalogOffers } from '@/api/client'
import { MetricCard, PageIntro, QueryFeedback, SectionCard, StatusPill, formatCount, formatDate } from '@/components/management/ManagementPrimitives'

export function CatalogPage() {
  const catalog = useQuery({ queryKey: ['management', 'catalog-offers'], queryFn: listCatalogOffers })
  const offers = catalog.data ?? []
  const leaving = offers.filter((offer) => offer.availability === 'leaving_soon').length
  const stale = offers.filter((offer) => offer.stale_at).length
  return <div className="mga-page-enter space-y-7">
    <PageIntro eyebrow="Availability" title="What you can play" description="What you can play right now, what is about to leave, and what changed since last time." />
    <div className="grid gap-4 sm:grid-cols-3">
      <MetricCard label="Games" value={formatCount(offers.length)} detail="Everything your sources are offering" icon={<Database className="h-4 w-4" />} />
      <MetricCard label="Leaving soon" value={formatCount(leaving)} detail="Leaving soon, or no longer available" tone={leaving ? 'attention' : 'good'} icon={<Clock3 className="h-4 w-4" />} />
      <MetricCard label="Not checked lately" value={formatCount(stale)} detail="Not checked recently, so this may be out of date" tone={stale ? 'attention' : 'good'} icon={<RefreshCw className="h-4 w-4" />} />
    </div>
    <SectionCard title="Games and how you can play them" description="Where each game comes from, how you have access to it, and when that was last confirmed.">
      <QueryFeedback pending={catalog.isPending} error={catalog.error} empty={!catalog.isPending && offers.length === 0} emptyTitle="Nothing here yet" emptyDescription="Connect a store or subscription source, and what it offers will be tracked here over time." />
      {offers.length > 0 && <div className="grid gap-3 xl:grid-cols-2">{offers.slice(0, 40).map((offer) => <article key={offer.id} className="rounded-lg border border-mga-border bg-mga-elevated/40 p-4">
        <div className="flex items-start justify-between gap-4"><div><p className="text-sm font-semibold capitalize text-mga-text">{offer.provider}</p><p className="mt-1 text-xs text-mga-muted">{offer.platform} · {offer.region} · {offer.sku}</p></div><StatusPill label={offer.availability.replace('_', ' ')} tone={offer.availability === 'available' ? 'good' : offer.availability === 'leaving_soon' ? 'attention' : offer.availability === 'unavailable' ? 'danger' : 'neutral'} /></div>
        <div className="mt-4 grid grid-cols-2 gap-3 text-xs"><div><span className="text-mga-muted">Entitlement</span><p className="mt-1 capitalize text-mga-text">{offer.entitlement}</p></div><div><span className="text-mga-muted">Delivery</span><p className="mt-1 capitalize text-mga-text">{offer.delivery.replace('_', ' ')}</p></div><div><span className="text-mga-muted">Current version</span><p className="mt-1 text-mga-text">{offer.current_version?.version || offer.current_version?.build_id || 'Not reported'}</p></div><div><span className="text-mga-muted">Observed</span><p className="mt-1 text-mga-text">{formatDate(offer.observed_at)}</p></div></div>
      </article>)}</div>}
    </SectionCard>
  </div>
}
