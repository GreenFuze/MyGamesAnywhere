# MGA Codex return handoff — 15-07-2026

> **AUTHORITATIVE RETURN HANDOFF.** This is the current live-status entry point
> for the Codex agent that originally handed MGA to Cursor. It supersedes the
> 15-07 Cursor handoff for immediate work while preserving that file as detailed
> implementation history. Preserve any active worktree changes.

## Read first

1. `AGENTS.md` and `CLAUDE.md`.
2. This handoff.
3. `docs/architecture/agent-responsibility-boundary.md`.
4. `docs/architecture/0007-locally-confirmed-gog-inno-installation.md`
   (title now says Web-authorized; filename is retained for stable links).
5. `docs/architecture/0008-device-selected-installed-games-shelf.md`.
6. `docs/architecture/mga-device-protocol-v1.md`.
7. ADRs 0001–0006 and `unified-library-and-play-plan.md` as needed.
8. `docs/handoffs/2026-07-15-cursor-handoff.md` for full prior evidence/history.

The Codex agent is trusted as architecture-capable and may perform both roles,
but must still record any new decision before implementing it. ADR-0007 and
ADR-0008 are already bounded; do not reopen them without contradictory runtime
evidence.

## Non-negotiable constraints

- Fast-fail; prefer explicit object boundaries and injected platform adapters.
- Use SuitCode when available and rate every feature call; ordinary tooling is
  the fallback if unavailable.
- Every persisted SQLite/JSON/config change requires a versioned migration or
  explicit `NO_MIGRATION_NEEDED`.
- Never reset, clean, revert, overwrite, or discard active worktree changes.
- Do not commit, push, release, deploy to TV2, or rewrite history unless asked.
- Do not change or expose existing credentials.
- Preserve trusted-LAN HTTP, per-user non-admin client identity, and typed
  commands; never add generic shell/exec.
- Runtime E2E uses the packaged server and installed client, not npm dev or a
  directly launched development agent.
- Preserve `MGA_GOOGLE_DRIVE_DESKTOP_ROOT=G:\My Drive`.
- Use `C:\Games` for this endpoint's tests.
- Preserve the Plasma Pong installation.

## Git state

- Repository: `C:\src\github.com\GreenFuze\MyGamesAnywhere`
- Branch: `main`
- Committed implementation foundation:
  `1e59e51 feat: add GOG Inno installer foundation`
- Commit `1e59e51` contains the complete prior dirty GOG implementation,
  ADR-0007/0008 packets, tests, and Codex return handoff.
- The follow-up commit containing the final clean-state wording in this file is
  the handoff-sync commit; verify `git log -2 --oneline` after pull.
- `main` is expected to match `origin/main` with a clean worktree after that
  handoff-sync commit is pushed.
- `git diff --check` passes; only LF→CRLF warnings appear.

Important implementation surfaces include:

- `protocol/device/v1/gog_inno_installation.go` and tests;
- client `gog_inno_installer.go`, Windows/non-Windows adapters, tests, agent and
  service wiring;
- migration 17 and server installation persistence/state;
- GOG server controller/routes/source-plan/multi-file transfer;
- game-detail API/frontend Install/progress/attention behavior;
- OpenAPI/protocol/architecture/handoff updates.

Run `git status --short` first and expect a clean synchronized tree. If any
local changes exist after pull, inspect and preserve them before proceeding.

## Committed delta since Codex's prior handoff

Commit `ec698e6` added and verified:

- bundled pure-Go ZIP/7z/RAR support;
- equivalent traversal/link/size/disk/cancellation/rollback guards;
- licensed fixtures and third-party notices;
- Go 1.26.5 build-toolchain pin;
- real packaged 7z/RAR browser/server/client E2E;
- agent responsibility/escalation policy.

The earlier device inventory, managed archive, separate Download/Install
progress, safe launch-target selection, Play on device, identity/reconciliation,
notifications, and UI work remain committed.

## Committed ADR-0007 implementation foundation

The implementation agent completed most of the original signed-GOG Inno packet:

- typed `game.install_gog_inno` / `game.uninstall_gog_inno`;
- exact server candidate structure: one `setup_*.exe` plus matching
  `setup_*-N.bin`;
- client `WinVerifyTrust` GOG signer validation and bounded Inno marker;
- same-origin multi-file transfer, aggregate progress, hashing, disk checks;
- fixed Inno silent arguments and Windows `runas` fallback;
- native MGA install/uninstall confirmation adapters;
- schema-3 manifest, launch discovery, Inno uninstaller discovery;
- migration 17 installation kind/state/files/uninstaller metadata;
- server routes, persistence, game detail and frontend;
- richer failure/attention copy;
- correct `/DIR="..."` and `/LOG="..."` quoting;
- staging retained after a started installer fails so diagnostics survive.

Migration 17 is already applied to the real database with checksum
`c15af1fda922e7eedbede9fcb802ce19fe7994178d1f04252de8b5cafe249dd6`.
**Never edit migration 17.** Add migration 18 for the locked cleanup revision.

## Runtime evidence that revised architecture addresses

Representative package:

```text
G:\My Drive\Games\Installers\setup_duke_nukem_3d_1.5_(28044).exe
```

It has a valid `GOG Sp. z o.o.` Authenticode signature. With fixed silent flags:

- files and `unins000.exe` are written;
- Inno log records `Installation process succeeded`;
- setup then crashes during deinitialization with
  `0xC000041D` / signed `-1073740771`;
- the failure is installer deinit, not game Play.

The committed foundation currently treats every non-zero exit as
`installer_exit_nonzero` + `attention_required`, so packaged success E2E is
blocked until the revised completion classifier is implemented.

The current Duke database row is:

