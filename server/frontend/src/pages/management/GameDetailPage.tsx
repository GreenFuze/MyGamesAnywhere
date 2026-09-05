import { useMemo, useState } from 'react'
import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query'
import { Link, useParams } from 'react-router'
import { ArrowLeft, ImageUp, RotateCcw, Star } from 'lucide-react'
import {
  clearGameCoverOverride, clearGameFavorite, getGameDetail, listCatalogOffers,
  setGameCoverOverride, setGameFavorite,
  type CatalogOffer, type GameDetailResponse, type GameMediaDetailDTO, type SourceGameDetailDTO,
} from '@/api/client'
import { effectiveCover, effectiveCoverUrl, mediaUrl } from '@/lib/gameMedia'
import { GameMediaCollection } from '@/lib/gameMedia'
import { humanizeIdentifier, platformLabel, sourceLabel } from '@/lib/displayText'
import { availabilityLabel, describePlayability, entitlementLabel, isStale, offersForGame } from '@/lib/gameAvailability'
import { gameBadges } from '@/lib/gameBadges'
import { PageIntro, QueryFeedback, SectionCard, StatusPill, formatCount, formatDate } from '@/components/management/ManagementPrimitives'

/**
 * One game: where it came from, how you can play it, and what MGA knows.
 *
 * The Library was a grid of covers that opened nothing. This is the page they
 * open, and it is also the first place the entitlement and availability
 * recorded by MGA-118 are visible to anyone — until now they were written to
 * the database and read only by tests.
 *
 * There is no Play or Install control here, and that is not an omission.
 * ADR-0047 puts execution and installation in the frontend that owns the
 * device; MGA's job on this page is to say what is true.
 */
