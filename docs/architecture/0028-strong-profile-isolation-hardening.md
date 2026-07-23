# ADR-0028: Strong profile isolation hardening

- **Status:** Accepted; implementation complete, real two-profile/remote E2E pending
- **Date:** 2026-07-19
- **Scope:** Profile authentication, authorization, background work, events,
  connections, saves, settings sync, browser state, and regression evidence
- **Supersedes:** No earlier ADR. This hardens the boundaries established by
  ADR-0019 and ADR-0027.

## Purpose and completion rule

The selected MGA profile and its profile-owned rows are the security and data
ownership boundary. A browser-supplied profile ID is routing input, not proof
of identity. The initiating computer, MGA Client, plugin process, Windows
account, and another profile's session are never authority for that profile.

This file is the authoritative remediation checklist for the 2026-07-19
profile-isolation audit. Do not delete an item when implementation changes.
Check it only after the implementation, focused regression tests, and required
packaged-runtime evidence all exist. A parent item remains unchecked until all
of its child items are checked. Add newly discovered isolation problems here
before fixing them so the completion claim remains auditable.

The remediation is complete only when every checkbox in this file is checked,
the intentional worktree diff is fully accounted for (and committed only when
the user authorizes it), and the final evidence section contains exact commands
and packaged-runtime results. A separate fresh audit happens after that claim;
it is intentionally not a checkbox in this remediation plan.

## Audit baseline

The live packaged v0.2.6 server demonstrated the primary failure without
mutating data. An anonymous HTTP client selected the password-protected TCs
profile through `X-MGA-Profile-ID` and received:

- HTTP 200 and 273 visible games;
- HTTP 200 and 13 connection records, with OAuth token objects correctly
  redacted;
- HTTP 200 from administrator update status and scan-report routes;
- HTTP 401 from Devices, which already enforces matching session authority.

All existing server, database, save-sync, settings-sync, Xbox, Google Drive,
Steam, Epic, and frontend tests passed. The green suite therefore documents a
coverage gap rather than proof of strong separation.

## Persistence and migration note

`NO_SQLITE_MIGRATION_NEEDED` for the implementation. The completed scope
inventory proves that all affected rows already have an explicit owner or an
unambiguous owner join; migrations 1–28 remain immutable and migration 29 was
not created. Remote Settings Sync advances to the profile-owned v3 payload,
browser storage advances to profile-scoped v2 keys, and protected sync keys move
to per-profile paths. Compatibility and rollback behavior are recorded in the
profile-isolation scope ledger.

## Locked ownership and access policy

1. Public discovery and bootstrap routes remain deliberately public: health,
   about/license, setup status while setup is incomplete, profile picker data,
   login/session/logout, credential status and create-only initialization,
   OAuth callbacks/import, client downloads, and opaque capability-token
   redemption/transfer routes.
2. An unprotected profile remains usable without a password or PIN, preserving
   MGA's accepted trusted-LAN optional-credential policy.
3. Once a profile has a credential, every profile-owned read and write requires
   a valid session cookie for that exact profile. A session for profile A never
   authorizes profile B, even when A is an administrator.
4. A `must_change` session may read its own session/credential state, replace
   its credential, or log out. It cannot enter normal MGA APIs.
5. Administrator authorization is evaluated only after selected-profile access
   succeeds. Merely naming an administrator profile never grants administrator
   authority.
6. Device commands retain their stronger rule: they always require a matching,
   non-`must_change` authenticated session and the required device grant.
7. Query-string profile selection needed by EventSource, HEAD, media/runtime,
   and browser-play requests follows the same authorization policy as the
   profile header.
8. Object access fails closed. A cross-profile object ID returns not found or
   forbidden without revealing the other profile's object metadata.
9. Trusted-LAN HTTP remains supported. This plan does not silently require
   HTTPS or localhost and does not broaden the already accepted LAN threat
   model.
10. Truly server-global operations and profile-owned operations must be
    explicitly classified; global scope cannot be inferred from a missing
    `profile_id`.

## Remediation checklist

### P0 — Server authorization boundary

