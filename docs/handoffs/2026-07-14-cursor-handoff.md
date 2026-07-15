# MGA Cursor handoff — 14-07-2026

> **SUPERSEDED:** Continue from
> [`2026-07-15-cursor-handoff.md`](2026-07-15-cursor-handoff.md). This file is
> retained only as the pre-launch/progress implementation snapshot.

> **HISTORICAL WORKING HANDOFF (14-07-2026).** Read the superseding handoff before changing
> the current worktree. For implementation status, test status, runtime state,
> and next actions, this file overrides older roadmaps, release pages, screenshots,
> chat summaries, and unchecked task lists. Architecture ADRs remain authoritative
> for decisions unless this handoff explicitly supersedes them. Never discard the
> dirty worktree: it contains the user's active, uncommitted implementation.

## Mission and product direction

MyGamesAnywhere (MGA) is now a web-first, self-hosted game library and play
orchestrator: conceptually a fusion of LaunchBox, Playnite, Steam/Xbox PC,
LANCommander, local/cloud storefronts, emulators, browser play, achievements,
save sync, and device management. The old desktop UI direction was intentionally
abandoned. Device-local work is performed by the separate, windowless
`client/` MGA Client, while the main UI stays in `server/frontend/`.

The user wants a gamer-facing product, not an administrator console. Prefer short
labels, progressive disclosure, icons/status pills, and tooltips over verbose or
technical copy. Preserve distinct copies/editions/DLC; do not collapse merely
similar source games. The same game may appear in multiple integration groups.

## Non-negotiable project rules

Read the repository `AGENTS.md` first:

1. Fast-fail.
2. Prefer object-oriented design where practical.
3. Prefer SuitCode when available. It was not available in the Codex session;
   continue with normal repository tools if Cursor does not expose it.
4. Every persisted SQLite/JSON/config schema change needs a versioned migration
   or an explicit `NO_MIGRATION_NEEDED` safety note.

Additional user constraints:

- Work locally. **Do not publish a GitHub release, deploy to TV2, commit, push,
  reset, clean, or discard changes unless the user asks.**
- Never overwrite an existing profile password. Credentials are supplied
  out-of-band; `changeme` is only a bootstrap default for the first admin when
  no credential exists.
- Passwords/PINs are optional. PIN: at least 4 digits. Password: at least 4
  characters, no other complexity rule. Trusted LAN HTTP login is intentionally
  supported; do not reintroduce an HTTPS-only restriction.
- Credentials belong in Profiles/My Settings, never Devices. Passwords are
  server-side password hashes in SQLite, never plaintext.
- Device identity is `(physical device, OS user)`, represented by one client
  instance/endpoint. Another OS user runs/pairs a separate per-user client;
  the client must not require admin/service privileges.
- Preserve unrelated changes. The worktree is intentionally very dirty.

## Documentation precedence

Use documents in this order:

1. This handoff for live status and immediate next steps.
2. `docs/architecture/0001-mga-client-architecture.md` and
   `docs/architecture/mga-device-protocol-v1.md` for device architecture/protocol.
3. ADRs `0002`–`0006`, `player-facing-language.md`, and
   `unified-library-and-play-plan.md` for accepted product decisions.
4. Source code and migrations for actual behavior.

Treat `docs/public-roadmap.md`, HTML marketing/docs, screenshots, historical
release notes, and old plan/status prose as historical unless verified against
the above and the code. `docs/releases/next.md` is a draft, not proof that a
feature is finished. Update ADR/status text after the implementation is verified.

## Git and runtime snapshot

- Date/time context: 14-07-2026, Asia/Jerusalem.
- Repository: `C:\src\github.com\GreenFuze\MyGamesAnywhere`
- Branch: `main`
- HEAD: `760a5e1 chore: prepare v0.2.2 release`
- Worktree: roughly 74 modified tracked files plus many untracked implementation
  files; about 3,485 insertions. **Do not reset or clean.**
- `git diff --check` passed at handoff time (only Git LF→CRLF warnings).
- Running server process at handoff: PID 71284,
  `server\bin\mga_server.exe`.