export function GameDetailPage() {
  const { id = '' } = useParams()

  const game = useQuery({
    queryKey: ['management', 'game', id],
    queryFn: () => getGameDetail(id),
    enabled: id !== '',
  })
  const offers = useQuery({ queryKey: ['management', 'catalog-offers'], queryFn: listCatalogOffers })

  const media = useMemo(() => new GameMediaCollection(game.data?.media), [game.data?.media])
  const gameOffers = useMemo(() => offersForGame(offers.data, id), [offers.data, id])
  // The gallery is everything that is not currently the cover, so the picture
  // on show is never also offered as a choice.
  const shownCover = game.data ? effectiveCover(game.data) : null
  const shownCoverUrl = game.data ? effectiveCoverUrl(game.data) : null
  const gallery = useMemo(
    () => media.imageMedia().filter((item) => item.asset_id !== shownCover?.asset_id),
    [media, shownCover],
  )

  const queryClient = useQueryClient()
  const refreshGame = () => {
    void queryClient.invalidateQueries({ queryKey: ['management', 'game', id] })
    // The library row shows the same cover and badges, so it hears about this
    // too. A row that lags behind the page you just changed is its own lie.
    void queryClient.invalidateQueries({ queryKey: ['management', 'library'] })
  }
  const favourite = useMutation({
    mutationFn: (next: boolean) => (next ? setGameFavorite(id) : clearGameFavorite(id)),
    onSuccess: refreshGame,
  })
  const cover = useMutation({
    mutationFn: (assetID: number | null) => (assetID === null ? clearGameCoverOverride(id) : setGameCoverOverride(id, assetID)),
    onSuccess: refreshGame,
  })

  return (
    <div className="mga-page-enter space-y-7">
      <Link
        to="/library"
        className="inline-flex items-center gap-1.5 text-xs text-mga-muted transition hover:text-mga-text"
      >
        <ArrowLeft className="h-3.5 w-3.5" />
        Back to your games
      </Link>

      <QueryFeedback
        pending={game.isPending}
        error={game.error}
        empty={false}
        emptyTitle=""
        emptyDescription=""
      />

      {game.data && (
        <>
          <div className="flex flex-col gap-5 sm:flex-row sm:items-start">
            {shownCoverUrl && (
              <img
                src={shownCoverUrl}
                alt=""
                className="w-40 shrink-0 rounded-lg border border-mga-border object-cover shadow-lg"
              />
            )}
            <div className="min-w-0 flex-1">
              <PageIntro
                eyebrow={platformLabel(game.data.platform || 'unknown')}
                title={game.data.title}
                description={game.data.description || undefined}
              />
              {/* The same badges as the library row, so what someone scanned a
                  list for is still there when they open it. */}
              <div className="mt-3 flex flex-wrap items-center gap-1.5">
                {gameBadges(game.data, offers.data).map((badge) => (
                  <span key={badge.id} title={badge.title}>
                    <StatusPill label={badge.label} tone={badge.tone} />
                  </span>
                ))}
              </div>

              <button
                type="button"
                onClick={() => favourite.mutate(!game.data.favorite)}
                disabled={favourite.isPending}
                aria-pressed={game.data.favorite}
                className={`mt-4 inline-flex items-center gap-1.5 rounded-md border px-3 py-1.5 text-xs transition disabled:opacity-60 ${
                  game.data.favorite
                    ? 'border-amber-400/40 bg-amber-400/10 text-amber-200'
                    : 'border-mga-border text-mga-muted hover:text-mga-text'
                }`}
              >
                <Star className={`h-3.5 w-3.5 ${game.data.favorite ? 'fill-current' : ''}`} />
                {game.data.favorite ? 'A favourite' : 'Make a favourite'}
              </button>
              {favourite.error && (
                <p className="mt-2 text-xs text-rose-300" role="alert">
                  That could not be saved: {favourite.error instanceof Error ? favourite.error.message : 'the server refused it.'}
                </p>
              )}
              <Facts game={game.data} />
            </div>
          </div>

          <SectionCard
            title="How you can play this"
            description={gameOffers.length > 0 ? undefined : 'Nothing has been recorded for this game yet.'}
          >
            {gameOffers.length === 0 ? (
              <p className="text-xs leading-5 text-mga-muted">
                A source has to report on this game before MGA can say whether you own it, whether it is in a
                subscription, or whether it has gone away. Scanning a store or subscription source records that.
              </p>
            ) : (
              <div className="space-y-3">
                {gameOffers.map((offer) => <OfferRow key={offer.id} offer={offer} />)}
              </div>
            )}
          </SectionCard>

          {gallery.length > 0 && (
            <SectionCard
              title="Pictures"
              description={`${gallery.length} beyond the cover. Any of them can become the cover.`}
            >
              <Gallery
                items={gallery}
                onUseAsCover={(assetID) => cover.mutate(assetID)}
                busy={cover.isPending}
              />
              {game.data.cover_override && (
                <button
                  type="button"
                  onClick={() => cover.mutate(null)}
                  disabled={cover.isPending}
                  className="mt-4 inline-flex items-center gap-1.5 rounded-md border border-mga-border px-3 py-1.5 text-xs text-mga-muted transition hover:text-mga-text disabled:opacity-60"
                >
                  <RotateCcw className="h-3.5 w-3.5" />
                  Go back to the original cover
                </button>
              )}
              {cover.error && (
                <p className="mt-2 text-xs text-rose-300" role="alert">
                  The cover could not be changed: {cover.error instanceof Error ? cover.error.message : 'the server refused it.'}
                </p>
              )}
            </SectionCard>
          )}

          <SectionCard title="Where it comes from" description="Every source that has this game, and what it found.">
            <div className="space-y-3">
              {game.data.source_games.map((source) => <SourceRow key={source.id} source={source} />)}
              {game.data.source_games.length === 0 && (
                <p className="text-xs text-mga-muted">No source currently reports this game.</p>
              )}
            </div>
          </SectionCard>
        </>
      )}
    </div>
  )
}

/** Screenshots and artwork that were fetched all along and never shown. Almost
 *  every game in a scanned library has at least one, and the detail page was
 *  displaying only the cover. */
function Gallery({
  items, onUseAsCover, busy,
}: {
  items: GameMediaDetailDTO[]
  onUseAsCover: (assetID: number) => void
  busy: boolean
}) {
  const [open, setOpen] = useState<GameMediaDetailDTO | null>(null)
  return (
    <>
      <ul className="grid grid-cols-2 gap-3 sm:grid-cols-3 lg:grid-cols-4">
        {items.map((item) => (
          <li key={item.asset_id} className="group relative overflow-hidden rounded-lg border border-mga-border transition hover:border-mga-accent/40">
            <button
              type="button"
              onClick={() => setOpen(item)}
              className="block w-full focus:outline-none focus:ring-2 focus:ring-mga-accent/50"
            >
              <img src={mediaUrl(item)} alt="" loading="lazy" className="aspect-video w-full object-cover" />
            </button>
            {/* Revealed on hover and always reachable by keyboard, so the
                action is not hidden from anyone who does not use a mouse. */}
            <button
              type="button"
              onClick={() => onUseAsCover(item.asset_id)}
              disabled={busy}
              className="absolute bottom-1.5 right-1.5 inline-flex items-center gap-1 rounded border border-mga-border bg-mga-surface/90 px-1.5 py-1 text-[0.66rem] text-mga-muted opacity-0 transition hover:text-mga-text focus:opacity-100 focus:outline-none focus:ring-2 focus:ring-mga-accent/50 disabled:opacity-40 group-hover:opacity-100"
            >
              <ImageUp className="h-3 w-3" />
              Use as cover
            </button>
          </li>
        ))}
      </ul>

      {open && (
        // Click anywhere to close. A picture viewer that needs a hunt for the
        // close button is worse than no viewer.
        <div
          role="dialog"
          aria-modal="true"
          aria-label="Picture"
          onClick={() => setOpen(null)}
          onKeyDown={(event) => { if (event.key === 'Escape') setOpen(null) }}
          className="fixed inset-0 z-50 flex items-center justify-center bg-black/80 p-6"
        >
          <img src={mediaUrl(open)} alt="" className="max-h-full max-w-full rounded-lg" />
        </div>
      )}
    </>
  )
}

