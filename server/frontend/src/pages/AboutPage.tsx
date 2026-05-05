import { useQuery } from '@tanstack/react-query'
import { ExternalLink } from 'lucide-react'
import { getAboutInfo } from '@/api/client'
import { BrandMark } from '@/components/ui/brand-icon'
import { Skeleton } from '@/components/ui/skeleton'
import {
  getBrandDefinition,
  POWERED_BY_BRAND_IDS,
  SHIPPED_ICON_BRAND_IDS,
} from '@/lib/brands'

const poweredByBrands = POWERED_BY_BRAND_IDS
  .map((id) => getBrandDefinition(id))
  .filter((brand) => brand !== null)

const shippedIconBrands = SHIPPED_ICON_BRAND_IDS
  .map((id) => getBrandDefinition(id))
  .filter((brand) => brand !== null)

function formatBuildDate(value: string) {
  const date = new Date(value)
  if (Number.isNaN(date.getTime())) return value
  return date.toLocaleString([], {
    year: 'numeric',
    month: 'short',
    day: 'numeric',
    hour: '2-digit',
    minute: '2-digit',
  })
}

function InfoCard({ label, value, detail }: { label: string; value: string; detail?: string }) {
  return (
    <article className="rounded-mga border border-mga-border bg-mga-bg p-4">
      <p className="text-xs uppercase tracking-[0.18em] text-mga-muted">{label}</p>
      <p className="mt-2 break-all text-lg font-semibold text-mga-text">{value}</p>
      {detail ? <p className="mt-1 text-xs text-mga-muted">{detail}</p> : null}
    </article>
  )
}

function InfoCardSkeleton() {
  return (
    <article className="rounded-mga border border-mga-border bg-mga-bg p-4">
      <Skeleton className="h-3 w-20" />
      <Skeleton className="mt-3 h-6 w-32" />
      <Skeleton className="mt-2 h-3 w-24" />
    </article>
  )
}

function BrandCardSkeleton() {
  return (
    <article className="border-b border-mga-border/70 py-4 last:border-b-0">
      <div className="flex items-start justify-between gap-3">
        <div className="flex min-w-0 items-center gap-3">
          <Skeleton className="h-11 w-11 rounded-mga" />
          <div className="min-w-0 space-y-2">
            <Skeleton className="h-4 w-28" />
            <Skeleton className="h-3 w-48" />
          </div>
        </div>
        <Skeleton className="h-4 w-10" />
      </div>
    </article>
  )
}

function CreditRowSkeleton() {
  return (
    <article className="flex flex-col gap-3 border-b border-mga-border/70 py-4 last:border-b-0 md:flex-row md:items-center md:justify-between">
      <div className="flex min-w-0 items-center gap-3">
        <Skeleton className="h-11 w-11 rounded-mga" />
        <div className="min-w-0 space-y-2">
          <Skeleton className="h-4 w-24" />
          <Skeleton className="h-3 w-56" />
        </div>
      </div>
      <div className="space-y-2">
        <Skeleton className="h-3 w-24" />
        <Skeleton className="h-4 w-20" />
      </div>
    </article>
  )
}

