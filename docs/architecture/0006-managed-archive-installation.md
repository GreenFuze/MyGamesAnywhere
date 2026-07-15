# ADR-0006: Managed archive installation

Status: Accepted (ZIP installation and native launch slice implemented)

## Context

MGA needs to install games on a selected device/user endpoint without giving the
web server arbitrary filesystem or shell authority. The current installer
library contains distinct ZIP/7z/RAR archives, GOG-style EXE/BIN installers,
and storefront-owned products. These families require separate typed commands.

## Decision

The first command family is `game.install_archive`, schema version 1. ZIP is the
first guaranteed format. 7z and RAR will use the same server/UI contract after
their extractors have equivalent path, link, disk, cancellation, and rollback
guards. EXE/BIN and storefront installs remain separate future command families.

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
4. rejects ZIP traversal paths and symbolic links;
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

## Persistence and migrations

Server migration 15 adds command progress columns and
`device_game_installations`. Successful install/uninstall results update this
read model atomically with the terminal command.

Server migration 16 additively adds staged command progress plus launch target
and candidate fields. Existing migration-15 rows receive null stage/target
values and an empty candidate list, so they remain valid and become playable
after reinstalling with an updated client.

New installed directories contain manifest schema version 2 with launch target
metadata. Uninstall intentionally accepts manifest schemas 1 and 2, preserving
safe removal of existing managed installations. Launch requires schema 2 and
never guesses a target for a schema-1 installation.

`NO_MIGRATION_NEEDED` for `MGA_GOOGLE_DRIVE_DESKTOP_ROOT`: it is an optional
runtime environment value and adds no persisted config field. Existing installs
continue to work when it is absent.

## Consequences

- MGA gains progress, safe rollback, installed-state reporting, and guarded
  uninstall without permanent elevation.
- Duplicate source copies remain independently installable.
- ZIP ships before 7z/RAR rather than treating unlike formats as equivalent.
- Provider-direct streaming, shortcuts, launch arguments/working-directory
  overrides, prerequisites, save-path discovery, repair, and updates remain
  later slices.
