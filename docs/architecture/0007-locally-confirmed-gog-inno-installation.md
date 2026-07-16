# ADR-0007: Web-authorized GOG Inno Setup installation

- **Status:** Accepted; implementation foundation committed in `1e59e51`;
  web-authorized install, crash-after-success, and failed-cleanup revisions
  remain to implement. The Installed Games shelf is specified by ADR-0008.
- **Date:** 2026-07-15
- **Scope:** First EXE/BIN installer and prerequisite vertical slice
- **Note:** ADR = Architecture Decision Record.

## Context

MGA now installs ZIP/7z/RAR packages without executing their contents. The
library also contains GOG offline bundles shaped as one small
`setup_*.exe` plus zero or more large, same-prefix `setup_*-N.bin` files.
Executing an installer is materially different from archive extraction:

- it runs publisher code and may modify registry/system locations;
- it may request one-time elevation and install shared prerequisites;
- it cannot be made transactionally atomic by renaming a staging directory;
- arbitrary executable paths or user-supplied arguments would become a remote
  shell surface;
- an installer may leave partial files or continue running after MGA loses
  contact.

The first slice therefore supports one positively identified family rather than
introducing a generic EXE command.

## Decision

### First supported family

The first family is a **signed GOG offline installer using Inno Setup**:

- exactly one regular Windows PE file named `setup_*.exe`;
- zero or more regular companion files with the same setup stem and names shaped
  as `setup_*-N.bin`;
- a valid Authenticode chain whose leaf subject organization/common name is
  `GOG Sp. z o.o.`;
- positive Inno Setup identification by a bounded client detector;
- a base-game source record, not DLC/add-on, patch, language pack, or unknown
  content.

The current library contains representative packages matching this shape,
including signed single-file and EXE/BIN bundles. Extension alone is never
enough: unrelated DOS executables, game binaries, PS3 `EBOOT.BIN`, unsigned
setups, multiple EXEs, mismatched BIN files, and unsupported publishers are
rejected.

This family identifier is `gog_inno`. Adding another publisher/family requires a
new architecture decision, detector, fixed invocation policy, tests, and
player-facing behavior.

#### Recognition authority

The current two-layer policy is final for this slice:

1. **Server candidate filtering** is structural only: one `setup_*.exe`, zero or
   more same-stem `setup_*-N.bin` files, supported base-game content, and no
   ambiguous EXE set.
2. **MGA Client is the execution trust boundary** immediately before process
   start: `WinVerifyTrust` must validate the Authenticode chain, the leaf
   organization/common name must be `GOG Sp. z o.o.`, and the bounded detector
   must find the Inno Setup marker.

The server does not claim a filename is verified GOG, download remote files for
preflight, duplicate Authenticode trust, or persist a manual “trust this EXE”
override. This keeps behavior identical for Google Drive, SMB, and future
delivery adapters. Player-facing UI may call it a **GOG installer candidate**
until the client verifies it; audit/result data records the verified signer and
family afterward.

Adding server-side signature preflight, source tags, manual overrides, another
signer, or another family is out of scope and requires architecture escalation.

### Web-authorized silent policy

Installation is silent after explicit authenticated web consent:

1. The web Install dialog shows game, device, connection/package, and exact
   destination. The authenticated profile confirms **Install** and MGA Server
   enforces Manage access.
2. MGA Server resolves only the recorded source files and sends typed transfer
   descriptors; it sends no executable path or arguments.
3. MGA Client downloads and hashes every package file into command-owned staging.
4. The client validates PE shape, GOG Authenticode identity, Inno family,
   companion names, destination, and disk space.
5. The client invokes fixed Inno arguments:
   `/SP- /VERYSILENT /SUPPRESSMSGBOXES /NORESTART /DIR=<destination>` plus a
   command-owned `/LOG=<bounded staging log>`.

MGA Client does **not** show a separate native “Approve installation” popup.
The authenticated web action is the product consent boundary, matching archive,
Steam, and Xbox-style install behavior. This is safe only because the command
remains Manage-authorized, source-bound, GOG-signed, Inno-validated, and
argument-fixed. Windows UAC may still appear when Windows requires elevation;
MGA cannot suppress or automate it.