- game `0c31f912-b8c3-4523-a34b-afd87329d284`
- source `scan:d9d4d0c6cfe89a89`
- path `C:\Games\MGA E2E Duke Nukem 3D\Voxel Duke Nukem 3D`
- kind `gog_inno`
- state `attention_required`
- reason `installer_exit_nonzero`
- no recorded uninstaller or cleanup marker.

Do not synthesize a marker or recursively delete this legacy row/tree. It is
eligible only for user manual cleanup or Ignore after the new policy exists.

## Locked ADR-0007 completion packet

### 1. Recognition policy

Keep the current two layers:

- server: structural candidate filtering only;
- client immediately before execution: valid GOG Authenticode signer plus
  positive Inno marker.

No server signature preflight, manual trust override/tag, filename-only
verification, another signer, or another family.

### 2. Remove MGA Client install popup

Authenticated Manage-authorized confirmation in the web Install dialog is the
install consent boundary. MGA Client must not show a second “Approve install”
MessageBox.

- Remove install confirmer/details/call, Approve-on-device phase, and
  install-only confirmation failures/tests.
- Keep exact GOG/Inno/fixed-argument checks.
- Windows UAC may still appear.
- Native destructive confirmation remains for uninstall and failed cleanup.

### 3. Exact crash-after-success classifier

Treat install exit as success only when:

- unsigned-32 exit equals exactly `0xC000041D`;
- final 1 MiB of bounded Inno log contains case-insensitive
  `Installation process succeeded`;
- it does not contain `Installation process failed`, `Rolling back changes`, or
  `Rollback failed`;
- destination, exactly one regular uninstaller, launch discovery, result and
  schema-3 manifest validation all pass.

Add `completion_basis`:

- `exit_zero`
- `validated_post_success_crash`

Keep raw exit, PID and diagnostic evidence. Any other exit/sentinel/validation
outcome remains failed. This applies only to install, never uninstall/cleanup.
No DB migration is needed for completion basis.

### 4. Failed install cleanup / Retry / Ignore

Before process start create schema-1 `.mga-failed-install.json` in the
previously absent destination with random 256-bit marker ID, command/game/source,
root/path, family, primary hash and timestamp.

Failure classes:

- pre-start: remove staging and marker-only empty destination; no row;
- accepted deinit crash: Installed, replace marker with schema-3 manifest;
- true post-start + valid marker: `cleanup_required`;
- missing/unsafe marker: `attention_required`, no bounded cleanup.

Add typed Manage command:

```text
game.cleanup_gog_inno_failed
```

Add authenticated Manage HTTP actions:

```text
POST /api/devices/{id}/games/{game_id}/sources/{source_game_id}/cleanup-failed
POST /api/devices/{id}/games/{game_id}/sources/{source_game_id}/ignore-failed
POST /api/devices/{id}/games/{game_id}/sources/{source_game_id}/reopen-failed-cleanup
```

Cleanup:

1. Revalidate persisted identity/root/path/marker/family/hash and reject
   replacement/reparse/boundary ambiguity.
2. Show native destructive cleanup confirmation.
3. Run one validated recorded Inno uninstaller first when present.
4. If uninstaller fails/times out, preserve files and set `cleanup_failed`.
5. If no uninstaller exists, or successful uninstall leaves files, a no-follow
   deleter may remove only the exact marked destination.
6. Never directly remove registry/shared prerequisites/outside saves/root.

Ignore:

- state `ignored_failure`;
- files untouched, Play blocked;
- actor/time/event persisted;
- warning suppressed but cleanup remains reversible.

Retry performs cleanup first and reopens Install only after success; it never
automatically executes another installer.

### 5. Migration 18

Do not modify 17. Add `failed_install_cleanup` migration 18:

- installation columns:
  - `cleanup_marker_id`
  - `cleanup_ignored_at`
  - `cleanup_ignored_by_profile_id`
- `device_installation_events` with installation identity, optional actor,
  allow-listed event type, reason, sanitized details JSON and timestamp;
- identity/time index.

Add states:

- `cleanup_required`
- `cleanup_running`
- `cleanup_failed`
- `ignored_failure`

Existing rows get null cleanup metadata. Existing no-marker attention rows are
not eligible for bounded deletion.

Exact contracts, marker schema, events, API DTOs, UX, automated tests, synthetic
cleanup E2E and stop conditions are in ADR-0007. Follow it, not this summary,
when details differ.

## Locked ADR-0008 packet: Installed Games shelf

Implement this after ADR-0007 completion or as a separate bounded task:

- extract existing `mga.clientEndpoint.<profile-id>` association into one shared
  module/hook/event used by top bar and Play;
- valid associated endpoint first; one-device non-persisted fallback; multiple
  devices require explicit selection; never infer/aggregate devices;
- new profile-scoped
  `GET /api/play/devices/{id}/installed-games`;
- Installed Games is first on root Play, then Continue Playing and existing
  shelves;
- include only state `installed`; attention/cleanup/missing states are excluded
  and counted in an attention link;
- canonical dedupe selects target-bearing newest source, then lexical source ID;
- direct Play dispatches existing typed launch to the selected endpoint/source;
- offline/update/view-only/no-target/empty/error behavior is specified;
- attention link uses
  `/settings?tab=devices&device=<endpoint-id>`;
- `NO_MIGRATION_NEEDED`.

Full DTO, sorting, tests, packaged E2E and stop conditions are in ADR-0008.

## Fresh verification at return handoff

Passed on the implementation foundation before commit:

```powershell
Push-Location protocol; go test ./...; Pop-Location
Push-Location client; go test ./...; Pop-Location
Push-Location server; go test ./...; Pop-Location
Push-Location server\frontend; npm run build; Pop-Location
git diff --check
```

Frontend build warning remains:

- production JS approximately 801 KB, above Vite's 500 KB warning threshold.

OpenAPI was regenerated by the implementation agent; regenerate/test it again
after the revised routes/contracts. `govulncheck` passed on the previous
ZIP/7z/RAR baseline but was not rerun for the committed GOG foundation.

## Current runtime

At handoff:

- packaged server PID `60204`
  - `server\bin\mga_server.exe`
  - portable mode, HTTP 200 on `127.0.0.1:8900`
  - preserve `MGA_GOOGLE_DRIVE_DESKTOP_ROOT=G:\My Drive`
- installed client PID `39624`
  - `%LOCALAPPDATA%\Programs\MGA Client\mga-client-agent.exe`
- SQLite schema version 17;
- Plasma Pong remains installed at `C:\Games` with launch target
  `Plasma Pong/Plasma Pong.exe`;
- no Plasma Pong or GOG setup process was running at handoff.

PIDs are snapshots. Verify exact paths before stopping/replacing anything.

## Recommended execution order

1. Read ADR-0007 and inspect the committed implementation foundation.
2. Implement no-install-popup policy.
3. Implement exact completion basis/log parser.
4. Implement marker/failure payload/cleanup/Ignore/reopen and migration 18.
5. Add all ADR-0007 automated tests.
6. Regenerate OpenAPI; run all Go/frontend tests and `govulncheck`.
7. Rebuild packaged server/client and reinstall client.
8. Run signed Duke install → exact launch → publisher uninstall E2E.
9. Run synthetic marker cleanup/Ignore/legacy-no-marker packaged E2E.
10. Preserve existing Duke legacy row/tree unless user directs manual cleanup.
11. Implement ADR-0008 as a separate bounded task, then package/browser-test its
    selected-device shelf and exact Play path.
12. Update this handoff with final evidence.

Do not commit/push unless the user asks.

## Later accepted/planned work

- lower-priority desktop visual polish: MGA-branded local confirmation dialogs,
  a cleaner MGA Client tray icon, and a server tray icon designed explicitly
  for crisp small-size rendering;
- external deletion/damage reconciliation: shared connection/periodic/manual
  validator, Missing vs Needs repair, versioned migration;
- profile My Settings install-root default `%USERPROFILE%\Games`, endpoint
  override (`C:\Games` desired here), per-install override;
- standalone typed prerequisite model;
- durable reconnect idempotency, cancellation, repair/update, richer history,
  save-sync hooks.

These are not permission to broaden ADR-0007/0008 while completing them.

## Codex progress — 16 July 2026

This section supersedes the older runtime/test snapshots above.

Implemented and verified locally:

- ADR-0007 completion classifier, schema-2 sidecar marker, migration 18,
  cleanup/Retry/Ignore/reopen, Add/Remove Programs fail-closed inspection,
  startup recovery for orphaned running commands, schema-3 launch support, and
  player-facing actions are present in the intentional dirty worktree.
- ADR-0009 records explicit standard/elevated browser launch. Migration 19 is
  applied to the real database; the client remains a per-user process rather
  than a service or auto-start task.
- ADR-0010 replaces the unstable custom Win32 TaskDialog/helper path with
  `github.com/ncruces/zenity` v0.10.14. Packaged E2E held the foreground
  confirmation open while the same elevated agent remained connected and
  responsive. There is no second MGA helper process.
- Server startup now converts commands left `running` by a prior server/client
  interruption to failed `command_interrupted`, preventing permanently disabled
  UI actions.

Real packaged E2E evidence:

- Signed GOG **Duke Nukem: Manhattan Project** installed under
  `C:\Games\MGA E2E Manhattan 20260716`, accepted only through the exact
  `validated_post_success_crash` classifier, produced a schema-3 manifest and
  three launch candidates.
- `DukeNukemMP.exe` launched through `game.launch`; the exact resulting game
  process was observed and stopped after the test.
- Typed publisher uninstall command
  `f5be6c08-c174-4def-9460-30cdfe76ab83` succeeded with publisher process PID
  `35048`, exit `0`. MGA performed no direct recursive uninstall deletion; the
  publisher removed the game directory and its Add/Remove Programs association.
- Synthetic schema-1 no-uninstaller fixture under
  `C:\Games\MGA E2E Synthetic Cleanup 20260716` verified Ignore (files
  preserved plus actor/time/event), reopen, foreground local confirmation,
  Add/Remove Programs inspection, and bounded marked-folder deletion. Cleanup
  command `57560103-81f9-4388-aa50-26152c081041` succeeded with
  `publisher_uninstaller_used=false`, `bounded_delete_used=true`, and no
  leftover directory. Its synthetic row, folder, and four audit events were
  removed afterward.
- The legacy Duke row `0c31f912-b8c3-4523-a34b-afd87329d284` and tree remain
  unchanged: `attention_required`, no marker, path
  `C:\Games\MGA E2E Duke Nukem 3D\Voxel Duke Nukem 3D`.

Fresh verification:

```text
protocol: go test ./...                              PASS
client:   go test ./...                              PASS
server:   go test ./...                              PASS
frontend: npm run build                              PASS
govulncheck v1.6.0 protocol/client/server            PASS
```

The frontend retains the known approximately 807 KB production chunk warning.
`govulncheck` initially found reachable issues in server `golang.org/x/net`
v0.50.0; the worktree now uses v0.55.0 and the rescan passes. The associated
`x/crypto`, `x/sys`, and `x/text` transitive upgrades are compilation-only.
`NO_MIGRATION_NEEDED`: dependency and local dialog changes do not alter SQLite,
client config, protocol payloads, installation manifests, or persisted JSON.

Current runtime snapshot:

- packaged server PID `13924`, healthy at `http://127.0.0.1:8900`;
- installed elevated client PID `36036`, connected and responsive;
- real SQLite schema version 19 (migration 17 remains immutable);
- preserve `MGA_GOOGLE_DRIVE_DESKTOP_ROOT=G:\My Drive` and Plasma Pong.

ADR-0007 and ADR-0008 are now complete. This statement supersedes the remaining
work list above.

Final ADR-0007 evidence:

- packaged legacy/no-marker refusal exposed one Ignore action and zero Clean up
  actions for Duke row `0c31f912-b8c3-4523-a34b-afd87329d284`; the row and tree
  remain unchanged;
- interrupted cleanup startup recovery now also repairs the historical state
  where the command is already `command_interrupted` but the installation is
  still `cleanup_running`; it moves the installation to `cleanup_failed`, keeps
  the marker, and emits the audit event idempotently;
- the pre-existing synthetic cleanup row was recovered safely and deliberately
  not bypassed with manual filesystem/database deletion.

Final ADR-0008 evidence:

- root Play showed Installed Games first for endpoint
  `eaa3b874-bfad-4020-9020-36fd45a04ff9`, with Plasma Pong exactly once and two
  attention rows excluded/counted;
- the shelf Play action produced successful command
  `016cb572-d5ec-4e93-a7ef-9692269d6d90`; returned PID `15976` matched the exact
  observed Plasma Pong process, and the schema-2 manifest records
  `Plasma Pong/Plasma Pong.exe`;
- `/settings?tab=devices&device=eaa3b874-bfad-4020-9020-36fd45a04ff9`
  expanded and highlighted the exact device;
- automated tests cover single-device fallback, stale association, explicit
  multi-device selection behavior, canonical source selection, access
  isolation, state filtering, sorting, and all player action states.

Fresh final verification on 2026-07-16:

```text
protocol: go test ./...                              PASS
client:   go test ./...                              PASS
server:   go test ./...                              PASS
plugins:  go test ./... in all 7 standalone modules PASS
frontend: npm run test:unit                          PASS (3 tests)
frontend: npm run build                              PASS
security: govulncheck protocol/client/server         PASS (0 reachable)
quality:  git diff --check                           PASS
```

The frontend production chunk is approximately 814 KB and retains the known
Vite size warning. Standalone plugin modules were tidied to the server's current
`golang.org/x/text` dependency line so packaged compilation is consistent.

`NO_MIGRATION_NEEDED`: ADR-0008, interrupted-command recovery, frontend tests,
and plugin module alignment add no persisted schema or config shape. The real
database remains schema version 19; migration 17 was not edited.

Final runtime snapshot:

- packaged server PID `34624`, HTTP 200 at `http://127.0.0.1:8900/`, launched
  portable with `MGA_GOOGLE_DRIVE_DESKTOP_ROOT=G:\\My Drive`;
- installed elevated client PID `36036`, connected;
- Plasma Pong remains installed at `C:\\Games`;
- the intentional ADR-0007/0008 worktree remains uncommitted and must not be
  reset or cleaned.

Lower-priority visual work is planned, not started: brand local dialogs with
the MGA logo and redesign both client and server tray icons for crisp Windows
notification-area rendering.

## ADR-0007/0008 checkpoint and ADR-0011 progress — 16 July 2026

The complete ADR-0007/0008 work was committed and pushed to `main` as clean
checkpoint `c545108` (`feat: complete install cleanup and installed games
shelf`). Do not amend or rewrite that checkpoint.

ADR-0011 is implemented in the current intentional, uncommitted worktree:

- shared typed `game.validate_installations` schema-1 command and capability;
- one read-only archive/GOG validator for manual and scheduled checks;
- strict Installed, Missing, and Needs repair evidence/reason mapping;
- failed cleanup, ignored, attention, and cleanup-running states excluded;
- migration 20 verification metadata and transition events, preserving all
  existing event/installation rows and leaving migration 17 unchanged;
- transactional, identity-exact result application with idempotent transition
  events and no `updated_at` shelf reordering;
- profile-scoped automatic checks (default 15 minutes, first attempt after one
  minute, selectable 5 minutes through 24 hours, pausable);
- Settings > Devices schedule/status, manual Check installed games, per-game
  health presentation, SSE invalidation, and low-noise notification history;
- OpenAPI routes and generated contract coverage.

Fresh automated evidence:

```text
protocol: go test ./...                              PASS
client:   go test ./...                              PASS
server:   go test ./...                              PASS
plugins:  go test ./... in all 7 standalone modules PASS
frontend: npm run test:unit                          PASS (4 tests)
frontend: npm run build                              PASS
OpenAPI:  generator + frontend contract generator   PASS
security: govulncheck protocol/client/server         PASS (0 reachable)
quality:  git diff --check                           PASS before final docs
```

The known Vite chunk warning remains (approximately 823 KB). The new client
installer was packaged and installed successfully; its installed agent hash
matches `client/bin/mga-client-agent.exe`. The packaged server was rebuilt,
started in portable mode with
`MGA_GOOGLE_DRIVE_DESKTOP_ROOT=G:\\My Drive`, and applied migration 20 to the
real database. Plasma Pong remains Installed; the legacy Duke row remains
`attention_required` with no marker/tree change; the historical marked
cleanup-failed row remains protected.

Current runtime snapshot:

- packaged server PID `500`, HTTP 200 at `http://127.0.0.1:8900/`;
- real SQLite schema version 20;
- installed client PID `38620`, connected Ready in elevated mode;
- Chrome is authenticated at Settings > Devices with the device expanded;
- the Codex built-in browser intentionally blocks non-HTTP custom protocols;
  Chrome successfully launched the same direct `mga://` elevated action and
  produced the expected UAC prompt.

