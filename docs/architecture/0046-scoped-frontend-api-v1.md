# ADR-0046: Scoped Frontend API v1

Status: Accepted for implementation
Date: 2026-09-04
Jira: MGA-119
Canonical decision: Confluence page `ADR-0046 — Scoped Frontend API v1` (23199745)

This local mirror records the contract and the reasoning next to the code. Confluence remains authoritative.

## Context

ADR-0047 makes external frontends the player-facing surface and says they "use scoped, revocable API clients" whose permissions "distinguish catalog read, media read, content read, cache preparation and management capabilities".

Half of that existed. `internal/frontendauth` had all five scopes, a full issue/rotate/revoke/expire lifecycle, per-profile isolation and an audit trail — and gated nothing. `RequireFrontendAPIClient` had exactly one production call site, and it passed no required scopes at all. Every scope constant outside that package appeared only in tests.

The capability endpoint made it worse by advertising six features. Five of them were unreachable with the token that had just read them, so a frontend could complete a capability negotiation and then discover it had nowhere to go. The library, media and content pipes were fully built, tested and profile-scoped — but reachable only with a browser session cookie.

## Decision

Mount `/api/frontend/v1/*` as a bearer-token projection of the handlers the management console already uses, one scope per route.

| Feature | Scope | Routes |
|---|---|---|
| capability-discovery | *(none)* | capabilities |
| catalog-projection | `catalog.read` | games, game, game detail, catalog offers, offer, offer history |
| metadata-media | `metadata.read` | media asset (GET, HEAD) |
| content-delivery | `content.read` | copy manifest, copy file (GET, HEAD) |
| cache-preparation | `content.prepare` | create, poll and cancel a materialization |

Four decisions carry the design:

- **Reuse the handlers, do not fork them.** "What does this profile own" cannot drift between two implementations if there is only one. Only the proof of identity differs: `RequireFrontendAPIClient` resolves a token to a profile and puts it in the request context, which is all these handlers read.
- **Administration is structurally out of reach, not merely unrouted.** `RequireAdminProfile` reads the session-access context, which the bearer path never populates. A frontend token cannot satisfy it even where a shared handler is reachable by both paths.
- **Preparation is not a read.** `content.prepare` is separate from `content.read` because materialization spends the server's storage. A read-only frontend must not be able to trigger it.
- **Capability discovery is generated from what was mounted.** The router hands the controller the routes it actually registered. Discovery can no longer promise an endpoint that does not exist, which is the defect that motivated the ticket.

Capability discovery itself needs a valid token but no particular scope: a client has to be able to learn what it is missing without already having it. The response splits `features` from `unavailable_features`, and each withheld feature names the scope that unlocks it, so a 403 has a remedy instead of being a dead end.

## What was removed

`runtime-artifacts` is gone from the advertised feature list rather than implemented. ADR-0047 scopes frontend permissions to the five above and gates runtime bytes on per-artifact licence and compliance state instead; serving them here would have widened the permission model, which is a product decision and not this ticket's to make. Removing a claim the server could not honour is the smaller change.

## Verification

Unit tests walk the built router rather than the route table, so a route added later outside `registerFrontendAPIV1`, or with no scope, still has to fail closed under four probes: anonymous, revoked, expired, and scope withheld. Scope assignments are additionally pinned by an independent expectation table, because a test that reads its expectation from the table it checks proves the wiring and not the assignment.

Exercised against a running server with real issued tokens, on a profile holding 99 games:

| Check | Result |
|---|---|
| catalog.read token: games, offers, capabilities | 200 |
| catalog.read token: media, manifest, materialization | 403 |
| four-scope token: media asset | 200, 33855 bytes, `Accept-Ranges: bytes` |
| four-scope token: `Range: bytes=0-99` | 206, exactly 100 bytes |
| four-scope token: `If-Modified-Since` | 304 |
| no token / unknown token | 401 |
| another profile named in header or query | 403 |
| after revoking the client | 401 on every route, including capabilities |

Content byte delivery over a bearer token is covered by test rather than by the live instance: the review dataset has no reconciled copy with files, and the admin session returns the identical 404 on the same copy id, so the gap is in the data, not the route.

Media assets carry `Last-Modified` but no `ETag`; conditional requests work through `If-Modified-Since`. That is pre-existing `MediaController` behaviour and belongs to MGA-92, which specifies the media caching contract.

## Migration

`NO_MIGRATION_NEEDED`. Migration 38 already created `frontend_api_clients` with id, profile, name, scopes, secret hash, expiry and revocation. This change adds routing and authorization only. The capability response shape changed — `features` became objects and `endpoints` was folded into them — which is safe because no client existed to read the old shape.
