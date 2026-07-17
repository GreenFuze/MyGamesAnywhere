# ADR-0015: Device emulator routes and defaults

- **Status:** Accepted
- **Date:** 2026-07-17
- **Scope:** Server, web UI, device protocol, and MGA Client

## Context

A platform is not owned by one emulator. A game may be playable through several
standalone emulators, RetroArch cores, a browser emulator, or another local or
cloud route. Those routes differ in compatibility, achievements, save handling,
firmware requirements, and launch readiness. Choosing a default must not erase
the alternatives.

An MGA endpoint identifies one device and one OS user. Emulator discovery and
configuration therefore belong to that endpoint, not to a physical computer or
an MGA profile. The server can run on another LAN computer over HTTP; no design
may assume localhost or send server-local filesystem paths to the client.

## Decision

### Catalog and selection

1. MGA owns a versioned emulator catalog with the relation
   `platform -> compatible emulator[]`. The relation is never singular.
2. Each catalog entry has a stable emulator ID, player-facing name, supported
   platforms, launch-adapter state, and capability facts. Capability facts are
   scoped to an emulator, version, and, where applicable, core; they are not
   inferred from the platform.
3. The device inventory remains the source of truth for runtimes detected for
   that device+OS-user. Runtime executable paths are transient client facts and
   are not persisted as configuration or accepted from the web UI.
4. A device owner may select one default emulator for each platform. The default
   controls the main local-emulator action for that endpoint. Browser, cloud,
   storefront, installed, and other device routes retain their contextual route
   policy. Every compatible, ready emulator remains available in the Play
   dropdown.
5. If an explicit default is absent or no longer ready, MGA selects the first
   ready compatible route using stable catalog order. It does not silently
   rewrite the persisted preference.
6. If the resolved emulator can play several source copies, every copy remains
   in the dropdown and is labelled by source. Until MGA adds a separate
   per-edition source preference, stable canonical source order chooses the main
   action; this does not merge or hide the other copies.

### Persistence and authorization

Migration 22 adds `device_emulator_preferences`, keyed by endpoint and platform,
with the selected emulator ID, updating profile, and timestamp. Endpoint view
access may read catalog/readiness. Endpoint owner access is required to change a
default because it changes endpoint-wide behavior for every authorized profile.

Only stable IDs are persisted. Existing inventory JSON remains schema version 1
in this slice because its existing runtime ID/name/version/path facts are
sufficient; no persisted inventory shape changes.

### Readiness and capabilities

Each emulator route reports an explicit state and reason. A route is `ready`
only when all of the following are true:

- the endpoint is connected and advertises the typed emulator-launch capability;
- a compatible emulator runtime is detected;
- MGA has a typed launch adapter for that emulator;
- required core, firmware, or configuration facts are known and satisfied; and
- the selected source can be safely delivered to the endpoint.

Otherwise the route is visible as `needs_setup`, `unavailable`, or `unknown`.
MGA must not claim RetroAchievements, Save Sync, core, firmware, or launch support
from platform compatibility alone. Unknown facts stay unknown.

### Launch protocol and trust boundary

Local emulator play uses a new typed `game.launch_emulator` command. It is not an
extension of native installed-game `game.launch` and is not a generic execution
facility.

The request identifies the canonical/source game, platform, emulator ID, and
typed content artifacts. It never carries an executable path, shell command,
arbitrary argument list, credential, or server-local path. Content downloads use
short-lived, capability-scoped server tokens over the endpoint's configured MGA
server URL, including supported trusted-LAN HTTP deployments.

The MGA Client:

- resolves the emulator only through its allowlisted local runtime discovery;
- validates the requested emulator/platform/adapter combination;
- stores delivered content in an MGA-owned per-user cache;
- validates artifact name, size, and digest before launch;
- constructs arguments through a typed emulator adapter; and
- fails closed on unsupported cores, firmware, content layouts, or paths.

The first implementation may mark an emulator route `needs_setup` until a safe
adapter and all required facts exist. Visibility is preferable to false
playability. Managed emulator installation and updating are a later ADR.

### Player experience

The Emulators settings page is device-oriented. It shows every compatible
emulator by platform, whether it was detected, capability/readiness details, and
the editable default. Technical detail belongs in expandable help/tooltips.

Game cards use the resolved default route for the main Play button and list all
other available browser, cloud, storefront, installed, and emulator routes in
the dropdown. There is no generic Open action.

### Save Sync and non-local storefronts

Save Sync is a route capability, not a game-wide boolean. Follow-up work must
model each route as one of:

- MGA-managed local save files;
- provider API-managed saves;
- provider-managed opaque cloud saves; or
- unsupported/unknown.

Steam, Xbox, Xbox Cloud Gaming, and other non-local storefront routes must not
create competing writers or imply that cloud saves are accessible when the
provider does not expose them. Cross-route synchronization requires explicit
save-format compatibility or a recorded converter. Until then, the UI reports
the boundary and leaves provider-managed saves to the provider.

## Compatibility and rollback

Older clients do not advertise `game.launch_emulator`; their routes remain
visible but are not ready. Existing native launch, browser play, cloud play,
installations, and inventory reports are unchanged. Rolling back application
code leaves migration 22's additive table unused; migration 22 is never edited
after application.

## Acceptance criteria

- Migration 22 is additive, tested, and preserves existing installs.
- The API returns multiple compatible emulator routes per platform and a
  separately resolved default.
- Owners can change/reset a device+OS-user default; viewers cannot.
- Settings show detected and missing compatible emulators without persisting
  client executable paths.
- Cards preserve alternate routes and do not collapse a platform to one emulator.
- Typed protocol validation rejects arbitrary paths, arguments, unsupported
  emulator/platform pairs, and malformed artifacts.
- At least one typed adapter is proven end-to-end before any emulator route is
  labelled ready; all incomplete adapters remain `needs_setup`.
- Server and client tests cover remote-LAN server URLs and do not assume
  localhost or HTTPS.
- The authoritative handoff records exact tests, package hashes, migration
  evidence, and real packaged-server/installed-client E2E evidence.