## ADR-0011 packaged E2E complete — 17 July 2026

ADR-0011 is fully verified. The real packaged server and installed elevated
client exercised the shared manual/background validation implementation through
the visible **Check installed games** action:

1. Plasma Pong and the isolated `adr11-e2e-20260717` managed-archive fixture
   both validated Healthy.
2. Moving only `C:\Games\MGA ADR11 E2E 20260717` to an exact restorable sibling
   produced Missing / `install_path_missing` while Plasma Pong stayed Healthy.
3. Settings > Devices showed the synthetic title as Missing with `The game
   folder is no longer on this device.` Chrome showed the matching toast, one
   unread notification, and history entry `Installed games need attention` /
   `1 missing`.
4. Restoring the same directory produced Installed / `healthy`, exactly one
   `installation_restored` event, a Ready UI state, and the history entry
   `Installed games are available again` / `1 restored`.
5. The synthetic installation, its two transition events, source/canonical
   records, staging files, and game directory were removed. The database has
   zero remaining `adr11-e2e-20260717` rows and the filesystem path is absent.

Final preservation evidence:

- schema remains 20;
- Plasma Pong remains Installed and Healthy at `c:\games\Plasma Pong`;
- the legacy Duke row remains `attention_required`, with no cleanup marker or
  tree change;
- the historical `MGA E2E Cleanup Prompt` row remains `cleanup_failed` with
  marker `DvW_ZxiaL-OClbTNQEBvJgkjVLZiMtClHD44uh8V8yc`;
- packaged server and installed elevated client remain running.

Fresh post-E2E verification passed on 17 July 2026:

```text
protocol: go test ./...                              PASS
client:   go test ./...                              PASS
server:   go test ./...                              PASS
plugins:  go test ./... in all 7 standalone modules PASS
frontend: npm run test:unit                          PASS (4 tests)
frontend: npm run build                              PASS
OpenAPI:  generator + frontend contract generator   PASS
security: govulncheck v1.6.0 protocol/client/server  PASS (0 reachable)
quality:  git diff --check                           PASS
```

The known approximately 824 KB Vite chunk warning remains. ADR-0011 and the
loopback/direct-link launch hardening were committed and pushed to `main` as
`c4fa101 feat: reconcile managed installations` on 17 July 2026.

## ADR-0012 install-location preferences — 17 July 2026

ADR-0012 is implemented and verified as the next bounded installation slice.
It is part of the ADR-0012/0013/0014 implementation checkpoint following
`c4fa101`; do not rewrite or discard that checkpoint.

Implemented behavior:

- destination precedence is per-install override, endpoint override, profile
  default, then `%USERPROFILE%\Games`;
- profile defaults live in the existing profile-scoped `profile_settings` key
  `install_root_template`;
- migration 21 creates the cascading `device_install_preferences` table with
  updater identity and timestamp; it is applied to the real database and must
  not be edited—use migration 22 for any later schema change;
- profile preferences are available to the active signed-in profile through
  **My Settings**; endpoint overrides require Owner, while installation still
  requires Manage;
- MGA Server never expands or interprets the endpoint filesystem template.
  Environment expansion remains in MGA Client under the target OS user, so a
  server on another LAN computer (including TV2) behaves identically;
- trusted-LAN HTTP remains supported. Non-loopback server hosts remain exact
  and are never rewritten to localhost.

Packaged browser E2E:

1. The production frontend loaded **My Settings** with the standard
   `%USERPROFILE%\Games` fallback.
2. `%USERPROFILE%\MGA ADR12 Profile E2E` persisted and reloaded, then the reset
   action removed that row and restored the standard fallback.
3. The expanded Owner card for `tc-pc / TC-PC\tcs` first inherited the profile
   value, then saved the intended endpoint override `C:\Games` and displayed
   `This device uses its own folder.` This override is intentionally retained.
4. The top bar showed `Client elevated`; the endpoint remained Ready. Plasma
   Pong and both preserved legacy attention rows were untouched.
5. The in-app browser reported no frontend warnings or errors during the flow.

Current runtime:

- packaged server PID `6084`, exact executable
  `server\bin\mga_server.exe`, portable mode, HTTP 200 at
  `http://127.0.0.1:8900`;
- installed elevated client PID `38620`, connected Ready;
- real SQLite schema version 21;
- `MGA_GOOGLE_DRIVE_DESKTOP_ROOT=G:\My Drive` was preserved for the packaged
  server process;
- profile default is the standard fallback and the local endpoint override is
  `C:\Games`.

Fresh ADR-0012 verification:

```text
protocol: go test ./...                              PASS
client:   go test ./...                              PASS
server:   go test ./...                              PASS
plugins:  go test ./... in all 7 standalone modules PASS
frontend: npm run test:unit                          PASS (4 tests)
frontend: npm run build                              PASS
OpenAPI:  generator/test + frontend contracts       PASS
security: govulncheck v1.6.0 protocol/client/server  PASS (0 reachable)
quality:  git diff --check                           PASS
```

The expected approximately 829 KB Vite chunk warning remains. The next product
slice should not guess standalone prerequisite types or additional executable
installer families; those remain explicit decisions in the unified plan.

## ADR-0013 device-side installation preflight — 17 July 2026

ADR-0013 is implemented and locally verified on top of ADR-0012 in the same
ADR-0012/0013/0014 implementation checkpoint.

Locked behavior:

- MGA Server dispatches typed, read-only `installation.preflight` schema 1 to
  the selected device/OS-user endpoint before installation;
- requests are bound to game ID, source ID, destination template, category,
  bounded package bytes, and typed prerequisite IDs, with no shell, script,
  executable path, arbitrary probe, elevation, or mutation;
