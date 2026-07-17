# ADR-0013: Device-side installation preflight and prerequisite policy

- **Status:** Implemented and locally verified
- **Date:** 2026-07-17
- **Scope:** Checks shown before MGA dispatches an installation

## Context

MGA Server may run on another trusted-LAN computer and therefore cannot inspect
the target device's filesystem, free space, installed storefronts, emulators,
or OS-user environment. An installation must not infer those facts from the
server host. Players need a clear answer on the web Install screen before MGA
starts machine-local work.

Prerequisites do not have one universal owner:

- Steam and Xbox delivery require their corresponding storefront software;
- emulated games require a compatible emulator and may require features such
  as RetroAchievements support;
- a normal game installer owns its embedded or chained prerequisites;
- an archive may be a ready-to-play game, a game with undeclared runtimes, or a
  compressed installer, and MGA cannot safely infer which from its extension.

This also exposes three distinct settings scopes: MGA-wide policy, player
profile preferences, and device/OS-user configuration.

## Decision

### Preflight ownership and flow

When the player opens or confirms an Install action, MGA Server creates a typed
installation-preflight request for the selected device/OS-user endpoint. MGA
Client evaluates it using that endpoint process's environment and returns typed
checks. The web interface shows the result before dispatching the mutating
installer command.

The request may contain only a validated destination-root template, a bounded
storage estimate, a known installation category, and typed prerequisite IDs.
It cannot contain a shell command, executable path, script, arguments, registry
path, environment dump, or arbitrary local probe. A preflight never installs,
launches, elevates, or removes anything.

Results distinguish:

- **ready** — a required fact was positively detected;
- **missing** — a supported probe positively found the requirement absent;
- **unknown** — MGA cannot reliably determine the fact;
- **installer managed** — the signed/selected game installer owns it;
- **not applicable** — the check does not apply to this installation.

Only a definite blocking failure, such as insufficient destination storage or
a positively missing required storefront/emulator, disables Install. Unknown
archive prerequisites are a visible warning and do not block the player.

Preflight is current point-in-time evidence, not a guarantee that the device
will remain unchanged. The existing installer repeats its safety checks before
mutation and remains authoritative if facts change between preflight and
installation.

### Category policy

- **Storefront-delivered game:** require the typed storefront runtime. Steam is
  `storefront.steam`; Xbox is `storefront.xbox`. An unsupported Xbox probe must
  report unknown rather than incorrectly claiming the Xbox app is missing.
- **Emulated game:** MGA Server resolves the source platform through the global
  emulation catalog and requests the device/OS-user's selected compatible
  emulator plus required capabilities. RetroAchievements is a capability, not
  a second copy of the game or an assumed property of every emulator.
- **Game with native installer:** show prerequisites as installer managed. MGA
  does not enumerate, install, or later remove embedded/shared prerequisites.
- **Managed archive:** show prerequisite knowledge as unknown until the source
  has explicit package classification/metadata. This intentionally covers the
  compressed-installer ambiguity without silently executing an extracted
  installer.

### Emulator settings ownership

Emulation has two layers:

1. MGA-wide catalog data maps each normalized emulation platform to an ordered
   list of compatible emulator definitions (`platform -> emulator[]`). It may
   nominate a default, but never collapses the list to one emulator. Capability
   facts belong to each emulator/version/core combination; RetroAchievements,
   save formats, firmware needs, and launch features are not assumed to be
   shared by every compatible emulator.
2. Device/OS-user settings select or configure zero or more emulators available
   to that endpoint and may choose a default for a platform. Paths and discovery
   belong to MGA Client, never MGA Server's filesystem. A default affects the
   main Play action only; every other ready compatible emulator remains a
   separate selectable route.

The UI will expose a dedicated **Emulators** settings surface organized by
device/OS user. A later bounded emulator-management ADR must define the catalog,
persisted selection/configuration schema, launch contract, managed installation,
updates, cores/firmware, and RetroAchievements credentials before those values
become mutable. ADR-0013 only establishes the preflight contract and shows
currently detected emulator facts; it does not guess that larger schema.

### Settings taxonomy

Settings UI and future schemas must label their scope explicitly:

- **MGA:** server/library/integration policy shared by the deployment;
- **My Settings:** active MGA profile preferences;
- **This device:** the paired device + OS-user endpoint.

Device facts and emulator availability are never presented as global MGA
settings, even when only one device is currently paired.

## Persistence and compatibility

`NO_MIGRATION_NEEDED` for this bounded slice. Preflight commands and their
terminal JSON results use the existing `device_commands` audit/result storage;
no new long-lived setting is introduced. Existing endpoints without the new
capability remain usable for existing installations, but the web interface
reports that preflight is unavailable and requires the player to acknowledge
the warning rather than claiming the device was checked.

Persisted emulator selection/configuration is deliberately deferred and will
require migration 22 or later. Migration 21 is already applied and must not be
edited.

## Failure and transport behavior

- Offline, incompatible, or timed-out clients fail preflight visibly and do
  not start installation.
- A stale result is not silently reused for another destination or source.
- Plain HTTP/WS remains supported for an explicitly trusted LAN. The endpoint
  may be on any LAN host; no non-loopback server origin is rewritten to
  localhost and the UI does not claim the transport is secure.
- Preflight result/audit data excludes environment values, broad installed-app
  inventories, secrets, and arbitrary local paths. A detected allow-listed
  runtime may retain the same bounded path already present in device inventory.

## Initial acceptance criteria

1. A typed, read-only `installation.preflight` capability is validated by the
   shared protocol, server authorization, and MGA Client allow-list.
2. The client expands the destination template locally, checks destination
   storage when an estimate is available, freshly checks allow-listed runtime
   IDs, and returns ready/missing/unknown/installer-managed results.
3. The existing GOG Inno dialog reports installer-managed prerequisites.
4. The existing archive dialog warns that prerequisites and compressed-
   installer classification are not yet known, without blocking solely for
   that warning.
5. Definite insufficient storage blocks Install and all results are visible in
   casual player-facing language.
6. Settings shows detected emulator facts per device/OS user without adding a
   fake global configuration or persisting an undefined emulator mapping.

## Verification — 17 July 2026

The packaged portable server and installed elevated per-user MGA Client ran the
new capability through the production frontend. Opening Pikuniku's real GOG
Install dialog dispatched one read-only preflight command bound to its source,
`C:\Games`, and 148,780,824-byte package. The terminal result reported enough
free space and installer-managed components; the UI showed `142 MB`, current
free space, and kept Install enabled. It did not launch the installer or change
game files. Browser focus did not dispatch a duplicate command after the final
query policy was applied.

Settings > Emulators showed the paired `tc-pc / TC-PC\tcs` endpoint separately
and detected ScummVM at its client-reported path. The surface remains read-only
until the later emulator catalog/configuration ADR. Browser console inspection
reported no warnings or errors.

Protocol, client, server, all seven standalone plugin modules, OpenAPI
generation/currentness, frontend contract generation, frontend unit tests, and
the production build pass. `govulncheck` reports no reachable vulnerabilities
for protocol, client, or server. The known approximately 835 KB Vite chunk
warning remains.