- Running installed client process: PID 18292,
  `%LOCALAPPDATA%\Programs\MGA Client\mga-client-agent.exe`.
- Those running packaged binaries predate the newest launch/staged-progress source
  edits. Rebuild/reinstall before end-to-end testing.
- Local web URL: `http://127.0.0.1:8900`.
- Server was started with `MGA_GOOGLE_DRIVE_DESKTOP_ROOT=G:\My Drive` so archived
  games under `G:\My Drive\Games\Installers` can be resolved.
- Current test game canonical ID:
  `2f983636-0f17-4bc9-8496-7bfec959d68b` (Plasma Pong).
- Current managed install is/was `C:\Games\Plasma Pong`; its pre-handoff manifest
  is schema 1. New installs write manifest schema 2.

## What is already implemented in the dirty worktree

### Web-first client/device architecture

- MGA Client lives under its own `client/` module.
- Per-user pairing, signed WebSocket connection, access grants, endpoint presence,
  inventory, commands, protocol URL launch (`mga://`), download/connect controls,
  top-bar status, stop action, startup registration, windowless agent, tray icon,
  Exit, and Show Logs are implemented.
- Device cards in Settings are collapsed/expandable. `Ping` is a liveness check;
  `Refresh` re-reads metadata/inventory. Old “profile access” wording was cleaned up.
- Device state includes purple `Update required`.

### Profiles/authentication

- Authentication happens at sign-in, not inside Devices.
- Any admin role can manage devices; it is not hard-coded to “Admin Player”.
- Show-password control and credential management/forgot-reset flow exist.
- Current profile is shown once at the top of Profiles; the lower list shows all
  profiles with Edit.

### Library/UI redesign already present

- Library/grouping/player-facing terminology work is in the worktree.
- Grouping supports storefront/integration/platform-style views while preserving
  source copies. A canonical game can appear in every relevant integration group.
- Play surfaces account for installed and browser-play routes rather than treating
  the canonical title as one launchable binary.
- Badges/provenance, compact action menus, status pills, tooltips, grouped views,
  play shelves, and less verbose settings copy were added.
- Game detail includes device availability and archive setup actions.
- Notification center/history and background scan visibility/schedule controls are
  implemented in source.
- “View in library” deep links apply the integration filter.

### Reconciliation/background scanning

- Manual Rescan All and scheduled background scans share the same reconciliation
  pipeline.
- Missing source games can be retired/removed so stale TV2/Xbox/local entries do
  not live forever.
- Scan progress/events and notification history are visible.
- Interval setting exists. Verify behavior, but do not fork a separate scan path.

### Identity/copies/editions

- ADR-0004 data model work for title/edition/source separation is present.
- Multiple regional/platform editions, duplicated storefront copies, DLC/add-ons,
  achievements, browser/local play options, and save-sync implications must stay
  distinct. Do not regress this to title-only deduplication.

### Device inventory and managed ZIP installation (verified before current edits)

- Device storage/runtime inventory and game availability are implemented.
- Managed ZIP flow was successfully tested through the UI before this handoff:
  server resolves the Google Drive archive, issues an origin-relative bearer-token
  transfer, client downloads/verifies/extracts safely, writes
  `.mga-install.json`, atomically commits, server records the install, and UI can
  uninstall it.
- Transfer tokens are sent in `Authorization`, redacted from audit persistence,
  and restricted to the paired MGA Server origin.
- ZIP traversal/symlinks, destination existence, disk space, and manifest/root
  boundaries are checked. Uninstall removes only an MGA-managed folder.
- Existing resolver-match actions include “Use This Match”; Plasma Pong was
  matched through the UI.
- Destination default decision: future per-profile **My Settings** value,
  `%USERPROFILE%\Games` by default with environment-variable expansion. The user's
  machine preference is `C:\Games`. Do not make this a server-global setting.

## Current user request and exact UI requirement

The active implementation request is:

1. Discover/select a safe installed game executable and allow **Play on device**.
2. Installation UI must show two separate progress parts with explicit percentages:
   - **Download** percentage with a blue/cyan bar.
   - **Install** percentage with a different purple bar.