export function AboutPage() {
  const aboutQuery = useQuery({
    queryKey: ['about'],
    queryFn: getAboutInfo,
  })

  const about = aboutQuery.data

  return (
    <div className="space-y-8">
      <section className="rounded-mga border border-mga-border bg-mga-surface p-5 shadow-sm shadow-black/10 md:p-6">
        <div className="flex flex-col gap-4 md:flex-row md:items-center">
          <img
            src="/logo.png"
            alt=""
            width={96}
            height={96}
            className="h-20 w-20 shrink-0 rounded-mga border border-mga-border bg-mga-elevated object-contain p-1"
          />
          <div className="space-y-2">
            <h1 className="text-2xl font-semibold tracking-tight text-mga-text md:text-3xl">
              MyGamesAnywhere
            </h1>
            <p className="max-w-3xl text-sm leading-7 text-mga-muted">
              Local-first game library server and embedded web client. The frontend surfaces source
              integrations, metadata providers, achievement providers, sync providers, and browser
              runtimes while keeping the server API generic and client-agnostic.
            </p>
          </div>
        </div>

        <div className="mt-5 overflow-hidden rounded-mga border border-mga-border bg-mga-bg">
          <img
            src="/title.png"
            alt="MyGamesAnywhere title"
            className="block w-full max-w-2xl"
          />
        </div>
      </section>

      <section className="rounded-mga border border-mga-border bg-mga-surface p-5 shadow-sm shadow-black/10 md:p-6">
        <div className="flex flex-wrap items-start justify-between gap-4">
          <div>
            <h2 className="text-lg font-semibold text-mga-text">Build Information</h2>
            <p className="mt-1 text-sm text-mga-muted">
              The server is the source of truth for app version metadata and author credits.
            </p>
          </div>
          <a
            href="/api/about/license"
            target="_blank"
            rel="noreferrer"
            className="inline-flex items-center gap-1 rounded-mga border border-mga-accent/30 bg-mga-accent/10 px-3 py-1.5 text-sm font-medium text-mga-accent hover:bg-mga-accent/20"
          >
            View Open Source Licenses
            <ExternalLink size={14} />
          </a>
        </div>

        {aboutQuery.isPending ? (
          <div className="mt-5 grid gap-3 md:grid-cols-2 xl:grid-cols-4">
            {Array.from({ length: 4 }, (_, index) => (
              <InfoCardSkeleton key={index} />
            ))}
          </div>
        ) : null}

        {aboutQuery.isError ? (
          <p className="mt-4 text-sm text-red-300">Failed to load build metadata: {aboutQuery.error.message}</p>
        ) : null}

        {about ? (
          <div className="mt-5 grid gap-3 md:grid-cols-2 xl:grid-cols-4">
            <InfoCard label="Version" value={about.version} />
            <InfoCard label="Commit" value={about.commit} />
            <InfoCard label="Built" value={formatBuildDate(about.build_date)} />
            <InfoCard
              label="Authors"
              value={about.author_credits.join(', ') || 'Unknown'}
              detail="Server-provided credits"
            />
          </div>
        ) : null}
      </section>

      <section className="space-y-3">
        <div>
          <h2 className="text-lg font-semibold text-mga-text">Powered By</h2>
          <p className="mt-1 text-sm text-mga-muted">
            External vendors and runtimes currently represented in the app through sources, metadata,
            achievements, sync, or embedded browser playback.
          </p>
        </div>

        <div className="divide-y divide-mga-border/70 rounded-mga border border-mga-border bg-mga-surface/50 px-5">
          {aboutQuery.isPending
            ? Array.from({ length: 6 }, (_, index) => <BrandCardSkeleton key={`brand-skeleton-${index}`} />)
            : null}
          {poweredByBrands.map((brand) => (
            <article
              key={brand.id}
              className="flex items-start justify-between gap-3 border-b border-mga-border/70 py-4 last:border-b-0"
            >
              <div className="flex min-w-0 items-center gap-3">
                <BrandMark brand={brand} tileClassName="border border-mga-border" />
                <div className="min-w-0">
                  <h3 className="font-semibold text-mga-text">{brand.label}</h3>
                  <p className="mt-1 text-sm text-mga-muted">{brand.description}</p>
                </div>
              </div>

              {brand.websiteUrl ? (
                <a
                  href={brand.websiteUrl}
                  target="_blank"
                  rel="noreferrer"
                  className="inline-flex shrink-0 items-center gap-1 text-sm font-medium text-mga-accent hover:underline"
                >
                  Visit
                  <ExternalLink size={14} />
                </a>
              ) : null}
            </article>
          ))}
        </div>
      </section>

      <section className="space-y-3">
        <div>
          <h2 className="text-lg font-semibold text-mga-text">Icon Credits</h2>
          <p className="mt-1 text-sm text-mga-muted">
            Shipped logos and platform marks have normalized filenames in the frontend, but the original
            source references are preserved here.
          </p>
        </div>

        <div className="divide-y divide-mga-border/70 rounded-mga border border-mga-border bg-mga-surface/50 px-5">
          {aboutQuery.isPending
            ? Array.from({ length: 4 }, (_, index) => <CreditRowSkeleton key={`credit-skeleton-${index}`} />)
            : null}
          {shippedIconBrands.map((brand) => (
            <article
              key={brand.id}
              className="flex flex-col gap-3 border-b border-mga-border/70 py-4 last:border-b-0 md:flex-row md:items-center md:justify-between"
            >
              <div className="flex min-w-0 items-center gap-3">
                <BrandMark brand={brand} tileClassName="border border-mga-border" />
                <div className="min-w-0">
                  <p className="font-medium text-mga-text">{brand.label}</p>
                  <p className="text-sm text-mga-muted">{brand.creditNote ?? 'Bundled icon asset.'}</p>
                </div>
              </div>

              <div className="text-sm text-mga-muted md:text-right">
                {brand.tempResourceName ? <p>Temp resource: {brand.tempResourceName}</p> : null}
                {brand.websiteUrl ? (
                  <a
                    href={brand.websiteUrl}
                    target="_blank"
                    rel="noreferrer"
                    className="inline-flex items-center gap-1 font-medium text-mga-accent hover:underline"
                  >
                    Vendor site
                    <ExternalLink size={14} />
                  </a>
                ) : null}
              </div>
            </article>
          ))}
        </div>
      </section>
    </div>
  )
}