No payload, API, config, or advanced UI may add arbitrary arguments or restore
an unattended confirmation-bypass switch—the normal flow already has no MGA
native prompt. The Install dialog explains that Windows permission may appear;
progress says **Verifying installer**, **Starting installer**, and **Installer
running**. It does not say **Approve on device** or advertise “silent install.”

Interactive installer wizards, custom flags, scheduled/offline installs without
an active authenticated web request, and changing destination inside a wizard
are out of scope.

### Elevation

MGA Client remains a non-elevated per-user process and never becomes a service.

- Start normally first.
- If Windows reports elevation is required, the client may use
  `ShellExecuteEx` with verb `runas` for this one validated staged GOG installer,
  the same fixed arguments, and a retained process handle.
- Windows must display UAC and the local OS user must approve it.
- UAC decline is a typed failure and is never retried automatically.
- The client never passes its endpoint key, transfer token, session credential,
  or general command capability to the elevated installer.
- No custom elevation helper is introduced in this slice.

Any operation requiring an elevated MGA helper, service, scheduled task,
credential prompt automation, or broader executable authority is an escalation
and requires a separate security decision.

### Typed protocol commands

Protocol v1 adds:

- capability/command `game.install_gog_inno`, schema 1, minimum **Manage**;
- capability/command `game.uninstall_gog_inno`, schema 1, minimum **Manage**;
- capability/command `game.cleanup_gog_inno_failed`, schema 1, minimum
  **Manage**.

The install request contains only:

- game/source identity and title;
- destination root/name;
- one typed `installer` transfer descriptor;
- bounded typed `companions` transfer descriptors.

Each transfer descriptor contains basename, role, byte size, origin-relative
download path, and bearer token. Basenames must be unique case-insensitively,
one path segment, and match the fixed family rules. Maximum companion count is
64. There is no path, URL origin, executable name, argument, environment,
working-directory, shell, script, or prerequisite field controlled by the web
caller.

The install result contains:

- game/source identity, install root/path, completion time;
- family `gog_inno`;
- primary SHA-256, total package bytes, and every file name/size/SHA-256;
- verified signer subject/thumbprint;
- fixed invocation mode;
- detected relative uninstaller target;
- launch target/candidates using the existing safe discovery rules;
- process ID, exit code, and bounded diagnostic reference (not raw log content).

The uninstall request is constructed only from persisted installation metadata
and contains game/source identity, install path, family, and recorded relative
uninstaller target. It cannot supply another executable or arguments.

The failed-cleanup request is also constructed only from persisted installation
metadata. It contains game/source identity, install root/path, family, recorded
cleanup-marker ID, and recorded uninstaller target when one was positively
discovered. It cannot supply a different path, executable, arguments, deletion
root, or cleanup policy.

The cleanup result contains only game/source identity, `removed`,
`publisher_uninstaller_used`, `bounded_delete_used`,
`system_changes_may_remain`, `leftover_directory`, optional process ID/exit
code/diagnostic reference, and a bounded cleanup summary. `removed` means MGA
removed the active failed-install record after its authorized filesystem work;
it never claims registry/shared-prerequisite rollback.

**Ignore** is an authenticated server-side Manage action, not a device command.
It changes only the persisted failed-install state/audit metadata and never
touches the endpoint filesystem.

Authenticated HTTP surfaces are fixed as:

- `POST /api/devices/{id}/games/{game_id}/sources/{source_game_id}/cleanup-failed`
- `POST /api/devices/{id}/games/{game_id}/sources/{source_game_id}/ignore-failed`
- `POST /api/devices/{id}/games/{game_id}/sources/{source_game_id}/reopen-failed-cleanup`

All require Manage, decode the source ID safely, accept an empty JSON object,
and derive every identity/path/marker value from persistence. Cleanup returns
the dispatched device command; Ignore/Reopen return the updated installation
state. Retry uses cleanup first and then the existing install flow; it has no
combined device command.

Bearer tokens are redacted from command audit persistence for every package
file. Successful results are type-checked against command identity before
persistence.

### Progress, local action, failure, and audit

The existing `percent`, `stage`, and `stage_percent` fields remain compatible.

- **Download** reports real aggregate bytes across EXE/BIN files, 0–100, blue.
- **Install** reports validation/execution/finalization milestones, purple.
- While the external installer runs, Install is indeterminate; MGA must not
  fabricate a percentage it cannot observe.
- Install becomes 100 only after post-install validation plus either exit code
  0 or the narrowly accepted GOG post-success deinitialization crash below.