- client results distinguish ready, missing, unknown, installer-managed, and
  not-applicable; only definite required missing facts block Install;
- signed/native installers own embedded prerequisites; MGA never enumerates or
  removes them;
- managed archives visibly remain unknown because they can be ready-to-play,
  require undeclared runtimes, or contain another installer;
- Steam and known emulator detection is allow-listed. Xbox is intentionally
  unknown until a reliable client probe exists, never falsely reported missing;
- Settings now has a per-device **Emulators** surface showing detected emulator
  facts. Persisted selection, cores, firmware, managed emulator installation,
  launch, updates, and RetroAchievements capability mapping require the next
  emulator ADR and migration 22 or later;
- `NO_MIGRATION_NEEDED` for ADR-0013. Existing `device_commands` stores the
  bounded audit/result JSON; schema stays 21 and migration 21 must not be edited.

Packaged E2E evidence:

1. Final packaged server PID `3932` runs exact executable
   `server\bin\mga_server.exe` in portable mode with HTTP 200 at the local test
   URL. This is not an architectural localhost assumption; relative API calls,
   exact non-loopback origins, trusted-LAN HTTP/WS, and remote LAN servers such
   as TV2 remain supported.
2. Final installed elevated client PID `51656` runs from the per-user installed
   MGA Client and connects Ready. Chrome launched the elevated `mga://` action;
   the in-app browser remained the verification surface.
3. Pikuniku source `scan:61e0a917ab382de4`, game
   `54c6e130-843b-45db-8c90-145a1af41867`, destination `C:\Games`, and its real
   148,780,824-byte GOG package produced succeeded command
   `9a924542-9208-4a96-9d8a-eb55d3cea434`.
4. The dialog showed enough free space (`142 MB` package, `263.9 GB` then
   available) and `The game installer will handle its own components.` Install
   remained enabled. The installer was not started and no game files changed.
5. Exactly one final command was created after disabling focus/reconnect
   refetch. The two earlier development commands remain valid audit evidence;
   total preflight command count is three.
6. Settings > Emulators displayed the `tc-pc / TC-PC\tcs` endpoint and detected
   ScummVM at `C:\Program Files\ScummVM\scummvm.exe`. Browser warnings/errors:
   none.
7. Schema remains 21. Endpoint override remains `C:\Games`; profile fallback,
   Plasma Pong, the legacy Duke attention row, and the historical cleanup row
   remain untouched.

Fresh verification:

```text
protocol: go test ./...                              PASS
client:   go test ./...                              PASS
server:   go test ./...                              PASS
plugins:  go test ./... in all 7 standalone modules PASS
frontend: npm run test:unit                          PASS (4 tests)
frontend: npm run build                              PASS
OpenAPI:  generator/test + frontend contracts       PASS
security: govulncheck protocol/client/server         PASS (0 reachable)
quality:  git diff --check                           PASS
```

Current next architectural task: define the emulator catalog and per-endpoint
configuration contract, including normalized
`platform -> compatible emulator[]` catalog entries, default and candidate
selection precedence, per-emulator/version/core capabilities, cores/firmware,
RetroAchievements support, save compatibility, launch ownership, managed
installation/update policy, persistence migration 22, and the split between
MGA-wide catalog data and device/OS-user configuration. A default emulator must
never hide other ready compatible emulator routes.

## ADR-0014 multi-route Play control — 17 July 2026

ADR-0014 is implemented and locally verified on top of ADR-0012/0013 in the
same implementation checkpoint.

Locked behavior:

- a game may expose several simultaneous routes: multiple local emulators,
  browser emulation, native/storefront play, and cloud/remote play;
- cards use one contextual default action plus a down-arrow menu containing all
  other explicit routes; Details remains separate;
- card actions use route-specific player wording such as **Play in browser**
  and **Play in xCloud**. The generic visible **Open** action is removed;
- browser and Cloud shelves make their named route primary, while the installed
  shelf preserves the selected device/OS-user action and keeps browser/cloud
  alternatives available;
- the resolver contract accepts multiple emulator/local/storefront actions, so
  later emulator catalog work does not need to flatten them into one route;
- route selection never silently falls back to a different route. Save-domain,
  achievement/RetroAchievements, target, and edition identity remain route
  facts when the full read model supplies them;
- remembered per-game/profile defaults remain deferred. The initial default is
  deterministic and contextual, so `NO_MIGRATION_NEEDED`; schema remains 21.

Verification evidence:

1. The packaged production app, not a Vite server, showed **Play in browser**
   for Altered Beast and **Play in xCloud** for A Game About Digging a Hole,
   each with separate Details and no generic Open action.
2. The current real library has 52 browser-playable and 74 cloud-playable
   records with no overlap. Four focused resolver tests therefore cover the
   unavailable live multi-route combinations: Cloud-shelf defaulting,
   simultaneous browser/cloud alternatives, a disabled installed-device
   primary with playable alternatives, derived-route deduplication, and the
   Details fallback.
3. Frontend unit tests pass (8 total), the production build passes, and browser
   warnings/errors are empty. The expected approximately 838 KB Vite chunk
   warning remains.
4. Packaged server PID `51616` runs exact `server\bin\mga_server.exe` in
   portable mode with HTTP 200 at the local verification URL. Installed elevated
   client PID `51656` remains connected; top bar reports Client elevated.
5. Schema remains 21. `C:\Games`, `%USERPROFILE%\Games`, Plasma Pong, the Duke
   attention row, preflight audit results, credentials, and
   `MGA_GOOGLE_DRIVE_DESKTOP_ROOT=G:\My Drive` remain preserved.

