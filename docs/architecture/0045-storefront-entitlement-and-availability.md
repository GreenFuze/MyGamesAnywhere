# ADR-0045: Storefront Entitlement, Availability and Play Engagement

Status: Accepted for implementation
Date: 2026-09-04
Jira: MGA-118
Canonical decision: Confluence page `ADR-0045 — Storefront Entitlement, Availability and Play Engagement`

This local mirror records the contract and the honesty rules next to the code. Confluence remains authoritative.

## Context

Game Pass means an Xbox library is not one list. A title can be owned outright, owned but delisted, playable through a subscription, or a game the player finished two years ago that has since left the subscription and can now only be bought. Those differences decide what the console may offer, so they cannot be collapsed into one boolean.

They are three axes, not five values: **entitlement × availability × engagement**.

`internal/catalog` (MGA-88) already defined the first two, with persisted history and `added` / `removed` / `returned` / `leaving_soon` events. It had **no production writer** — `Service.Observe` was called only from tests, so the tables were empty in every real deployment.

## Decision

- The `source.games.list` contract gains optional `entitlement` and `availability` in the catalog's vocabulary, plus engagement evidence (`last_played_at`, achievement counts, gamerscore). The plugin reports the conclusion because only it knows what its provider's flags mean; the server does not guess per provider.
- The scan writes one catalog observation per described source game, after persistence, when canonical ids exist. This is the first and only production writer.
- Offers are keyed by `(plugin, external id)` when joining scan results to persisted rows, because persistence rebinds a scanned row onto an existing one by that natural key.
- **Supported APIs only.** Title Hub and the documented OAuth scopes. `displaycatalog.mp.microsoft.com` and `catalog.gamepass.com/sigls/` are undocumented internal endpoints and are not used, consistent with the policy already set for Google Play.

## The honesty rules

These are the point of the change, and each is enforced by a test.

- **An unrecognised or absent entitlement claim becomes `unknown`, never `owned`.** The Xbox connector reads play history, not entitlements. A title without the Game Pass flag is equally a demo, a trial, an expired Game Pass title, or a family-shared play. MGA does not assert ownership it has not observed.
- **Silence is not removal.** An absent or unrecognised availability claim becomes `unknown`, which is distinct from `unavailable`. A provider going quiet must not make MGA announce that a game has been removed.
- **Played is evidence-based, not presence-based.** Appearing in a play-history listing is not the same as having played something. The evidence is a `lastTimePlayed` timestamp, an unlocked achievement, or earned gamerscore.
- **A source that says nothing gets no offer.** A filesystem connection does not acquire a fabricated entitlement just because a scan ran.
- **A failed observation never fails a scan.** History is worth having; it is not worth losing a completed scan over.

## What this does and does not deliver

| State | Outcome |
|---|---|
| On Game Pass, played | Delivered. `subscription` / `available`, with engagement evidence. |
| Was on Game Pass, played, now gone | Readable from the offer's observation history, **from the moment observations begin**. Not retroactive, and it shows as an entitlement transition rather than a labelled `removed` event, because leaving Game Pass makes MGA *unsure* about access rather than certain the title is gone. |
| Owned and available | Entitlement stays `unknown`. There is no ownership signal in the supported API surface. |
| Owned but delisted | Not separable from the previous row without ownership plus a store lookup. |
| On Game Pass, never played | Structurally absent. A play-history endpoint cannot enumerate untouched titles. |

Measured on a real account: 96 observations recorded, 51 `subscription`/`available`, 45 `unknown`/`unknown`, 0 claimed as owned.

## Fixed in passing

`Offer.StaleAt` was a `time.Time` with `omitempty`, which does not omit a zero struct. Every fresh offer therefore serialized `"stale_at":"0001-01-01T00:00:00Z"`, which is truthy in JavaScript, and the console reported every freshly observed offer as stale evidence. It is a pointer now, so "not stale" is absent from the payload. The bug was invisible until the table had rows in it.

## Migration

`NO_MIGRATION_NEEDED`. Migration 39 already created `catalog_offers`, `catalog_offer_observations` and `catalog_offer_events`; this change only starts writing to them. The `source.games.list` additions are optional fields on an IPC payload, and a plugin that omits them is treated as saying nothing rather than as reporting unavailability.