Visible phases/messages are:

- Preparing installer
- Downloading installer
- Verifying publisher
- Starting installer
- Installer running
- Checking installed game
- Installed / Couldn't install

Typed failure codes include:

- `unsupported_installer`
- `invalid_installer_signature`
- `invalid_companion_set`
- `local_confirmation_declined`
- `local_confirmation_timeout`
- `uac_declined`
- `installer_start_failed`
- `installer_timeout`
- `installer_exit_nonzero`
- `install_validation_failed`
- `uninstaller_missing`
- `uninstaller_mismatch`
- `uninstaller_exit_nonzero`
- `cleanup_marker_missing`
- `cleanup_marker_mismatch`
- `cleanup_uninstaller_failed`
- `cleanup_boundary_failed`
- `cleanup_failed`

`local_confirmation_declined` and `local_confirmation_timeout` remain valid for
publisher uninstall and failed-install cleanup only; install no longer emits
them.

Server audit records command identity, family, safe package metadata, profile,
endpoint, destination, timing, progress, signer summary, PID/exit code, and
terminal result. It never stores bearer tokens, command-owned log contents,
environment dumps, or credentials.

### Narrow crash-after-success completion

GOG Duke Nukem 3D provides reproducible evidence that an otherwise completed
silent install can crash during Inno deinitialization:

- the bounded installer log contains `Installation process succeeded`;
- destination files and exactly one `unins*.exe` are present;
- then the installer exits `0xC000041D`
  (`STATUS_FATAL_USER_CALLBACK_EXCEPTION`) from its temporary setup process.

This exact outcome is treated as **Installed**, not a Play failure or
`attention_required`, only when every condition below is true:

1. The package already passed the locked GOG Authenticode + Inno verification
   and fixed-argument policy.
2. The process exit value, compared as unsigned 32-bit, is exactly
   `0xC000041D` (signed Windows value `-1073740771`).
3. A bounded read of the command-owned Inno log contains the exact
   case-insensitive success sentinel `Installation process succeeded`. The log
   must not contain case-insensitive `Installation process failed`,
   `Rolling back changes`, or `Rollback failed`. The known post-success
   access-violation message does not by itself negate the accepted deinit crash.
4. The intended destination exists within its recorded root.
5. Exactly one regular relative Inno uninstaller is present.
6. Safe launch discovery and all normal schema-3 result/manifest validation
   pass.

The log parser reads at most the final 1 MiB, decodes UTF-16LE when a BOM/NUL
pattern indicates it and otherwise UTF-8, and performs only the fixed
case-insensitive sentinel checks above. Missing or unreadable diagnostics fail
closed; no regex/category inference is allowed.

The result and schema-3 manifest add `completion_basis`, allow-listed as:

- `exit_zero`
- `validated_post_success_crash`

The original non-zero `exit_code`, PID, diagnostic reference, signer, hashes,
and package metadata remain unchanged in result/audit evidence. Install reaches
100 only after all validation succeeds. The allowlist applies only to
`game.install_gog_inno`; it never applies to uninstall or cleanup.

Any other non-zero code, missing/ambiguous uninstaller, missing sentinel,
post-validation failure, or different crash remains a failed installation.
There is no prefix/range/category matching and no “accept any crash if files
exist” fallback.

`NO_MIGRATION_NEEDED` for this completion basis: command results are JSON,
manifest schema 3 is not changed to a new schema number before release, and the
existing `state_reason`/command result retain the raw exit. Migration 18 below
adds cleanup persistence for true failures, not crash acceptance.

### Timeouts, cancellation, and retry

- Server command lifetime: 4 hours for install, 1 hour for uninstall.
- Failed-cleanup command lifetime: 1 hour.
- Native destructive confirmation timeout for uninstall/cleanup: 10 minutes.
- Installer process wait timeout: 2 hours.
- Uninstaller process wait timeout: 30 minutes.
- Transfer grants retain the existing 12-hour maximum.

Before process start, cancellation/context loss removes command staging and
performs no installation. Protocol v1 still has no remote cancellation message.

After installer/uninstaller start:

- MGA does not terminate an elevated/native installer automatically;
- connection loss, client restart, or timeout produces terminal command status
  `failed`, a typed error, and persisted installation state
  `attention_required` with PID when known;
- MGA never retries automatically;
- the UI tells the player the installer may still be running and to check the
  device.

