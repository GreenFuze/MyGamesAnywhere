# ADR-0006: Managed archive installation

Status: Accepted (ZIP/7z/RAR installation and native launch implemented)

## Context

MGA needs to install games on a selected device/user endpoint without giving the
web server arbitrary filesystem or shell authority. The current installer
library contains distinct ZIP/7z/RAR archives, GOG-style EXE/BIN installers,
and storefront-owned products. These families require separate typed commands.

## Decision

The first command family is `game.install_archive`, schema version 1. ZIP, 7z,
and RAR use the same server/UI contract. The client bundles pinned pure-Go
readers (`bodgit/sevenzip` and `nwaples/rardecode`) rather than requiring a
machine-wide extractor. EXE/BIN and storefront installs remain separate command
families; the first signed GOG Inno Setup packet is defined by
[ADR-0007](0007-locally-confirmed-gog-inno-installation.md).

The server selects a concrete canonical game, source record, and archive file.
It creates a random 256-bit, time-limited bearer grant and sends a typed command
to an authenticated endpoint with Manage access. The fixed transfer endpoint
exposes one file only when that grant is supplied in its Authorization header.
The grant carries no profile session credential, is redacted from persisted
command history, is not an installation record, and expires after twelve hours.
Grants are currently held in memory, so a server restart intentionally
invalidates an in-flight transfer.

The server sends an origin-relative transfer path. The client resolves it
against its own paired-server address and accepts only HTTP(S) transfers on
that origin. This avoids treating the browser's hostname as the device's
pairing address. The client then:

1. expands the destination template in the endpoint user's environment;
2. verifies free space before download and extraction;
3. downloads into an MGA staging directory while hashing the archive;
4. rejects traversal, symbolic links, unsupported non-regular entries, excessive
   entry counts, unknown/overflowing sizes, encrypted archives, and multi-volume
   archives;
5. extracts without executing archive contents;
6. writes a schema-versioned `.mga-install.json` manifest;
7. atomically renames staging into the final game directory.

Failure or cancellation removes staging and leaves the final destination
unchanged. Uninstall requires a matching manifest and refuses paths outside the
manifest's recorded MGA root.

Download and installation are separate progress stages from the same client
command stream. Download reports 0–100 percent and maps to overall 0–40;
archive checking, extraction, launch discovery, and finalization report Install
0–100 and map to overall 40–100. The web interface shows Download in blue/cyan
and Install in purple while retaining overall percent for protocol compatibility.

After extraction, the client recursively discovers regular `.exe` files and
excludes setup, redistributable, updater, crash-handler, helper, and uninstall
executables. It automatically chooses a target only when there is one candidate
or one candidate has a clear title/path score advantage. Ambiguous candidates
remain unselected for the player to choose.

`game.launch` requires Play access. The server dispatches only the launch target
recorded for that installation, and target changes require Manage access and
must match a recorded candidate. The client independently validates manifest
schema 2, game/source identity, candidate membership, normalized relative path,
resolved directory containment, regular-file status, and `.exe` extension
before starting the process with the executable's directory as its working
directory. Archive contents are never executed as part of installation.

## Destination settings

The long-term preference is a profile-owned path template, defaulting to
`%USERPROFILE%\Games`, in a future **My Settings** group. Variables are expanded
only by the selected client. A device/user endpoint may override the profile
template, and an individual install may override both. This is a two-layer model
because one MGA profile can use several endpoints with different filesystems.

The current UI exposes the per-install override. Local testing on the current
endpoint uses `C:\Games` as selected by the user.

## Source delivery

Direct server-local sources are streamed directly. Google Drive API records can
use the local Google Drive for desktop mirror when the server is started with
`MGA_GOOGLE_DRIVE_DESKTOP_ROOT` (for example `G:\My Drive`). Paths are resolved
beneath that root and traversal is rejected. A provider-direct delivery adapter
can later replace this without changing the client command.

## External-removal reconciliation

[ADR-0011](0011-device-installation-reconciliation.md) implements read-only
reconciliation through the selected endpoint's MGA Client. Scheduled and manual
checks share the same bounded `game.validate_installations` command, client
validator, result applier, events, and UI state. The server no longer presents
a game as playable solely because `device_game_installations` contains an old
successful row.

The implemented states distinguish:

- **Installed:** managed directory, matching manifest, and selected launch target
  are present.
- **Missing:** the managed directory was removed outside MGA.
- **Needs repair:** the directory remains but its manifest, selected executable,
  or other managed files are missing or inconsistent.

Missing or damaged reports disable Play, remain visible in device/game details,
and produce low-noise transition notifications. Detection preserves audit
history and never repairs, deletes, or forgets anything automatically. The
client remains the filesystem authority; the server owns persisted state,
timestamps, reason codes, and UI history. Repair, reinstall, cleanup, and Forget
actions remain deferred.

## Persistence and migrations

Server migration 15 adds command progress columns and
`device_game_installations`. Successful install/uninstall results update this
read model atomically with the terminal command.

Server migration 16 additively adds staged command progress plus launch target
and candidate fields. Existing migration-15 rows receive null stage/target
values and an empty candidate list, so they remain valid and become playable
after reinstalling with an updated client.

Server migration 20 additively records verification reason/details and extends
installation audit events with Missing, Needs repair, and restored transitions.
It preserves existing installation/event rows and all migration-17 cleanup
state. See ADR-0011 for the exact validation and compatibility rules.

New installed directories contain manifest schema version 2 with launch target
metadata. Uninstall intentionally accepts manifest schemas 1 and 2, preserving
safe removal of existing managed installations. Launch requires schema 2 and
never guesses a target for a schema-1 installation.

`NO_MIGRATION_NEEDED` for 7z/RAR support: the command schema already carries
`archive_format`, and server migration 16 plus manifest schema 2 already hold
all required progress and launch metadata. Existing SQLite rows, client
configuration, pairing identity, and managed installations remain valid.

`NO_MIGRATION_NEEDED` for `MGA_GOOGLE_DRIVE_DESKTOP_ROOT`: it is an optional
runtime environment value and adds no persisted config field. Existing installs
continue to work when it is absent.

## Consequences

- MGA gains progress, safe rollback, installed-state reporting, and guarded
  uninstall without permanent elevation.
- Duplicate source copies remain independently installable.
- ZIP, 7z, and RAR share one typed workflow while retaining format-specific
  readers behind a common guarded extraction boundary.
- Provider-direct streaming, shortcuts, launch arguments/working-directory
  overrides, prerequisites, save-path discovery, repair, and updates remain
  later slices.