- [x] Add one reusable profile-access policy/service that fast-fails on missing,
  invalid, expired, wrong-profile, or `must_change` sessions for protected
  profiles while allowing deliberately unprotected profiles.
- [x] Apply that policy to every profile-owned API group, including library,
  game detail, browser play/HEAD, achievements, statistics, connections,
  connection checking/browsing/refresh, manual review, scans/reports,
  save-sync, source cache, settings sync, frontend preferences, profiles, media
  overrides, updates, and profile-owned SSE.
- [x] Keep only the explicitly public bootstrap/callback/capability routes
  outside the policy and document why each route is public in the router.
- [x] Make administrator middleware depend on an already authorized selected
  profile; prove that an anonymous request naming an administrator receives
  401/403 instead of administrator access.
- [x] Restrict `must_change` sessions to credential replacement, session status,
  and logout.
- [x] Preserve the existing stronger device-session and device-grant checks.
- [x] Add table-driven router tests covering anonymous, unprotected, correctly
  authenticated, wrong-profile, expired, deleted-profile, and `must_change`
  access across every route family.
- [x] Repeat the original anonymous packaged-server probe and record 401/403 for
  protected library, connections, update status, and scan reports while Devices
  remains protected.

### P1 — Background jobs and command ownership

- [x] Give scan jobs an immutable owning profile; scope start/status/cancel to
  that owner.
- [x] Keep any necessary global scan serialization, but return an opaque busy
  conflict to another profile rather than its job ID, integrations, progress,
  errors, or cancellation handle.
- [x] Give achievement-refresh jobs an immutable owning profile; scope
  deduplication, status, and progress to that owner.
- [x] Give integration-refresh jobs an immutable owning profile and validate it
  on every status lookup.
- [x] Give save-sync migration and prefetch jobs an immutable owning profile and
  validate it on every status lookup.
- [x] Confirm source-cache jobs/entries remain repository-scoped and add HTTP
  tests proving cross-profile job and entry IDs are inaccessible.
- [x] Add concurrent two-profile tests proving one profile cannot observe,
  reuse, cancel, or receive the result of another profile's job.

### P1 — SSE and notification isolation

- [x] Require every profile-owned event to carry the owning `profile_id` at a
  single event-publication boundary rather than relying on individual callers.
- [x] Profile-scope scan, achievement refresh, integration refresh, connection
  CRUD/status, OAuth completion/error, settings sync, save sync, source cache,
  and profile-initiated operation-error events.
- [x] Keep only explicitly classified server-global events visible to every
  profile, such as server update lifecycle and plugin-process health.
- [x] Reject or log profile-owned events that reach the bus without an owner;
  never silently broadcast them.
- [x] Bind SSE access itself to the selected-profile authorization policy.
- [x] Add two simultaneous SSE subscriber tests proving each receives its own
  events plus allowed global events, with no foreign job IDs, connection labels,
  game titles, provider errors, or OAuth state.
- [x] Verify browser notification history remains profile-keyed and that a
  same-tab or cross-tab profile switch reconnects SSE before showing the new
  profile UI.

### P1 — Save-sync worker context

- [x] Capture the initiating profile before starting save-prefetch, upload, and
  migration workers; rebuild the background context with that exact profile.
- [x] Ensure an all-games save migration enumerates only games visible to its
  owner and resolves only that profile's source and target connections.
- [x] Ensure queued upload workers can resolve only the connection that was
  validated for the owning profile.
- [x] Include owner identity in in-memory job records and profile-owned events
  without exposing credentials or local save paths.
- [x] Add two-profile tests with distinct games, save manifests, and providers;
  prove migration, prefetch, upload, delete-after-success, conflicts, and status
  never cross the boundary.

### P1 — Settings Sync semantics and compatibility

- [x] Change ordinary Settings Sync to export exactly one owner profile, that
  profile's connections, and that profile's settings. It must not export other
  players' names, roles, avatars, connections, or preferences.
- [x] Change ordinary Settings Sync pull to update/create only the selected
  owner profile and never create unrelated profiles from that payload.