Durable reconnect idempotency and cancel/kill semantics remain separate future
decisions.

### Commit and rollback semantics

Transactional guarantees end when the native installer starts.

#### Runtime correction — 15 July 2026: non-empty Inno destination

Packaged-client E2E against the signed Duke Nukem 3D installer established a
contradictory runtime fact: placing the schema-1 marker inside a newly-created
destination makes this Inno installer abort silently with exit `0x00000001`
before it writes its log or game files. The marker itself makes the requested
silent destination non-empty. This correction changes only marker placement;
it does not broaden executable trust, cleanup authority, or the accepted
post-success crash allowlist.

New markers therefore use schema 2 and are written atomically immediately
before process start at the deterministic command-bound sidecar path
`<install-root>/.mga/failed-installs/<marker-id>.json`. The client first
creates the otherwise-empty destination and records its Windows volume/file-ID
identity in the marker. Cleanup revalidates the sidecar's exact marker ID,
identity, root/path, command/game/source, family, and package hash, then
requires the current destination to have the same filesystem identity. A
replaced destination is refused. The sidecar is removed only after successful
installation or successful cleanup.

Existing schema-1 markers inside the destination remain readable solely for
cleanup compatibility; their in-destination location remains their replacement
defence. No server/database/protocol migration is required: persistence retains
only the already-version-neutral marker ID, and schema 2 is client filesystem
state.

#### Runtime safety addition — 16 July 2026: Windows registered-program check

Immediately before MGA uses its bounded direct deleter (whether no validated
uninstaller was discovered or a validated publisher uninstaller completed but
left files), MGA Client must inspect Windows Add/Remove Programs uninstall
registry views. A registry entry is associated only when its `InstallLocation`
normalizes exactly to the failed install path, or its parsed uninstall executable
is provably inside that exact path. Display-name-only matches are never enough.

If any associated entry remains, MGA must preserve the destination and return
the typed cleanup failure `cleanup_registered_program_present`; the player uses
Windows Apps & features/the publisher uninstaller instead. Registry access
errors other than a missing uninstall key also fail closed. MGA never executes
an untrusted registry command. A successfully run recorded publisher
uninstaller is followed by a fresh check before any direct deletion.

This is a Windows-client runtime inspection only. It changes no server DTO,
SQLite state, pairing/config/settings, or marker payload; therefore
`NO_MIGRATION_NEEDED`.

#### Runtime correction — 16 July 2026: cross-platform destructive prompt

Packaged tray-hosted E2E showed that MGA's custom Windows message-box and task
dialog implementations could exit abruptly or return an invisible Cancel. MGA
therefore no longer maintains a custom Win32 confirmation boundary.

The destructive prompt remains mandatory, bounded, local, and fail-closed.
[ADR-0010](0010-cross-platform-local-confirmation-dialogs.md) supersedes only
the transient rendering mechanism: MGA Client uses the cross-platform Zenity
adapter with foreground behavior, an explicit action/Cancel choice, Cancel as
the default, and the existing command timeout. The typed authorization,
verified installation metadata, and post-confirmation filesystem/process
safety rules in this ADR are unchanged.

`NO_MIGRATION_NEEDED`: this changes only transient local UI rendering; no
persisted protocol, database, marker, or configuration data changes.

- Download/validation/decline failures remove only command staging.
- The client creates a schema-versioned failed-install marker immediately before
  process start. New schema-2 markers use the command-bound sidecar above;
  legacy schema-1 markers remain in the destination only for cleanup
  compatibility. It records a random marker ID, command/game/source identity,
  install root/path, primary package hash, and creation time.
- The exact accepted crash-after-success outcome is committed as Installed and
  replaces the failure marker with the normal schema-3 manifest.
- A true post-start failure does not delete immediately. It persists
  `cleanup_required` when the marker is valid, or `attention_required` when no
  safe cleanup authority can be proven.
- Success requires destination existence, a detected uninstaller, and either a
  safe launch target or explicit no-launch-target state requiring player review.
- Package staging is removed only after the native process has terminated and
  required hashes/diagnostics are persisted.

Failure-marker schema 1 uses a random 256-bit base64url marker ID and contains
only:

- schema version;
- marker and command IDs;
- game/source identity;
- install root/path;
- installer family and primary package SHA-256;
- creation timestamp.

