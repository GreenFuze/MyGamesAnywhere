# ADR-0011: Device installation reconciliation

- **Status:** Implemented and fully verified
- **Date:** 2026-07-16
- **Scope:** MGA-managed installation validation, automatic checks, manual checks,
  player-visible state, and notification history

## Context

MGA records successful installations on an exact `(profile, endpoint, game,
source copy)` identity, but files and publisher registrations can later be
removed or damaged outside MGA. A database row must not remain playable merely
because an earlier install succeeded. Manual and automatic checks must not
diverge, and validation must not become an arbitrary filesystem inventory.

## Decision

### One typed validation path

Add `game.validate_installations`, schema 1. The server builds a bounded batch
from the requesting profile's existing MGA installation rows. The client uses
one `InstallationValidator` for both manual and automatic commands. The command
requires View access, is read-only, needs no elevation or local confirmation,
and contains no arbitrary glob, executable, registry key, or shell input.

Only `installed`, `missing`, and `needs_repair` rows are eligible. Failed-install
cleanup, attention, ignored, and cleanup-running rows are never validated or
overwritten by this flow.

### Validation rules

Every item validates the exact absolute install path, MGA manifest identity,
manifest schema, install kind, and recorded launch target when one exists.

- **Installed:** directory and matching supported MGA manifest exist; a recorded
  launch target exists as a regular file. GOG Inno also has its recorded
  uninstaller and an Add/Remove Programs association.
- **Missing:** the managed directory is absent. For GOG Inno, both its directory
  and Add/Remove Programs association must be absent.
- **Needs repair:** some installation evidence remains but the manifest is
  missing/invalid/mismatched, a recorded executable or uninstaller is missing,
  an unsafe reparse boundary is encountered, or GOG files and publisher
  registration disagree.

No hash of installed game content is recomputed. No files, manifests, registry
entries, or database rows are deleted or repaired automatically.

The client returns one result for every requested identity, with a strict reason
code and check time. Unknown kinds, duplicate identities, omitted results, and
unsafe paths fail fast rather than producing a false Missing state.

### Server state and audit

The server accepts results only for the original endpoint/profile/identity batch
and updates only eligible rows. It records `last_verified_at`, a stable
verification reason, and bounded non-sensitive details. State transitions emit
`installation_missing`, `installation_needs_repair`, or
`installation_restored` audit events. Repeated identical checks update the last
verification time without adding duplicate transition events.

Missing and Needs repair rows are excluded from Installed Games and cannot be
launched. They remain visible on game/device details and count in the selected
device's attention link. A restored result returns the row to Installed.

### Automatic and manual triggers

Automatic validation is profile-scoped, enabled by default, first attempts one
minute after server startup, and defaults to every 15 minutes. The player can
pause it or choose 5 minutes through 24 hours in Settings > Devices.

The scheduler and **Check installed games** button call the same coordinator,
server request builder, typed command, client validator, result applier, and
events. One validation command per profile/endpoint runs at a time. Offline,
update-required, incompatible, busy, or no-installation endpoints are visibly
waiting/skipped and are not queued for later execution.

Settings > Devices shows scheduled/running/paused/waiting/failed state, next and
last check, command progress, and per-installation verification state. State
changes publish profile-scoped SSE events. Browser notification history records
only new Missing, Needs repair, restored, or failed outcomes; unchanged healthy
background checks do not create noise.

## Protocol and security boundaries

- Maximum 256 items per command; larger inventories are rejected until a later
  pagination decision.
- The client reads only the exact path and manifest/recorded relative targets in
  each typed item plus the narrow Add/Remove Programs association check already
  used by ADR-0007.
- Reparse points at the installation boundary or recorded target fail closed as
  Needs repair.
- Results cannot add, remove, retarget, or change ownership of an installation.
- The server never infers device identity from host name, user name, network, or
  physical-machine grouping.

## Persistence and migration

Migration 20 is additive except for rebuilding the installation-event table to
extend its event-type constraint while preserving every existing row. It adds
`verification_reason_code` and `verification_details_json` to
`device_game_installations`, then allows the three reconciliation transition
events. Existing rows begin with no verification reason and `{}` details; their
state and cleanup metadata are unchanged. Migration 17 remains immutable.

`NO_MIGRATION_NEEDED` for the optional profile setting
`installation_validation_schedule`: settings are already version-tolerant
key/value JSON, and profiles without the key receive the safe defaults above.

## Required verification

- protocol request/result validation and duplicate/omission rejection;
- archive/GOG client validator tests for healthy, missing, damaged, unsafe, and
  publisher-registration disagreement states;
- migration 19 to 20 upgrade preserving legacy Duke and all existing events;
- transactional server result application, authorization/isolation, protected
  failed-state behavior, restoration, and idempotent events;
- shared manual/background coordinator tests including offline and no-work;
- frontend status/reason rendering, schedule control, notification filtering,
  and Installed Games exclusion;
- full protocol/client/server/plugin tests, frontend unit/production build,
  OpenAPI generation, `govulncheck`, packaging, and `git diff --check`;
- packaged server plus installed client E2E: validate Plasma Pong as healthy,
  use a unique synthetic managed installation to prove Missing and restoration
  without altering Plasma Pong or the preserved legacy Duke tree, remove only
  the synthetic fixture afterward, and leave MGA running.

## Verification completed

The packaged Windows E2E completed on 17 July 2026 with the packaged server at
schema 20 and the installed client connected in elevated mode through the real
browser-to-`mga://` launch path.

- Plasma Pong was validated Healthy before, during, and after the test.
- The isolated `adr11-e2e-20260717` / `MGA ADR11 E2E 20260717` managed-archive
  fixture validated Healthy, then Missing after only its exact install directory
  was moved aside, and Healthy again after that directory was restored.
- The server recorded exactly one `installation_missing` transition with
  `install_path_missing` and one `installation_restored` transition with
  `healthy`; the initial and repeated Healthy results created no noisy event.
- Settings > Devices showed Ready, Missing with the player-facing explanation,
  and Ready again. Chrome showed the matching notification toast, unread count,
  and history entries for `1 missing` and `1 restored`.
- The synthetic installation, events, source/canonical rows, and filesystem tree
  were removed afterward. The real database reports zero remaining synthetic
  rows.
- The legacy Duke row remains `attention_required` without a cleanup marker, the
  historical marked cleanup-failed row remains protected, and Plasma Pong
  remains Installed and Healthy.
- Fresh protocol/client/server tests, all seven standalone plugin-module tests,
  frontend unit and production builds, OpenAPI generators/checks, protocol/
  client/server `govulncheck`, and `git diff --check` passed. The existing Vite
  chunk-size warning remains non-fatal.

## Deferred

- automatic repair, reinstall, cleanup, or forgetting;
- content hashing and storefront-owned installs not created by MGA;
- pagination beyond 256 installations per profile/endpoint;
- physical-host grouping and cross-endpoint aggregation;
- emulator/prerequisite reconciliation and save-data health checks.