function Facts({ game }: { game: GameDetailResponse }) {
  const facts: { label: string; value: string }[] = []
  if (game.developer) facts.push({ label: 'Developer', value: game.developer })
  if (game.publisher) facts.push({ label: 'Publisher', value: game.publisher })
  if (game.release_date) facts.push({ label: 'Released', value: formatDate(game.release_date) })
  if (game.genres?.length) facts.push({ label: 'Genres', value: game.genres.slice(0, 4).join(', ') })
  if (game.max_players) facts.push({ label: 'Players', value: String(game.max_players) })
  if (game.achievement_summary) {
    const summary = game.achievement_summary
    facts.push({
      label: 'Achievements',
      value: `${formatCount(summary.unlocked_count)} of ${formatCount(summary.total_count)} unlocked`,
    })
  }
  if (facts.length === 0) return null

  return (
    <dl className="mt-4 grid grid-cols-[7rem_1fr] gap-x-4 gap-y-2 text-xs">
      {facts.map((fact) => (
        <div key={fact.label} className="contents">
          <dt className="text-mga-muted">{fact.label}</dt>
          <dd className="text-mga-text">{fact.value}</dd>
        </div>
      ))}
    </dl>
  )
}

/** One provider's answer, with the evidence date attached — because "you own
 *  this" observed four months ago is a different claim from the same words
 *  observed this morning. */
function OfferRow({ offer }: { offer: CatalogOffer }) {
  const answer = describePlayability(offer)
  const stale = isStale(offer)

  return (
    <div className="rounded-lg border border-mga-border bg-mga-elevated/40 p-4">
      <div className="flex flex-wrap items-start justify-between gap-3">
        <div className="min-w-0">
          <p className="text-sm font-medium text-mga-text">{answer.headline}</p>
          <p className="mt-1 text-xs leading-5 text-mga-muted">{answer.detail}</p>
        </div>
        <StatusPill label={answer.tone === 'good' ? 'Playable' : answer.tone === 'danger' ? 'Gone' : 'Check this'} tone={answer.tone === 'neutral' ? 'neutral' : answer.tone} />
      </div>
      <div className="mt-3 flex flex-wrap items-center gap-x-4 gap-y-1 text-[0.68rem] text-mga-muted">
        <span>{sourceLabel(offer.provider)}</span>
        <span>{entitlementLabel(offer.entitlement)}</span>
        <span>{availabilityLabel(offer.availability)}</span>
        <span>Last checked {formatDate(offer.observed_at)}</span>
        {stale && <StatusPill label="Not checked lately" tone="attention" />}
      </div>
    </div>
  )
}

function SourceRow({ source }: { source: SourceGameDetailDTO }) {
  const missing = source.status !== 'found'
  return (
    <div className="flex flex-wrap items-start justify-between gap-3 rounded-lg border border-mga-border bg-mga-elevated/40 p-4">
      <div className="min-w-0">
        <p className="text-sm font-medium text-mga-text">
          {source.integration_label?.trim() || sourceLabel(source.plugin_id)}
        </p>
        <p className="mt-1 truncate text-xs text-mga-muted">
          {source.root_path || source.raw_title}
          {source.files?.length ? ` · ${formatCount(source.files.length)} file${source.files.length === 1 ? '' : 's'}` : ''}
        </p>
        {source.last_seen_at && (
          <p className="mt-1 text-[0.68rem] text-mga-muted">Last seen {formatDate(source.last_seen_at)}</p>
        )}
      </div>
      <StatusPill
        label={missing ? 'Missing right now' : 'Found'}
        tone={missing ? 'attention' : 'good'}
      />
      {source.kind && source.kind !== 'base_game' && <StatusPill label={humanizeIdentifier(source.kind)} />}
    </div>
  )
}