- [x] Introduce a versioned profile-owned sync payload (version 3 or later) with
  an explicit owner-profile identity and fail-closed validation.
- [x] Define legacy version-2 import compatibility: import only an unambiguous
  matching owner; otherwise stop with an actionable choice instead of guessing.
- [x] Keep a future whole-server backup as a separate, explicit administrator
  feature; do not overload a player's Settings Sync connection with that role.
- [x] Prove two profiles using different Google accounts cannot upload, list,
  decrypt, restore, or overwrite one another's Settings Sync payloads.
- [x] Record the remote-payload compatibility change and rollback behavior.
  `NO_SQLITE_MIGRATION_NEEDED` unless implementation evidence contradicts it.

### P1 — Frontend cache and browser-state isolation

- [x] Introduce a single profile-scoped query-key factory or equivalent cache
  boundary and use it for every profile-owned React Query entry.
- [x] Remove profile-owned cached data before exposing a newly selected profile;
  invalidation alone is insufficient because stale data can render during
  refetch.
- [x] Profile-scope local library/play preferences, recently played entries,
  browser-emulator source/executable choices, review queues, duplicate-delete
  selections, and other player-owned local/session storage.
- [x] Bind persisted browser-play sessions to their owning profile and reject a
  session token after the selected profile changes.
- [x] Version the browser storage keys. Discard ambiguous legacy player-owned
  cache rather than assigning it to another profile; authoritative server-side
  profile settings may repopulate it.
- [x] Preserve deliberately browser-global appearance/accessibility settings
  only after recording them in the scope ledger below.
- [x] Add frontend tests that switch between two profiles while requests are
  delayed or failing and prove no old library, connections, notifications,
  recent games, saves, or play session appears in the new profile.
- [x] Record `NO_SQLITE_MIGRATION_NEEDED`; the versioned browser-storage cleanup
  is the required persisted-state compatibility action.

### P2 — OAuth lifecycle hardening

- [x] Add creation time, expiry, and bounded cleanup to in-memory OAuth state.
- [x] Validate plugin, profile, saved-connection identity, and expiry on normal
  callbacks as well as pasted callback imports.
- [x] Preserve the two-phase draft flow without allowing a completed draft to
  be consumed by a different profile or plugin.
- [x] Ensure OAuth success/error SSE contains the owner profile and is invisible
  to other profiles.
- [x] Add replay, expiry, wrong-profile, wrong-plugin, wrong-connection,
  concurrent-profile, and server-restart tests.
- [x] Confirm integration list/create/update/duplicate/status/browse responses
  never expose OAuth tokens, refresh tokens, authorization codes, PKCE material,
  or provider client secrets. Safe provider identity may remain visible.

### P2 — Persistent scope ledger and shared metadata

- [x] Inventory every SQLite table, persisted JSON/config object, protected-key
  entry, server cache directory, browser storage key, in-memory registry, and
  event family as `server_global`, `profile_owned`, `device_os_user`, or
  `opaque_capability`.
- [x] Review canonical cover/hover/background overrides, media state, emulator
  preferences, install roots, update state, encryption keys, and plugin health;
  record why each is shared or make it profile-owned.
- [x] Require repository APIs for profile-owned rows to derive or validate
  profile ownership even when callers already checked, providing defense in
  depth for achievements, source games, caches, saves, and integrations.
- [x] Add migration 29 only if the scope inventory requires SQLite ownership
  columns or constraints. Migrations 1–28 remain immutable.
- [x] For every persisted change not requiring SQLite migration, add an explicit
  `NO_MIGRATION_NEEDED` note covering existing installs, rollback, and stale
  state.

## Final regression and packaged evidence gate

- [x] Create a reusable adversarial test fixture containing a protected
  administrator, protected player, unprotected player, distinct connections,
  overlapping game titles, distinct source IDs, saves, jobs, notifications, and
  device grants.
- [x] Run the full server, protocol, MGA Client, standalone-plugin, frontend,
  migration-guard, formatting, vet, contract-generation, and production-build
  suites successfully.
- [x] Run a route inventory test that fails whenever a new profile-owned route
  lacks the profile-access policy.
