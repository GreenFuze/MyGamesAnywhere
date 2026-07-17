# ADR-0016: Emulator setup, components, and managed updates

- **Status:** Accepted
- **Date:** 2026-07-17
- **Scope:** Server, web UI, device protocol, and MGA Client

## Context

ADR-0015 made the emulator relation correctly plural and proved typed ScummVM
launching. A runtime name alone is not enough to decide whether another route is
ready. RetroArch also needs a compatible installed core, some systems need
legally obtained firmware, and RetroAchievements compatibility varies by
emulator/core/version. The web UI needs useful setup actions without turning the
MGA Client into a generic installer or command runner.

The endpoint remains one device plus one OS user. The MGA Server may be on a
different trusted-LAN computer over HTTP. No setup or launch request may contain
an executable path, package-manager package ID, download URL, shell command, or
arbitrary argument list.

## Decision

### Runtime correction: persisted inventory is not opaque

Packaged E2E on 17 July 2026 showed that `device_inventories` stores storage and
runtimes in separate JSON columns. Inventory schema 2 reached the server, but
package-manager facts were discarded when the endpoint was reloaded. Migration
24 therefore adds `package_managers_json`; server reads and writes it with the
other bounded inventory facts. Migration 23 remains immutable. This replaces
the earlier `NO_MIGRATION_NEEDED` assumption for inventory schema 2 without
changing the protocol contract.

### Versioned inventory and component facts

1. Device inventory schema 2 adds bounded package-manager facts and emulator
   component facts. The server accepts schema 1 and 2 so older clients remain
   compatible. Schema-1 runtimes have unknown component and management state.
2. Runtime version probes use only fixed, client-owned arguments with short
   timeouts. Failure leaves the version unknown and never hides a detected
   runtime.
3. Component inventory contains stable IDs, names, versions when available, and
   probe completeness. It does not contain a core/firmware path. MGA Client
   resolves any needed path again from fresh allow-listed discovery at action
   time.
4. RetroArch core discovery reads only its known configuration keys and scans
   the resolved core directory for bounded `*_libretro` modules. It does not
   enumerate arbitrary programs or directories.
5. Firmware is classified as `not_required`, `present`, `missing`, or `unknown`
   per emulator/core/platform. Proprietary firmware is always user supplied.
   MGA never downloads, uploads, copies, deletes, or licenses console firmware.
   A route cannot be ready when required firmware is missing or unknown.

Schema-1 snapshots remain valid and schema-2 fields are additive. Migration 24
preserves the package-manager part of that self-versioned snapshot in the
existing split-column inventory table.

### Core catalog, selection, and capabilities

1. The MGA catalog relation remains `platform -> emulator[]`. A RetroArch
   emulator entry additionally has ordered compatible `core[]` entries per
   platform. It is never reduced to one core.
2. Core IDs are stable normalized module IDs such as `snes9x` and
   `mupen64plus_next`; executable filenames and paths are not persisted.
3. Migration 23 adds `device_emulator_core_preferences`, keyed by endpoint,
   platform, and emulator. Owners may select/reset one preferred core for that
   route. All other compatible detected cores remain visible and selectable.
4. Automatic core resolution uses stable catalog order. Like emulator defaults,
   an unavailable stored choice is retained as user intent while the first ready
   compatible core becomes the resolved runtime choice; persistence is not
   silently rewritten.
5. RetroAchievements is a core/emulator/version fact. MGA's initial compatibility
   table follows the upstream RetroAchievements emulator-support list. A listed
   compatible core reports `supported`; an explicitly unsupported core reports
   `unsupported`; all others report `unknown`. This is display/readiness evidence,
   not proof that a particular game hash has an achievement set.
6. Save Sync remains a route capability. This ADR reports local-file/unknown
   ownership only and does not automatically copy saves between emulator and
   storefront/cloud routes.

### Typed RetroArch launch

For catalog entries whose file layout and firmware policy are known, the
existing `game.launch_emulator` request may carry a stable `core_id` and safe
relative `content_path`. MGA Server selects the content entry point from source
evidence. MGA Client re-discovers the requested core and starts RetroArch with a
fixed adapter-owned argument shape. Neither the UI nor server supplies a module
path or arbitrary arguments.

Multi-file sources without a deterministic entry point, unvalidated firmware
systems, unsupported cores, and unknown layouts remain visible as Needs setup.
ScummVM keeps its existing adapter and does not require a core.

### Managed install and update boundary

1. The new typed `emulator.setup` command supports only `install` and `update`.
   It requires endpoint Owner access because it changes endpoint-wide software.