The next architectural task remains the bounded emulator catalog and
per-endpoint configuration ADR. It must define normalized platforms,
`platform -> compatible emulator[]`, default/candidate precedence, multiple
emulator routes, per-emulator/version/core capabilities, cores/firmware,
RetroAchievements and save compatibility, launch ownership, managed
install/update policy, and migration 22 before making Settings > Emulators
mutable.

The subsequent Save Domain work must explicitly fix and extend Save Sync for
non-local storefronts and cloud routes. It must distinguish client-visible
local files, provider API access, and provider-managed opaque saves; cover
Steam/Xbox cloud ownership, xCloud and games installed on other endpoints;
avoid competing automatic writers; and require explicit compatibility or a
typed conversion adapter before moving saves between storefront and emulator
routes.

## ADR-0015 device emulator routes — 17 July 2026

ADR-0015 is implemented and verified in the current intentional, uncommitted
worktree. It is the authoritative continuation of ADR-0013/0014 and must not be
reset, cleaned, or partially discarded.

Locked and implemented behavior:

- the catalog is normalized `platform -> emulator[]`; a selected default only
  chooses the main local-emulator action and never hides compatible alternatives
  or duplicate source copies;
- migration 22 adds owner-audited per-endpoint/platform emulator preferences.
  It is applied to the real database and must never be edited; use migration 23
  for any later persistence change;
- Settings > Emulators shows every platform, all compatible choices, per-device
  discovery/readiness, and an Owner-only Automatic/default selector;
- the read model emits one route per emulator and source copy, and the Library,
  Play shelves, cards, and game detail surface ready emulator actions alongside
  browser, installed, and cloud routes;
- the first launch adapter is typed ScummVM. The server sends no executable
  path, shell, arbitrary arguments, or server-local path. MGA Client resolves
  its allow-listed runtime, accepts content only from the exact paired server
  origin over HTTP or HTTPS, verifies size/SHA-256 into the per-user MGA cache,
  and starts ScummVM with fixed adapter arguments;
- short-lived content grants are tokenized and all tokens are redacted in the
  device command audit row;
- relative Google Drive source records are safely resolved beneath configured
  `MGA_GOOGLE_DRIVE_DESKTOP_ROOT`; artifact paths are relative to the selected
  game root and out-of-root paths are rejected;
- RetroArch remains visible but Needs setup until typed core discovery exists.
  DOSBox, DuckStation, and PCSX2 remain planned catalog candidates rather than
  falsely ready adapters;
- Save Sync ownership/capability facts are cataloged but Save Domain behavior
  for non-local storefronts/cloud routes remains the next separate design slice.

Runtime evidence found and fixed one pre-existing presence defect: a hard MGA
Server stop could leave persisted `ready` state even though the process-local
WebSocket hub was empty. Endpoint list/readiness now treats the live hub as the
presence authority, so the UI correctly offers Connect after a restart and a
command cannot present a stale connected device. `NO_MIGRATION_NEEDED`: this is
derived process-local presence only and changes no stored schema or data.

Packaged real E2E:

1. Chrome launched the installed client through the elevated `mga://` action;
   Windows UAC succeeded and endpoint `eaa3b874-bfad-4020-9020-36fd45a04ff9`
   connected Ready/Elevated with `game.launch_emulator` advertised.
2. Settings > Emulators showed ScummVM `Ready · Main`, while MS-DOS and
   PlayStation each retained two visible compatible choices. Selecting ScummVM
   persisted one preference row; returning to Automatic deleted it. The final
   preference row count is zero.
3. Castle of Dr. Brain source `scan:75357152afbfe583`, canonical game
   `aa24f7a9-18ee-4a8f-ba06-e30a9d50b782`, exposed a real ScummVM button.
4. Clicking that button created succeeded command
   `6b43469f-f011-451f-87ad-94845b1066e1`, transferred and verified 39 files /
   3,809,076 bytes from `G:\My Drive`, and launched the installed ScummVM
   process. The command audit contains `[redacted]` for every transfer token.
   The temporary ScummVM process was stopped after verification; the immutable
   MGA client cache remains available for later launch reuse.
5. The packaged frontend and server were used throughout; no npm/Vite dev server
   was run.

Fresh verification:

```text
protocol: go test ./...                              PASS
client:   go test ./...                              PASS
server:   go test ./...                              PASS
plugins:  go test ./... in all 7 standalone modules PASS
frontend: npm run test:unit                          PASS (8 tests)
frontend: npm run build / packaged server build      PASS
OpenAPI:  generator + frontend contract generator   PASS
quality:  git diff --check                           PASS
```

Current runtime and preservation state:

- packaged server PID `37136`, exact `server\bin\mga_server.exe`, portable,
  HTTP 200 at the local verification URL, SHA-256
  `44B9071387CA6C46F7DFE3B917DCE0C78A6D91C7219009DF6A177AFA5ABCE80C`;
- installed elevated MGA Client PID `36712`, connected, installed agent
  SHA-256 `BD78B90F98F7CBCB443EB5DDC7759230BFB788AEAEE7354C3CBD3FEEF5B70914`;
- real SQLite schema version 22; migration 22 is immutable and emulator
  preference row count is zero;
- endpoint install override remains `C:\Games`; profile fallback remains
  `%USERPROFILE%\Games`; the server keeps
  `MGA_GOOGLE_DRIVE_DESKTOP_ROOT=G:\My Drive`;
- Plasma Pong remains Installed at `c:\games\Plasma Pong`; the legacy Duke row
  remains `attention_required` with no cleanup marker; the historical cleanup
  row remains `cleanup_failed` with its original marker.

Next bounded work: typed emulator setup management—runtime/core/firmware
discovery, version/capability reporting (including RetroAchievements), and
managed install/update policy—followed by the Save Domain work for local,
provider-API, and provider-opaque saves. Do not collapse platform mappings to a
single emulator, and do not start automatic save copying between storefront and
emulator routes without an explicit compatibility/conversion contract.