- [x] Run a profile-owned event inventory test that fails whenever such an event
  lacks an owner.
- [x] Build the packaged server and MGA Client installer and test the packaged
  runtime rather than an npm development server or directly launched dev agent.
- [x] Verify logs and API responses contain no credentials, OAuth tokens,
  passphrases, save contents, or foreign-profile details.

The remaining real-browser, remote trusted-LAN, and explicit shared-device
validation is tracked only by
[MGA-37](https://greenfuzer.atlassian.net/browse/MGA-37). The independent fresh
audit is tracked by [MGA-38](https://greenfuzer.atlassian.net/browse/MGA-38).
This ADR no longer acts as a live progress checklist.

## Final evidence — fill only after implementation

```text
Implementation commits: intentionally uncommitted; commit/push require explicit user authorization
SQLite migration / NO_MIGRATION_NEEDED evidence: scope ledger complete; migrations 1-28 unchanged; no migration 29; check-migration-guard.ps1 PASS
Remote Settings Sync payload compatibility evidence: v3 single-owner payload; fail-closed mixed-owner validation; exact/single-owner v2 compatibility; two-account push/list/decrypt/pull/restore/overwrite test PASS
Browser storage compatibility evidence: mga.profile.v2 owner keys; ambiguous legacy player state discarded; theme/date preserved; profileIsolation and profileCacheBoundary tests PASS
Server tests: cd server; go test ./... -count=1 PASS
Protocol tests: cd protocol; go test ./... -count=1 PASS; go vet ./... PASS
MGA Client tests: cd client; go test ./... -count=1 PASS; go vet ./... PASS
Standalone plugin tests: epic-source, gog, hltb, retroachievements, steam, steam-source, xbox-source go test ./... -count=1 + go vet ./... PASS
Frontend tests and production build: generate:api-contracts PASS; test:unit PASS (13); build PASS (known 872.32 kB chunk warning)
Migration guard / formatting / vet: migration guard PASS; all changed Go files gofmt-clean; git diff --check PASS; server go vet ./... PASS
Packaged server hash and health: PID 5240; SHA256 173D4078F11515531A5C8597EAEBC207F04BF7533C177B64D4CF13B5CBC02F01; /health HTTP 200 OK
Packaged MGA Client hash and connection state: installer SHA256 9F7612E8A76FF6C08195CD31F9BB984DCD4988AD359AD806615DEEE4BA28FB90; client SHA256 71209939ECE9DED14925FA351BDE001B8A0AF3A212655CC7DE099793621AF198; installed elevated agent PID 48236; packaged UI reports Client elevated
Local two-profile E2E: TCs authenticated packaged UI PASS (273 games, 13 connections, profile-keyed notifications, safe Xbox identity, no credential field text); Orr/profile switching pending because Orr exists only on TV2
Remote-LAN two-profile E2E: pending hardened TV2 deployment and Orr's interactive Microsoft/Google account selection
Anonymous/wrong-profile adversarial probe: packaged anonymous TCs /api/games, /api/integrations, /api/update/status, /api/scan/reports, /api/devices all HTTP 401; wrong/expired/deleted/must_change table tests PASS
SSE/job isolation probe: two-subscriber visibility matrix, ownerless-event rejection, explicit global allowlist, foreign scan/achievement/integration/save/source-cache job tests PASS
Credential/token/log redaction probe: integration list/create/update/browse response tests PASS; packaged UI DOM contains no credential-field names; OAuth state stores no raw credentials
Known accepted global scopes: canonical identity/artwork/media metadata, server update lifecycle, plugin-process health, browser theme/date rendering preferences
Remaining issues: real Orr + remote-LAN + shared-device E2E; authorization to commit/release/deploy TV2; user-owned provider login interaction
```

## Post-completion fresh audit

The fresh audit in
[MGA-38](https://greenfuzer.atlassian.net/browse/MGA-38) must use a clean threat
model rather than merely replaying this historical checklist. Any resurfaced or
newly discovered problem becomes a Jira bug and, when it changes an accepted
decision, a successor decision.