The marker is written atomically with user-only permissions where supported.
The marker ID is returned only in sanitized failure result/persistence; it is
not an authentication credential and is never accepted for another identity or
path.

### Failed-install cleanup and Ignore

Failure is categorized before offering cleanup:

1. **Pre-start failure:** no installer process ran; command staging is removed,
   the client removes its marker and newly created destination only when it
   still contains no non-marker entries, no installation row is retained, and
   normal Install/Retry remains available.
2. **Validated post-success crash:** treated as Installed under the exact
   allowlist above; cleanup is not offered.
3. **True post-start failure with valid cleanup marker:** state becomes
   `cleanup_required`; Play is blocked.
4. **Unknown/unsafe failure without valid marker:** state remains
   `attention_required`; automatic/bounded cleanup is unavailable.

After a true failure process terminates, the client performs a non-success
failure inventory: validate the marker/destination and record an uninstaller
target only if exactly one safe regular `unins*.exe` exists. It does not write
the schema-3 installed manifest, select a launch target, enable Play, or convert
the failure to Installed except through the exact crash allowlist above.

Every post-start failed command returns a sanitized typed failure payload with
game/source identity, install root/path, family, package hashes/sizes, signer,
process ID/exit code/diagnostic reference, cleanup-marker ID, and optional
validated uninstaller target. The server type-checks identity/boundaries before
persisting `cleanup_required` or `attention_required`; pre-start failures do not
create a failed installation row.

For `cleanup_required`, the web UI presents:

- **Clean up** — primary action;
- **Retry** — first performs the same cleanup; after cleanup succeeds it reopens
  Install but does not automatically execute another installer;
- **Ignore** — leaves files untouched and records an ignored failure.

Cleanup is never silently started at the instant of failure. The user must
choose Clean up/Retry and confirm the exact destination in the web UI. This
confirmation is required because game-folder files may be deleted. Native local
install confirmation does not exist. Publisher uninstall and failed cleanup
remain destructive operations and retain native confirmation in this packet.

Before cleanup filesystem/process work, MGA Client also shows the current
native destructive confirmation with the exact failed destination. Decline or
timeout leaves `cleanup_required` unchanged. Windows UAC may still appear for
the publisher uninstaller. Removing this local cleanup/uninstall confirmation
requires a separate architecture decision.

The typed client cleanup performs:

1. Revalidate endpoint/profile authorization, failed state, install
   root/path, random marker ID, command/game/source identity, package hash, and
   non-root containment.
2. Reject a replaced path, missing/mismatched/irregular marker, reparse-point
   root, or any boundary ambiguity.
3. If one recorded/uniquely validated Inno uninstaller exists, run it first
   using the existing fixed silent uninstall flags and UAC policy.
4. If that publisher uninstaller returns non-zero or times out, stop, preserve
   all files, and set `cleanup_failed`; do not fall through to deletion.
5. If no uninstaller exists, or after a successful uninstaller leaves files,
   remove only the marked destination tree with a no-follow/reparse-safe bounded
   deleter. It may remove reparse-point entries themselves but never traverse
   them.
6. Never delete the install root, a path existing before the command, registry
   data, files outside the marked destination, saves outside that destination,
   or shared/embedded prerequisites directly.

Successful cleanup removes the active failed-install row and records whether a
publisher uninstaller ran, whether marked leftovers were deleted, and that
external system changes/prerequisites may remain. Cleanup failure keeps the row
and exact actionable reason.

**Ignore** changes state to `ignored_failure`, records profile/time/audit event,
leaves the filesystem untouched, keeps Play blocked, and suppresses the active
cleanup warning. It is reversible: the UI retains **Clean up** / **Review
cleanup**. Retry to the same destination is unavailable until cleanup succeeds;
Ignore is not “accept as installed.”

Rows created before cleanup markers existed, including the current Duke
attention row, are not eligible for bounded folder deletion. They may be
ignored or cleaned manually by the user; the implementation must not synthesize
a marker retroactively.

### Uninstall semantics

Executable installations never use archive `RemoveAll` uninstall.

1. Server dispatches the recorded typed uninstall command.
2. Client validates manifest schema 3, identity, family, install boundary, and
   recorded uninstaller membership.
3. A native local prompt explains that the publisher uninstaller will run and
   saves/settings may remain.
