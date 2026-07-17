# ADR-0012: Profile and device install-location preferences

- **Status:** Implemented and locally verified
- **Date:** 2026-07-17
- **Scope:** Installation destination selection for existing typed installers

## Context

MGA already accepts a destination-root template for managed archives and the
signed GOG Inno family. The web dialog currently starts every installation at
`%USERPROFILE%\Games`, so a player who consistently uses another location must
re-enter it. MGA Server may run on another LAN computer, which means it cannot
resolve a path using its own environment or filesystem.

An MGA device is a device/OS-user endpoint. A useful default therefore needs
both a player preference and an optional endpoint-specific choice while still
allowing one installation to use another location.

## Decision

MGA resolves the destination-root template in this order:

1. a non-empty override confirmed in the current Install dialog;
2. the selected device/user endpoint override;
3. the signed-in profile's **My Settings** default;
4. `%USERPROFILE%\Games`.

The UI shows the effective value and its source. Clearing a profile value
restores `%USERPROFILE%\Games`; clearing an endpoint value restores inheritance
from the active profile. Profile preferences belong to the signed-in profile
and are editable by that profile. Because an endpoint override affects every
profile allowed to install on that endpoint, changing it requires Owner access.
Manage access remains required to perform an installation.

The stored value is a template, not a server path. MGA Server validates only a
bounded, non-control-character template and sends the resolved template in the
existing typed install request. MGA Client expands Windows `%NAME%` variables
using the target endpoint process's OS-user environment, rejects unknown or
empty variables, requires the result to be absolute, and applies the existing
root/destination boundary and disk-space checks. MGA Server must never expand
the value or assume it shares a filesystem, host, drive letters, or OS user
with the endpoint.

The fallback remains `%USERPROFILE%\Games` even when MGA Server is reached over
plain HTTP on another trusted-LAN computer. Transport choice does not change
path ownership or expansion semantics.

## Persistence and compatibility

Migration 21 creates `device_install_preferences`, keyed by `endpoint_id` with
an `ON DELETE CASCADE` foreign key, the root template, updating profile, and
timestamp. Absence means the endpoint inherits the active profile preference.

The profile default uses the existing extensible `profile_settings` table with
key `install_root_template`. `NO_MIGRATION_NEEDED` for that key: existing rows
and schemas remain valid, absence has the explicit fallback above, and the
table already stores additive profile-scoped settings. Migration 21 does not
backfill endpoint rows, so all existing endpoints retain current behavior.

## API and UI

- `GET/PUT /api/install-preferences/profile` reads or changes the active
  profile's default.
- `GET/PUT /api/devices/{id}/install-preference` returns the endpoint override,
  profile default, effective template, and provenance; PUT sets or clears the
  endpoint override with Owner authorization.
- Settings gains a player-facing **My Settings** tab. The expanded device card
  exposes the endpoint override only to an Owner.
- The Install dialog loads the effective template for its selected endpoint,
  remains editable, and sends a per-install override only after confirmation.

## Non-goals

- Discovering or installing standalone prerequisites.
- Choosing installer families beyond the already accepted typed families.
- Expanding server-side environment variables or browsing the endpoint's
  filesystem from the browser.
- Moving existing installations when a preference changes.

## Verification — 17 July 2026

The packaged server applied migration 21 to the real portable database and ran
the production frontend from `server\bin`. Browser E2E verified that:

- **My Settings** loaded `%USERPROFILE%\Games`, persisted a temporary
  `%USERPROFILE%\MGA ADR12 Profile E2E` value, then cleared it back to the
  standard fallback;
- the connected `tc-pc / TC-PC\tcs` Owner device card inherited that profile
  default, then persisted the intended endpoint override `C:\Games` and showed
  that the device uses its own folder;
- the top bar continued to report the installed client as elevated and Ready;
- the existing Plasma Pong and legacy attention installations remained
  untouched.

Backend tests cover precedence, access levels, invalid templates, repository
set/clear behavior, migration non-backfill/cascade behavior, and the existing
client-side environment expansion. OpenAPI generation, frontend type-check,
unit tests, and the production build also pass.