3. Both manual/device behavior and UI must use the same command progress data;
   do not simulate progress solely in React.

## Current mid-work implementation (NOT YET END-TO-END VERIFIED)

The following source changes were made on 14-07-2026:

### Protocol

- Added `CapabilityGameLaunch = "game.launch"`.
- `CommandProgress` now optionally carries `stage` and `stage_percent`, validated
  independently from compatibility field `percent`.
- `ArchiveInstallResult` now carries `launch_target` and `launch_candidates`.
- Added typed `GameLaunchRequest`/`GameLaunchResult` plus safe normalized relative
  `.exe` validation.
- Install manifest schema bumped from 1 to 2. Uninstall intentionally accepts
  schemas 1 and 2 so existing installs remain safe.

### Client

- `CommandProgressReporter` now receives a structured update.
- ZIP download reports stage `download`, 0–100, mapped to overall 0–40.
- archive validation/extraction/finalization report stage `install`, 0–100, mapped
  to overall 40–100.
- `discoverLaunchTargets` recursively finds `.exe` files, removes unsafe helper,
  setup, redist, updater, crash, and uninstall executables, scores title matches
  and shallow paths, and records candidates/automatic choice in the manifest.
- Added OO `GameLauncher` / `WindowsGameLauncher`. It validates the manifest,
  candidate membership, directory boundary, regular `.exe`, starts with the game
  directory as working directory, records PID/time, and releases the process.
- Agent advertises/executes `game.launch`.
- Added client tests for target discovery and separate stages.

### Server/persistence

- Added versioned SQLite migration **16**:
  - `device_commands.progress_stage`
  - `device_commands.progress_stage_percent`
  - `device_game_installations.launch_target`
  - `device_game_installations.launch_candidates_json` default `[]`
- Device command/install models and SQLite persistence/readback were extended.
- Added server-side launch target selection constrained to recorded candidates.
- `game.launch` requires `play` access. Changing the launch target still requires
  `manage` access. Install/uninstall require `manage`.
- Added endpoints:
  - `POST /api/devices/{id}/games/{game_id}/sources/{source_game_id}/launch`
  - `PUT /api/devices/{id}/games/{game_id}/sources/{source_game_id}/launch-target`
- Game detail device DTO now returns `can_play`, launch support/target/candidates.
- Successful launch results are type-checked against their command.
- `server/openapi.yaml` was regenerated after adding the routes.

### Web UI

- Added APIs for launch and launch-target selection.
- Installed device card now offers Play and Uninstall.
- If several valid executables exist, a compact “Starts with” selector is shown.
- Active archive-install commands render blue Download and purple Install bars,
  each with its own percentage. Other commands retain one overall bar.
- UI derives completed earlier stage state from current stage:
  during install, Download remains 100%; success shows both 100%.

## Test status — do not overstate completion

After writing this handoff, the final compact smoke pass succeeded on the current
source (including the access/filter/test edits):

- `protocol`: `go test ./...`
- `client`: `go test ./internal/clientapp`
- `server`: `go test ./internal/devices ./internal/db ./internal/http`
- current frontend `npm run build`
- `git diff --check`

OpenAPI generation and `go test ./internal/openapi` had passed immediately before
the smoke pass and no route change followed. A broader `go test ./...` in the
client/server modules, the listed test gaps, packaging, migration-on-real-DB, and
browser end-to-end verification remain outstanding. Do not call the feature done.

## Immediate continuation checklist

Run from PowerShell; fail fast on the first real error:

1. Inspect the latest diff around protocol install/progress, client agent/installer/
   launcher, server device DB/service/HTTP, `api/client.ts`, and `GameDetailPage.tsx`.
2. Run module tests:

   ```powershell
   Push-Location protocol; go test ./...; Pop-Location
   Push-Location client; go test ./...; Pop-Location
   Push-Location server; go test ./...; Pop-Location
   Push-Location server\frontend; npm run build; Pop-Location
   Push-Location server; go run ./cmd/openapi-gen; go test ./internal/openapi; Pop-Location
   git diff --check
   ```

