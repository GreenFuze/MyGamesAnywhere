# ADR-0017: Route-level Save Domains and provider boundaries

- **Status:** Accepted
- **Date:** 2026-07-17
- **Scope:** Server read model, web UI, Save Sync wording, storefront and device routes

## Context

MGA already stores browser-runtime save snapshots through a selected Save Sync
service such as Local Disk or Google Drive. That storage transport is not the
same thing as a game's save ownership or format. A canonical game may be played
through a browser runtime, several emulators, an installed copy, Steam, Xbox,
or Xbox Cloud Gaming, and those routes do not necessarily share saves.

Steam Cloud, Xbox cloud saves, and similar provider systems are not exposed by
MGA's current integrations. MGA must not claim that a provider has a cloud save,
that MGA can read it, or that moving MGA backup storage migrates provider saves.
Title identity alone is not compatibility evidence.

## Decision

### Save capability belongs to a route

Every visible source or play route may expose a typed save capability with:

- a stable, route-specific domain ID;
- access mode: `mga_managed`, `local_files`, `provider_api`,
  `provider_opaque`, `unsupported`, or `unknown`;
- status: `available`, `provider_managed`, `needs_adapter`, `unsupported`, or
  `unknown`;
- manager: `mga`, `device`, `provider`, or `unknown`;
- explicit MGA read/write booleans; and
- transfer policy: `same_domain_only`, `converter_required`, `unavailable`, or
  `unknown`.

The initial resolver derives capabilities from existing source, runtime,
storefront, endpoint, and emulator facts. It does not infer compatibility
between two routes. Domain IDs deliberately include source and route identity;
two routes share a domain only after a future explicit compatibility rule or
converter says they do.

### Initial ownership classifications

1. Browser-runtime saves are `mga_managed`. MGA can back them up through a
   configured Save Sync service and may restore only the same route domain.
2. Local emulator routes and MGA-managed/native installations are
   `local_files`, but `needs_adapter` until a bounded game/emulator/storefront
   adapter discovers and validates the save location and format. MGA does not
   scan arbitrary profile directories or guess save paths.
3. Steam, Xbox, Xbox Cloud Gaming, and Epic routes are
   `provider_opaque`. The provider may manage saves, but MGA currently has no
   supported read or write path. The UI uses conditional wording and never
   asserts that a cloud save exists.
4. `provider_api` is reserved for a future provider integration with a documented,
   authenticated save API. No current storefront is classified this way.
5. Unknown providers and incomplete routes remain `unknown`; absence of
   evidence does not become unsupported or compatible.

An installed Steam or Xbox copy retains the provider boundary even though it
runs on a local endpoint. xCloud is a separate Xbox-provider domain and is not
automatically compatible with an installed Xbox copy.

### Conflict, authority, and copying

MGA never overwrites or competes with a provider-managed cloud writer. Cross-route
copying is unavailable until the exact source and destination domains have an
explicit compatibility record or a bounded converter. A later converter must
define format/version checks, ownership, conflict policy, rollback, and failure
behavior before it can write.

Existing MGA browser backup optimistic concurrency remains authoritative for
MGA-managed snapshots. This ADR does not change retention or force-write policy.
The first UI is informational and exposes no cross-route copy action.

### Player experience

Game details show a concise Saves section for the actual source, browser,
cloud, installed, and emulator routes. It states who manages each save and what
MGA can currently do. Technical IDs remain out of the primary view.

Settings calls the existing transport operation **Move MGA save backups**. Its
dialog explains that it moves MGA's browser-save backups between configured
storage services; it does not copy Steam, Xbox, or other provider saves.

## Persistence and compatibility

`NO_MIGRATION_NEEDED`: this slice adds an entirely derived API read model and
player-facing wording. It writes no DB row, SQLite column, persisted JSON/config,
preference, compatibility mapping, location, or converter. Existing installs
remain safe and migrations 22, 23, and 24 remain untouched. Migration 25 is
reserved for the first future persisted save-location, compatibility, converter,
or user-override model.

Older clients and servers ignore the additive response fields. Existing browser
snapshot keys and Save Sync service configuration are unchanged. Trusted-LAN
HTTP and remote MGA Server origins remain supported; no localhost, shared-disk,
or HTTPS assumption is introduced.

## Security and failure behavior

- The browser receives capability classifications, never discovered save paths,
  credentials, provider tokens, or file contents.
- Unknown or incomplete evidence fails closed to read-only explanatory state.
- No current route classification triggers device filesystem access or a client
  command.
- Provider names are normalized from fixed integration/plugin IDs; arbitrary
  labels do not grant read or write capability.

## Acceptance criteria

- Unit tests cover browser, emulator, native installed, Steam, Xbox, xCloud,
  Epic, and unknown classifications.
- Game detail responses attach capabilities to sources and launch options;
  device availability attaches them to installed and emulator routes.
- The frontend presents route-specific save ownership without promising
  provider access or cross-route compatibility.
- Save Sync settings clearly distinguish backup-storage movement from save
  conversion or storefront-cloud migration.
- OpenAPI/contracts, server tests, frontend unit tests, and production build pass.

## Deferred decisions

- first bounded local-save discovery adapters and their privacy/path rules;
- persisted same-format compatibility and converter registry;
- per-domain conflict and retention policies beyond existing browser snapshots;
- authenticated provider APIs if a provider exposes a supported save API; and
- user/community overrides, including evidence and revocation policy.