4. Client runs the recorded Inno uninstaller with fixed
   `/VERYSILENT /SUPPRESSMSGBOXES /NORESTART`, using normal launch then one-time
   `runas` only if Windows requires it.
5. Exit code 0 removes the active installation read-model row and reports any
   remaining install directory as leftovers. MGA does not delete leftovers.
6. Failure keeps the installation record and reports attention required.

Manual uninstall outside MGA remains undetected until the separately planned
installation-reconciliation slice is implemented.

### Prerequisite ownership

Standalone prerequisite installation is **out of scope** for this slice.

- Prerequisites embedded and invoked by the verified GOG installer may run.
- MGA treats them as installer/external shared components.
- MGA never removes embedded/shared prerequisites directly during game
  uninstall or rollback.
- The publisher uninstaller decides what it owns; MGA does not claim ownership.

Future MGA-managed prerequisites are endpoint-scoped entities keyed by typed
prerequisite ID, version/range, architecture, and install evidence. Games refer
to them through separate relations. A prerequisite is removable only when MGA
installed it, no remaining game references it, and its typed uninstaller policy
allows removal. Existing/external prerequisites are never removed by MGA.

Implementing any standalone prerequisite requires a new protocol/persistence
packet; a regular agent must stop rather than add one to this slice.

## Persistence and migration 17

Migration 17, `executable_installation_state`, additively extends
`device_game_installations` with:

- `install_kind TEXT NOT NULL DEFAULT 'managed_archive'`
- `installer_family TEXT`
- `installer_files_json TEXT NOT NULL DEFAULT '[]'`
- `uninstall_target TEXT`
- `install_state TEXT NOT NULL DEFAULT 'installed'`
- `state_reason TEXT`
- `last_verified_at INTEGER`
- `state_changed_at INTEGER`

Application validation allow-lists:

- install kinds: `managed_archive`, `gog_inno`;
- states for this slice: `installed`, `attention_required`.

The broader planned reconciliation states `missing` and `needs_repair` are
reserved but not produced until that feature is implemented.

Existing migration-16 rows become `managed_archive` + `installed`, retain all
paths/hashes/candidates, remain launchable, and remain uninstallable through the
archive command. Existing SQLite/profile/device/source data is not rewritten.
Normal startup backup and migration checksum behavior remain mandatory.

For `gog_inno`, legacy aggregate fields store the primary installer SHA-256 and
total package bytes for API compatibility; `installer_files_json` is the
authoritative per-file record.

Manifest schema 3 is used only for `gog_inno` and records the same bounded
package/signer/family/uninstaller/launch metadata. Archive installs continue to
write schema 2; archive uninstall continues to accept schemas 1 and 2.

`NO_MIGRATION_NEEDED` for client `config.json`, pairing identity, settings-sync,
and save-sync payloads.

## Persistence and migration 18

Migration 17 has already been applied to the real development database and must
never be edited or re-checksummed. Failed-cleanup/Ignore uses additive migration
18, `failed_install_cleanup`:

- `device_game_installations.cleanup_marker_id TEXT`
- `device_game_installations.cleanup_ignored_at INTEGER`
- `device_game_installations.cleanup_ignored_by_profile_id TEXT`
- a new `device_installation_events` audit table:
  - `id TEXT PRIMARY KEY`
  - `endpoint_id TEXT NOT NULL`
  - `game_id TEXT NOT NULL`
  - `source_game_id TEXT NOT NULL`
  - `actor_profile_id TEXT`
  - `event_type TEXT NOT NULL`
  - `reason TEXT`
  - `details_json TEXT NOT NULL DEFAULT '{}'`
  - `created_at INTEGER NOT NULL`
- an index on installation identity/time for event history.

The event allowlist for this packet is:

- `failure_detected`
- `post_success_crash_accepted`
- `cleanup_started`
- `cleanup_succeeded`
- `cleanup_failed`
- `failure_ignored`
- `failure_reopened`

`details_json` contains only sanitized family, exit/completion basis,
publisher-uninstaller usage, bounded-delete result, and leftover summary. It
never stores bearer tokens, raw logs, credentials, or environment data.

Application state validation adds `cleanup_required`, `cleanup_running`,
`cleanup_failed`, and `ignored_failure` while preserving migration-17
`installed` and `attention_required`. Existing archive/GOG rows receive null
cleanup fields and remain unchanged. Existing attention rows without a marker
are deliberately ineligible for bounded deletion.