3. Close likely test gaps before packaging:
   - migration 16 columns and upgrade from migration 15;
   - DB round-trip of stage fields and launch candidates/target;
   - launch-target selection accepts only a recorded candidate;
   - HTTP launch/selection routes and encoded source IDs;
   - fake `devices.Store` implementations compile with
     `UpdateInstallationLaunchTarget`;
   - launcher unit tests should use a fake process starter if refactored; never make
     generic unit tests open a real game.
4. Update ADR-0006/device protocol docs and `docs/releases/next.md` only after the
   tests establish final behavior. State that migration 16 protects existing
   SQLite installs and manifest schema 1 remains uninstallable.
5. Rebuild the actual packaged app and installer using repository scripts, not an
   npm dev server and not a directly-run client binary.
6. Stop only the exact old packaged PIDs/paths, rebuild server, package client,
   reinstall the client with its installer, and restart server with
   `MGA_GOOGLE_DRIVE_DESKTOP_ROOT=G:\My Drive`.
7. Verify SQLite schema migration reaches version 16 before UI testing.
8. Test through the actual browser at port 8900:
   - refresh Plasma Pong game detail;
   - uninstall schema-1 managed installation through the UI;
   - reinstall to `C:\Games` through the UI;
   - observe both progress bars. The archive is small, so poll quickly; also verify
     persisted command/API fields instead of relying on one screenshot;
   - confirm new manifest is schema 2 and selected target is
     `Plasma Pong/Plasma Pong.exe`, never `unins000.exe`;
   - confirm installed state and Play;
   - click Play through UI, verify command success and exact executable PID;
   - terminate only that exact test game process afterward;
   - leave MGA server and installed MGA Client running; leave the game installed
     unless the user says otherwise.
9. Do not commit/push/release/deploy. Report exact evidence and next decision.

## Packaging/runtime cautions

- Run the app, not an `npm` server. Frontend must be embedded in packaged Go server.
- Do not run `mga-client-agent.exe` directly for protocol testing. Install with the
  generated installer so `mga://`, startup, tray, and paths are genuinely tested.
- GUI installer invocation may not set `$LASTEXITCODE`; verify hash/path/process.
- Stop/replace processes by exact path/PID. Never broad-kill unrelated binaries.
- Preserve `MGA_GOOGLE_DRIVE_DESKTOP_ROOT`; otherwise the archive cannot resolve.
- The schema-1 install has no launch candidates and must be reinstalled with the
  new client to populate schema-2 launch metadata.

## Product decisions still pending after this slice

Do not stop before completing current launch/progress verification unless a real
decision is required. The next likely decision is 7z/RAR support: bundled vetted
extractor versus explicit prerequisite. Later work includes EXE/BIN installers,
prerequisites, disk estimates, cancel/retry, logs, repair/update, and save-sync hooks.

Also planned, not part of the immediate slice:

- profile-owned **My Settings** and persisted default install root;
- environment-variable-aware path UI (`%USERPROFILE%\Games` default);
- richer per-device install queue/history and notifications;
- launch arguments/working-directory overrides and emulator selection;
- choosing browser play versus a local emulator for the same source.

Any persisted My Settings implementation requires its own versioned migration.

## Design rationale worth preserving

- `mga://` is only a browser-to-client wake/connect bridge. Durable work is a
  server-authorized command to an authenticated endpoint; never put orders/secrets
  in the URL.
- A green client light means this profile can see a connected endpoint, not merely
  that some process exists on the physical PC.
- Server owns identity, authorization, metadata, command state, and audit history.
  Client owns inventory, filesystem operations, launching, and local progress.
- Keep overall percent for compatibility; stage percent drives the two visual bars.
- Server launch selection is constrained by candidates in the client-owned install
  manifest, and the client revalidates every launch.
- Never execute archive contents during extraction. Launch is a separate explicit
  user action after installation.

## Completion definition

The active task is complete only when all tests/builds pass, migration 16 is applied,
the packaged server and installed client run, installation visibly reports separate
blue Download and purple Install percentages, the correct executable is discovered,
Play launches it through the server/client command path, and evidence is reported
without publishing anything.