## ADR-0016 emulator setup and components — 17 July 2026

ADR-0016 is implemented and verified in the same intentional, uncommitted
ADR-0015/0016 checkpoint. Do not reset, clean, or partially discard this
worktree. The accepted design is recorded in
`docs/architecture/0016-emulator-setup-and-components.md`.

Locked and implemented behavior:

- device inventory schema 2 reports bounded, allow-listed package-manager,
  emulator-runtime, core, and firmware-component facts. It never reports
  executable paths, firmware paths, hashes, arbitrary directory contents, or
  server-supplied probe commands;
- Windows discovery currently identifies `winget`, supported emulator
  runtimes, RetroArch `*_libretro` cores, and exact allow-listed PlayStation and
  Sega CD BIOS sets. Probe completeness is explicit, so MGA distinguishes
  missing from unknown instead of making false claims;
- migration 23 adds owner-audited per-endpoint/platform/emulator RetroArch core
  preferences. Migration 24 corrects the real split-column inventory store by
  adding `device_inventories.package_managers_json`. Both migrations are
  applied to the real database and immutable; use migration 25 for any later
  persisted change;
- the normalized catalog remains `platform -> emulator[]`. RetroArch platforms
  expose all compatible core alternatives, an optional default, per-core
  RetroAchievements support, and typed firmware readiness. A missing default
  or runtime never hides alternatives;
- MGA Server dispatches only typed `emulator.setup` install/update requests.
  MGA Client owns the fixed `winget` package IDs and arguments for RetroArch,
  ScummVM, PCSX2, DuckStation, and DOSBox. The server cannot provide a URL,
  executable, package ID, shell, script, or arbitrary argument, and setup does
  not include uninstall;
- Settings > Emulators now supports per-device default-emulator and core
  selection, detected/missing cores, RetroAchievements and firmware facts,
  Owner-only install/update actions, confirmation, two-phase command progress,
  and post-success inventory refresh;
- RetroArch launch is typed end to end. The client re-resolves the allow-listed
  runtime and selected core, verifies content remains inside the paired-server
  cache, and launches with fixed arguments. Server artifact selection is
  deterministic and rejects ambiguous content;
- remote trusted-LAN MGA servers remain supported over HTTP or HTTPS. No
  localhost, shared-filesystem, or TLS-only assumption was introduced.

Persistence correction discovered during packaged E2E:

The initial inventory-schema-2 design assumed the existing inventory payload
was stored as one opaque JSON value. Runtime evidence showed that
`device_inventories` stores storage and runtimes in separate columns, which
dropped the new package-manager facts. Migration 23 was already applied and
therefore remained untouched. Migration 24 adds the missing dedicated column,
and the repository now round-trips `winget` correctly. This correction and its
test are part of ADR-0016.

Packaged real E2E:

1. Chrome opened the production Settings > Emulators page and launched the
   installed MGA Client through the standard `mga://` action. Endpoint
   `eaa3b874-bfad-4020-9020-36fd45a04ff9` connected Ready in standard mode and
   advertised `emulator.setup`, `game.launch_emulator`, and
   `inventory.refresh`.
2. Inventory schema 2 persisted
   `[{"id":"winget","name":"Windows Package Manager"}]`. The device panel
   changed from Not connected to Connected and exposed the expected install and
   update actions.
3. RetroArch is not installed on this endpoint, but each platform still showed
   all compatible core choices as not installed. ScummVM `2026.1.0` remained
   detected and displayed Update. No setup button was activated, so no external
   emulator package or runtime was installed, updated, or removed.
4. The browser console contained no warnings or errors. The production page is
   left open at `http://localhost:8900/settings?tab=emulators` and the installed
   client remains connected for further local testing.

Fresh verification:

```text
protocol: go test ./...                              PASS
client:   go test ./...                              PASS
server:   go test ./...                              PASS
plugins:  go test ./... in all 7 standalone modules PASS
frontend: npm run test:unit                          PASS (8 tests)
frontend: npm run generate:api-contracts             PASS
frontend: npm run build                              PASS
OpenAPI:  generator/test                             PASS
quality:  gofmt + git diff --check                   PASS
security: govulncheck                                NOT RUN (tool unavailable)
```

Current runtime and preservation state:

- packaged server PID `45864`, exact `server\bin\mga_server.exe`, portable,
  HTTP 200 at the local verification URL, SHA-256
  `E20A3BA480BABEC60EE9BE20B6EBDF1FD93CF07554BE4F5238FADAF62FC5EB6C`;
- installed standard MGA Client PID `50760`, exact per-user installed agent,
  SHA-256
  `3E245D0E4360AA4E1852E0895A229C3D34A1C02791DAD2C789245901D1F33C27`;
- real SQLite schema version 24; migrations 22, 23, and 24 are immutable;
  emulator and core preference row counts are both zero;
- endpoint install override remains `C:\Games`; the profile default remains
  `%USERPROFILE%\Games`; the server was started with
  `MGA_GOOGLE_DRIVE_DESKTOP_ROOT=G:\My Drive`;
- Plasma Pong remains Installed. The legacy Duke row remains
  `attention_required` with reason `installer_exit_nonzero` and no cleanup
  marker. No game installation, cleanup, or emulator setup command was run.

Next bounded architectural task: Save Domains and non-local storefront Save
Sync. Define local-file, provider-API, and provider-opaque ownership; Steam and
Xbox cloud behavior; xCloud and games installed on other endpoints; route-level
save and achievement capabilities; conflict/authority rules; and explicit
compatibility or conversion adapters before copying between storefront and
emulator routes. Record the ADR before implementation and add migration 25 if
the resulting design persists new state.