The failure marker is client filesystem state with its own schema version 1; it
does not change client `config.json`, pairing identity, settings-sync, or
save-sync payloads.

## Player-facing behavior

- Normal action: **Install**
- Client verification state: **Verifying installer**
- Native execution state: **Installer running**
- UAC explanation: “Windows may ask for permission on this device.”
- Uninstall confirmation: “The game’s installer will remove the game. Saves and
  settings may remain.”
- Unsupported package: “This installer isn’t supported yet.”
- Signature failure: “MGA couldn’t verify who published this installer.”
- Timeout: “The installer may still be running on `<device>`. Check the device
  before trying again.”
- True post-start failure: “The install didn’t finish. Clean up the game files
  before trying again.”
- Cleanup actions: **Clean up**, **Retry**, **Ignore**.
- Cleanup confirmation names the exact destination and explains that files in
  that game folder will be removed while Windows components may remain.
- Ignored state: “Failed install ignored. Game files may remain on `<device>`.”
- Accepted deinit crash is shown as normal **Installed**; the raw exit and
  completion basis remain only under Technical details/audit.

Technical details may show family, signer, package files, hashes, command ID,
exit code, and diagnostic reference. Tokens and raw logs are never exposed.

## Bounded implementation packet

### In scope

- protocol typed contracts/validation and capability constants;
- per-command lifetime policy and token redaction;
- migration 17 plus persistence/readback/API fields;
- server source-plan selection for one signed-GOG-shaped EXE plus matching BINs;
- same-origin multi-file transfer grants;
- client multi-file download/hash/disk planning;
- Authenticode and Inno detectors behind injected interfaces;
- no install confirmer/popup; injected native confirmation remains only for
  destructive uninstall/cleanup;
- remove `InstallConfirmationDetails`, `ConfirmInstall`, install MessageBox,
  Approve-on-device progress, and install-only local-confirmation errors/tests;
- fixed Inno process/elevation runner behind an injected interface;
- exact `0xC000041D` post-success completion classifier and bounded log parser;
- schema-3 manifest, safe launch discovery, and recorded Inno uninstaller;
- schema-1 failed-install marker, typed cleanup command, Ignore/reopen actions,
  migration 18, and installation event audit;
- publisher-uninstaller-first plus marker-authorized no-follow folder cleanup;
- typed install/uninstall routes and game-detail availability;
- player-facing UI/progress/errors;
- OpenAPI/contracts, tests, packaging, docs, and real packaged E2E.

Likely implementation surfaces include:

- `protocol/device/v1/installation.go`, command/connection tests;
- `client/internal/clientapp/agent.go` and new family-specific installer files;
- `client/internal/desktop` for destructive uninstall/cleanup confirmation;
- Windows-only signature/process adapters plus non-Windows unsupported adapters;
- `server/internal/devices`, `server/internal/db`, migration tests;
- `server/internal/http/archive_install_controller.go` or a dedicated executable
  installer controller, router, game detail, and tests;
- `server/frontend/src/api/client.ts`,
  `server/frontend/src/pages/GameDetailPage.tsx`, generated contracts/OpenAPI;
- installer packaging, ADR/protocol/handoff/release documentation.

### Out of scope

- arbitrary EXE/MSI/MSIX/script execution;
- unsigned or non-GOG publishers;
- interactive installer wizards or custom arguments;
- patches, DLC/add-ons, language packs, multiple EXEs, or ambiguous bundles;
- standalone prerequisites or MGA prerequisite removal;
- external-removal reconciliation, repair, update, or install queue;
- remote cancellation after native process start;
- automatic retry/reconnect replay;
- generic elevation helper/service/scheduled task;
- automatic cleanup without the user selecting Clean up/Retry;
- deletion without the exact marker/root/path/identity boundary;
- registry cleanup, direct prerequisite removal, or deletion outside the marked
  failed destination;
- changing profile/device default install roots.

### Required automated evidence

Protocol:

- request/result validation, basename/role/count/identity constraints;
- no argument/path/shell fields;
- access level and capability tests;
- all transfer tokens redacted.

Client:

- fake transfer, Authenticode, Inno detector, clock, and process runner; install
  tests assert no native confirmer is called, while uninstall/cleanup use a fake
  destructive confirmer; generic tests never execute an installer or request
  UAC;
