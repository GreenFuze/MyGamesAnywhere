import { ExternalLink } from 'lucide-react'
import { BrandIcon } from '@/components/ui/brand-icon'
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

export function AboutPage() {
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
              integrations, metadata providers, achievement providers, and platform/vendor marks while
              keeping the server API generic and client-agnostic.
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

      <section className="space-y-3">
        <div>
          <h2 className="text-lg font-semibold text-mga-text">Powered By</h2>
          <p className="mt-1 text-sm text-mga-muted">
            External vendors and services currently represented in the app through sources, metadata,
            achievements, sync, or launch targets.
          </p>
        </div>

        <div className="grid gap-4 md:grid-cols-2 xl:grid-cols-3">
          {poweredByBrands.map((brand) => (
            <article
              key={brand.id}
              className="rounded-mga border border-mga-border bg-mga-surface p-4 shadow-sm shadow-black/10"
            >
              <div className="flex items-start justify-between gap-3">
                <div className="flex min-w-0 items-center gap-3">
                  <div className="flex h-11 w-11 items-center justify-center rounded-mga border border-mga-border bg-mga-bg">
                    {brand.iconPath ? (
                      <BrandIcon brand={brand} className="h-7 w-7" />
                    ) : (
                      <span className="text-xs font-semibold uppercase text-mga-muted">
                        {brand.label.slice(0, 3)}
                      </span>
                    )}
                  </div>
                  <div className="min-w-0">
                    <h3 className="font-semibold text-mga-text">{brand.label}</h3>
                    <p className="mt-1 text-sm text-mga-muted">{brand.description}</p>
                  </div>
                </div>

                {brand.websiteUrl && (
                  <a
                    href={brand.websiteUrl}
                    target="_blank"
                    rel="noreferrer"
                    className="inline-flex shrink-0 items-center gap-1 text-sm font-medium text-mga-accent hover:underline"
                  >
                    Visit
                    <ExternalLink size={14} />
                  </a>
                )}
              </div>
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

        <div className="space-y-3">
          {shippedIconBrands.map((brand) => (
            <article
              key={brand.id}
              className="flex flex-col gap-3 rounded-mga border border-mga-border bg-mga-surface p-4 shadow-sm shadow-black/10 md:flex-row md:items-center md:justify-between"
            >
              <div className="flex min-w-0 items-center gap-3">
                <div className="flex h-11 w-11 shrink-0 items-center justify-center rounded-mga border border-mga-border bg-mga-bg">
                  <BrandIcon brand={brand} className="h-7 w-7" />
                </div>
                <div className="min-w-0">
                  <p className="font-medium text-mga-text">{brand.label}</p>
                  <p className="text-sm text-mga-muted">{brand.creditNote ?? 'Bundled icon asset.'}</p>
                </div>
              </div>

              <div className="text-sm text-mga-muted md:text-right">
                {brand.tempResourceName && <p>Temp resource: {brand.tempResourceName}</p>}
                {brand.websiteUrl && (
                  <a
                    href={brand.websiteUrl}
                    target="_blank"
                    rel="noreferrer"
                    className="inline-flex items-center gap-1 font-medium text-mga-accent hover:underline"
                  >
                    Vendor site
                    <ExternalLink size={14} />
                  </a>
                )}
              </div>
            </article>
          ))}
        </div>
      </section>
    </div>
  )
}