2. The request contains only the emulator ID and action. Both server and client
   compile the same allow-list. On Windows, the first provider is `winget` with
   exact package IDs and non-interactive fixed arguments owned by MGA Client.
3. The initial allow-list covers RetroArch, ScummVM, DOSBox, DuckStation, and
   PCSX2. MGA does not accept a URL, package ID, source, executable, path, script,
   or extra arguments from the server or web UI.
4. MGA reports coarse typed phases (`checking`, `installing`/`updating`,
   `refreshing`, `complete`). It does not fabricate a byte percentage when the
   package manager does not provide a stable machine-readable percentage.
5. After success, MGA Client refreshes allow-listed inventory. Install succeeds
   only if the requested runtime becomes detectable. Update compares detected
   versions when available and otherwise reports completion without claiming a
   changed version.
6. External/manual installs remain externally owned. MGA may update them through
   the same explicitly requested package-manager action if the package manager
   recognizes the package, but it never adopts, relocates, uninstalls, rolls
   back, or deletes them. Uninstall is deliberately out of scope.
7. RetroArch cores are not OS packages. This slice discovers them and guides the
   player to RetroArch's own Online Updater; MGA does not download unsigned core
   modules from a mutable build feed. A future signed/pinned component provider
   requires another ADR.
8. A standard MGA Client may allow the package manager/Windows to request UAC
   for a setup action. An explicitly elevated client normally avoids an extra
   prompt. Elevation is never silently changed by the command.

### Player experience

Settings > Emulators remains grouped by device/OS user and platform. Each
emulator expands to show:

- detected version and setup ownership/provider;
- compatible installed/missing cores and the preferred core selector;
- RetroAchievements and firmware state in player-facing wording;
- Install or Check for updates only when the connected client and package
  manager advertise the typed setup capability; and
- concise guidance for user-owned firmware or RetroArch core installation.

Install/update is an explicit confirmation action. The UI follows the device
command and refreshes inventory/configuration on completion. It never implies
that installing an emulator also supplies games, firmware, achievement
credentials, or compatible saves.

## Compatibility, persistence, and rollback

- Migrations 22 and 23 are immutable. Migration 24 additively preserves package
  manager facts; rollback leaves the new column unused.
- Old clients continue to send inventory schema 1 and do not advertise
  `emulator.setup`; the UI shows discovery facts as unknown and no managed action.
- New clients accept existing schema-1 server state and replace it with schema 2
  on the next inventory report.
- Existing ScummVM, native, browser, cloud, and installed-game routes remain
  compatible. No existing emulator default is rewritten.
- Trusted-LAN HTTP and remote MGA Server origins remain supported exactly as in
  ADR-0015.

## Security and failure behavior

- All IDs and actions are validated against fixed catalogs at both ends.
- Package-manager output is bounded and never interpreted as executable input.
- Cancellation terminates only the client-owned package-manager process; MGA
  reports failure and refreshes inventory rather than guessing rollback state.
- Core discovery rejects directory escapes and excessive component counts.
- Firmware paths, hashes, credentials, and package-manager arguments are not
  exposed to the browser.

## Acceptance criteria

- Schema-1 and schema-2 inventory validation/normalization tests pass.
- Migrations 23 and 24 are additive, tested, applied once, and preserve prior
  migrations.
- RetroArch discovery returns bounded core IDs without persisting module paths.
- Server configuration returns selected/resolved cores, firmware state, and
  capability facts per emulator/core/platform.
- Owners can select/reset a core; viewers cannot; alternative cores remain.
- `emulator.setup` rejects unknown emulators/actions and every caller-controlled
  URL/package/path/argument shape; owner authorization and client capability are
  tested.
- The client invokes only exact allow-listed package-manager commands through an
  injectable tested runner and refreshes inventory afterward.
- Settings shows install/update/core/firmware/RetroAchievements state without
  exposing filesystem paths or promising inaccessible saves.
- ScummVM still launches end to end. RetroArch launch is labelled ready only for
  a detected compatible core, deterministic content entry point, and satisfied
  firmware policy.
- Full protocol/client/server/plugin/frontend tests, packaging, and packaged
  server/installed-client E2E are recorded in the authoritative handoff.

## Upstream evidence

- RetroAchievements emulator/core support:
  <https://docs.retroachievements.org/general/emulator-support-and-issues.html>
- RetroAchievements unsupported-core evidence:
  <https://docs.retroachievements.org/developer-docs/unsupported-emulators-and-cores.html>
- RetroArch directory semantics:
  <https://docs.libretro.com/guides/change-directories/>
- RetroArch achievements integration:
  <https://www.retroarch.com/achievements.php>