- aggregate multi-file download progress and disk checks;
- fixed argument/working-directory assertions;
- invalid signature/publisher/family/companions and off-origin rejection;
- decline/timeout/UAC/non-zero/unknown-outcome behavior;
- exact crash allowlist: success sentinel + `0xC000041D` + full validation
  succeeds; missing sentinel, another exit, or failed validation remains failure;
- `completion_basis` and raw exit preserved in result/manifest;
- failure marker written before start and replaced only on committed success;
- cleanup marker identity/boundary/reparse checks;
- publisher-uninstaller success before leftovers deletion, uninstaller failure
  preserves files, no-uninstaller marker cleanup, and legacy no-marker refusal;
- schema-3 manifest, launch discovery, uninstaller membership/boundary;
- uninstall fixed arguments and no recursive deletion;
- existing ZIP/7z/RAR/schema-1/schema-2 tests remain green.

Server/database/HTTP:

- migration 16→17, applied 17→18, and fresh schema defaults/checksums;
- old archive-row compatibility;
- executable metadata round-trip;
- cleanup marker/Ignore metadata and installation-event round-trip;
- source selection rejects unrelated EXE/BIN and ambiguous bundles;
- encoded source IDs, authorization, install/uninstall/cleanup/Ignore routes,
  redaction, typed results;
- Ignore/reopen state transitions never dispatch a device command;
- game detail/UI capability and state mapping.

Build:

- `go test ./...` in protocol, client, and server;
- frontend production build;
- OpenAPI generation/test;
- `govulncheck ./...` for client with pinned fixed toolchain;
- `git diff --check`.

### Required packaged E2E

- Use packaged server and installed per-user client only.
- Preserve Google Drive Desktop configuration and existing credentials.
- Use a small representative signed GOG package, preferably
  `setup_duke_nukem_3d_1.5_(28044).exe` (about 39 MB), only after verifying the
  source still maps to the intended base game.
- Install to a unique `C:\Games\MGA E2E ...` destination.
- No MGA Client install-confirmation popup may appear. Automation stops only for
  Windows UAC and later destructive uninstall/cleanup confirmation.
- Capture persisted Download progress, Verifying-installer phase, indeterminate
  Installer-running state, signer/family/audit fields, accepted
  `0xC000041D` completion basis/raw exit when reproduced, command success,
  schema-3 manifest, launch candidates, and installed read model.
- Launch only a discovered game executable through `game.launch`, verify exact
  PID/path, and terminate only that exact test process if still running.
- Uninstall through typed publisher-uninstaller flow with local confirmation,
  verify no recursive MGA deletion, record leftovers, and clean temporary E2E
  database/source state.
- Exercise cleanup safely with temporary synthetic failed rows and
  schema-1 markers under unique `C:\Games\MGA E2E ...` folders:
  - no-uninstaller Clean up removes only the marked folder and row;
  - Ignore leaves files, records actor/time/event, blocks Play, and remains
    reversible;
  - legacy/no-marker cleanup is refused;
  - remove all synthetic rows/folders/events afterward.
- Leave packaged server/client running and preserve the existing Plasma Pong
  installation.

## Stop and escalation conditions

The implementation agent must stop before changing this decision if:

- the representative package is unsigned, not GOG-signed, or not positively
  identified as Inno;
- support requires arbitrary/custom arguments, another executable, script, MSI,
  or publisher;
- the source contains multiple EXEs or companion matching is ambiguous;
- elevation cannot be achieved with one validated `ShellExecuteEx` operation and
  normal Windows UAC;
- waiting for the elevated process cannot retain a safe exact process handle;
- a standalone prerequisite must be installed or removed;
- success cannot establish destination/uninstaller/identity safely;
- crash success would require another exit, missing log sentinel, or weaker
  post-install validation;
- cleanup would require deletion without the exact marker boundary, registry
  edits, direct prerequisite removal, or fallback after publisher-uninstaller
  failure;
- implementation restores/adds an MGA Client install confirmation or otherwise
  bypasses the authenticated web consent/signature/family boundary;
- external-removal reconciliation becomes necessary to claim correctness;
- protocol, migration, authorization, ownership, or player-policy behavior is
  needed beyond this packet;
- real E2E would require changing credentials, disabling security checks, or
  running an unverified package.

Use the escalation format in
`agent-responsibility-boundary.md`; do not broaden the command.
