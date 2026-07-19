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

ADR-0015 is implemented and verified in feature checkpoint `1fe8c93`. It is
the authoritative continuation of ADR-0013/0014 and shipped in v0.2.3.

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

ADR-0016 is implemented and verified in the same committed ADR-0015/0016
checkpoint `1fe8c93` and shipped in v0.2.3. The accepted design is recorded in
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

## v0.2.3 release and TV2 deployment — 17 July 2026

- Feature checkpoint `1fe8c93` (`feat: add managed emulator routes and setup`)
  and release checkpoint `6ef4b21` (`chore: prepare v0.2.3 release`) are pushed
  to `origin/main`.
- Stable GitHub release `v0.2.3` targets exact commit `6ef4b21` and is the
  latest release: `https://github.com/GreenFuze/MyGamesAnywhere/releases/tag/v0.2.3`.
- Published artifacts are the server installer (623,071,298 bytes), portable
  ZIP (679,173,537 bytes), MGA Client installer (8,241,047 bytes), update
  manifest, and four-entry checksum file. GitHub-reported SHA-256 digests match
  the locally verified files.
- TV2's own Settings > Update interface found `0.2.3`, downloaded and verified
  the 594.2 MB installer, applied it, went offline for the expected restart,
  and returned HTTP 200. The UI reports `MGA restarted on version v0.2.3`, No
  update, and About confirms version `v0.2.3` at commit `6ef4b21`.
- TV2 had no browser warnings or errors after restart. One earlier Chrome
  extension `Receiving end does not exist` message occurred during the planned
  server restart and did not recur after the new server was online.
- The local packaged server was rebuilt and restored after release packaging.
  It runs v0.2.3 at commit `6ef4b21`, HTTP 200, schema 24. The installed v0.2.2
  MGA Client remains protocol-compatible and reconnected Ready in standard
  mode; it was not silently replaced by the server release.

## ADR-0017 route-level Save Domains — 17 July 2026

This is the current authoritative continuation after the v0.2.3 checkpoint.
The complete decision is recorded in `docs/architecture/0017-route-save-domains.md`.

Implemented architecture and behavior:

- save capability is attached to a play route or owned copy, never inferred
  from a canonical title alone;
- the typed read model now distinguishes `mga_managed`, `local_files`,
  `provider_api`, `provider_opaque`, `unsupported`, and `unknown` access, plus
  explicit status, manager, MGA read/write ability, and transfer policy;
- actual launchable browser-play routes expose an MGA-managed backup domain.
  A non-launchable browser placeholder exposes no save capability;
- local source copies, installations, and emulator routes expose local-file
  boundaries and remain `needs_adapter` until MGA has a verified adapter;
- Steam, Xbox, xCloud, and Epic boundaries are provider-managed/opaque. The UI
  uses conditional language and does not claim that a provider cloud save
  exists or that MGA can read or replace it;
- distinct routes remain distinct domains. MGA must not copy between a
  storefront, local native game, browser play, or emulator merely because the
  displayed title matches. A future compatibility or conversion adapter must
  establish that relationship explicitly;
- cards now show compact, accessible save-state badges. Game details include a
  Saves section describing every available route/copy separately;
- Settings > Connections renames the old save migration action to **Move MGA
  Save Backups** and states that it moves only MGA browser-save backups, not
  Steam, Xbox, or other storefront saves;
- existing browser-save snapshot concurrency and storage behavior remain
  unchanged.

Persistence:

`NO_MIGRATION_NEEDED`: ADR-0017 adds a derived server read model, API fields,
and UI presentation only. It does not persist a domain, adapter, mapping,
converter, or user choice and does not change existing SQLite or JSON/config
data. The real schema remains 24. Migrations 22, 23, and 24 are immutable;
migration 25 remains reserved for the first later persisted Save Domain change.

Fresh verification after the final edge-case correction:

```text
protocol: go test ./...                              PASS
client:   go test ./...                              PASS
server:   go test ./...                              PASS
plugins:  go test ./... in all 7 standalone modules PASS
frontend: npm run generate:api-contracts             PASS
frontend: npm run test:unit                          PASS (10 tests)
frontend: npm run build                              PASS
quality:  gofmt + git diff --check                   PASS
security: govulncheck                                NOT RUN (tool unavailable)
```

Packaged real E2E:

1. The production Play page showed 74 currently playable games. Browser-play
   cards displayed `MGA save backup available`, xCloud cards displayed
   `Provider-managed saves`, and local copies without an adapter displayed
   `Local saves need setup`.
2. `Castlevania: Aria of Sorrow` displayed three independent save domains: its
   Google Drive copy requires a verified local adapter, launchable Browser play
   has an MGA backup available, and the RetroArch route requires a verified
   emulator adapter.
3. `A Little to the Left` displayed only the Xbox copy and Xbox Cloud Gaming
   provider-managed domains. Its non-launchable browser placeholder was absent.
   The real API also returned `has_save=false` for that placeholder and
   `provider_opaque` for xCloud and the Xbox source.
4. After rebuilding and restarting the packaged server, the installed MGA
   Client reconnected Ready. The browser console contained no warnings or
   errors. The production page is left open on the Castlevania Saves section
   for review.

Current runtime and preservation state:

- packaged server PID `3384`, exact `server\bin\mga_server.exe`, portable,
  HTTP 200 at `http://127.0.0.1:8900`, SHA-256
  `0737B3EBBBC0021A0AF30F74A2443DC6AE6C4FF0C72ACD3BE3C389BD94578A65`;
- installed standard MGA Client PID `50760`, exact per-user installed agent,
  SHA-256
  `3E245D0E4360AA4E1852E0895A229C3D34A1C02791DAD2C789245901D1F33C27`;
- schema 24, credentials, `C:\Games`, `%USERPROFILE%\Games`, Plasma Pong, and
  the legacy Duke attention row/tree remain preserved. The server was started
  with `MGA_GOOGLE_DRIVE_DESKTOP_ROOT=G:\My Drive`;
- no install, uninstall, cleanup, emulator setup, save copy, or save deletion
  command was run during ADR-0017 verification;
- all ADR-0017 changes remain intentionally uncommitted on `main`. Do not
  reset, clean, revert, overwrite, or discard them.

Next bounded task: define and implement the verified local-save adapter
contract, then add the first concrete emulator adapters (RetroArch and
ScummVM) without weakening route identity. Record the next ADR first. If it
persists adapter discovery, route bindings, compatibility, converters, user
overrides, or sync state, add migration 25 rather than editing migrations
22-24. Provider APIs and cross-route conversion remain later, separate work.

## Profile, Xbox, and client recovery fixes — 18 July 2026

This section supersedes older runtime/worktree status above. `origin/main` is
currently `8399d6b5` and tagged `v0.2.4`; the fixes below remain intentionally
uncommitted. Do not reset, clean, revert, or discard any dirty or untracked
files, including the pre-existing `AGENTS.md`, `CLAUDE.md`, and
`client/internal/launchlog` work.

ADR-0019 records the accepted boundaries:

- the login/profile picker now offers **Add player**; it signs into an existing
  administrator, creates only a player through the normal authorized API, and
  selects the new profile;
- initial password/PIN setup is allowed from trusted-LAN remote browsers. It
  remains one-time and cannot replace an existing credential;
- Xbox tokens are profile-integration-owned. The plugin ignores legacy
  `tokens.json`, clears request state when config has no tokens, reads tokens
  only from the current scan/achievement request, and asks Microsoft to show
  the account chooser;
- draft OAuth results are held server-side by OAuth state and consumed only by
  the same profile/plugin when the integration is created. Tokens are not sent
  through the frontend retry;
- a client launched for a different server displays the paired/requested
  origins and offers a locally confirmed unbind. Other windowless protocol
  errors now display a native error dialog. The tray has **Unbind from server**;
- while disconnected, the top bar offers both launch actions for an existing
  pairing and **Pair this Windows user**, preventing an unbound client from
  becoming trapped in a launch loop;
- Settings tabs wrap into clean button rows and no longer expose the native
  horizontal/vertical scrollbar shown in the reported screenshot.

Persistence is `NO_MIGRATION_NEEDED`: no schema or persisted JSON shape
changed. Existing hashed credentials, encrypted integration config, client
config, and protected endpoint key formats are unchanged. User-confirmed
unbinding deletes only that OS user's existing pairing config and private key.
Schema migration 25 remains applied and immutable.

Fresh verification:

```text
client:       go test ./...                         PASS
server:       go test ./...                         PASS
Xbox plugin:  go test ./...                         PASS
frontend:     npm run test:unit                     PASS (10 tests)
frontend:     npm run build                         PASS
quality:      gofmt + git diff --check              PASS
```

Packaged-local state:

- `server\build.ps1 -WindowsGUI` completed and rebuilt the packaged server,
  frontend, tray, and plugins from the dirty worktree;
- `client\package-installer.ps1` completed and the new per-user installer was
  run silently; the installed agent timestamp is 18 July 2026 08:00:24;
- packaged server PID `55928` runs exact `server\bin\mga_server.exe`, portable,
  with HTTP 200 at `http://127.0.0.1:8900` and
  `MGA_GOOGLE_DRIVE_DESKTOP_ROOT=G:\My Drive`; server SHA-256 is
  `4BB67B19393DD62C500CD13ED72AD35704B5E07EC85DB8AFDEED32F8DB6ED92D`;
- the locally built client installer SHA-256 is
  `6D35F53367F87140BDC4A9D816F4D7B5C5C1E8EC5C7779C7A8303C590D71DFD4`;
- the real database remains schema 25. No profile credential, integration,
  device pairing, installation, cleanup, save, or game data was mutated during
  automated verification;
- Chrome automation could not initialize because its local control kernel
  reported `failed to write kernel assets: The system cannot find the path
  specified`. No browser-driven or native-dialog E2E claim is made. Unit and
  packaged HTTP evidence above is exact; interactive Chrome/TV2 validation is
  still required before release/deployment.

## Actionable attention, local disks, and multi-server client bindings — 18 July 2026

This section supersedes the client configuration/runtime and next-task status
above. ADR-0020 and ADR-0021 are accepted and implemented in the still-dirty,
intentional worktree. No commit, push, release, deployment, profile credential,
integration config, canonical/source game, save, or device grant was created or
changed. Normal client connection updated endpoint last-seen, inventory, and
installation-validation timestamps.

Implemented behavior:

- client config schema 2 stores a bounded `bindings[]` list. Each server owns a
  separate endpoint ID, client instance ID, and protected key. The real schema-1
  localhost config was verified against its DPAPI key and atomically migrated;
  the legacy endpoint/key remain unchanged and the binding has
  `legacy_identity=true`;
- pairing another server appends rather than replaces. Equivalent origins are
  rejected, `mga://start` selects the matching binding, and an unknown origin
  directs the player to **Pair this Windows user** without deleting anything;
- one tray process runs all binding agents. Pairing while the tray is already
  running uses a tested named restart signal so the new process takes over and
  starts the complete binding list immediately. Tray unbind is per-server; CLI
  requires `--server` when several bindings exist and `--all` to remove all;
- Windows storage inventory and install-root enforcement require a fixed drive
  backed by `\\Device\\HarddiskVolume*`. This excludes network, removable,
  SUBST-like, Google Drive Desktop, and other virtual/cloud mounts even when a
  driver reports them as fixed and supplies a volume GUID;
- every unhealthy Devices installation row shows a player-facing cause and
  **Review and resolve**. Duke's legacy record explains the installer failure,
  shows the old folder, explains why MGA cannot safely clean it, and offers
  **Dismiss warning** with confirmation. No cleanup action is exposed because
  the legacy row has no verified cleanup marker.

Cleanup performed at the user's request:

- removed `C:\Games\MGA E2E Cleanup Prompt`;
- removed `C:\Games\MGA E2E Duke Nukem 3D`;
- removed `C:\Games\MGA E2E Manhattan 20260716`;
- verified no `C:\Games\MGA E2E *` directory remains. The server-side Duke
  attention row remains intentionally preserved and can now be dismissed from
  the UI; MGA did not mutate that record during verification.

Fresh automated evidence:

```text
client:       go test ./...                         PASS
server:       go test ./...                         PASS
protocol:     go test ./...                         PASS
plugins:      go test ./... in all 7 modules       PASS
frontend:     npm run test:unit                     PASS (10 tests)
frontend:     npm run build                         PASS
quality:      gofmt + git diff --check              PASS
```

Fresh packaged/UI E2E:

1. `server\build.ps1 -WindowsGUI` and `client\package-installer.ps1` completed.
   The final client installer was installed per-user and its installed agent
   hash exactly matches `client\bin\mga-client-agent.exe`:
   `D7737BFDEB996D6B12606F9751B6536175B4669506625B4A7BD5B36262CD6B19`.
2. The installed client was launched through MGA's **Run MGA Client** action in
   Chrome (not directly). The custom-protocol process connected successfully
   with the preserved endpoint. The in-app browser confirmed **Client ready**.
3. The packaged Devices UI showed only `C:\`, with 251.9 GB free of 951.6 GB.
   G:, H:, and I: disappeared after the new inventory arrived. The exact-host
   Windows tests also reject those present Google Drive Desktop mounts as
   install roots.
4. **Review and resolve** navigated to Duke's exact game/device section. The
   page showed the installer error, the old folder path, the missing-cleanup-
   evidence explanation, and the confirmed **Dismiss warning** action.
5. The real client config is schema 2 with one preserved localhost binding.
   The real database remains schema migration 25.

Current runtime:

- packaged server PID `16288`, exact `server\bin\mga_server.exe`, portable,
  HTTP 200 at `http://127.0.0.1:8900`, SHA-256
  `1F9240E0B8C793751FCFEFBFDEF00FAC256D9102C05C6B0F0E291A395D1B06C0`;
- installed standard MGA Client PID `44860`, exact per-user installed agent,
  connected to preserved endpoint `eaa3b874-bfad-4020-9020-36fd45a04ff9`;
- server started with `MGA_GOOGLE_DRIVE_DESKTOP_ROOT=G:\My Drive`; credentials,
  Plasma Pong, `C:\Games`, profile/default install settings, and schema-25 data
  remain preserved;
- the worktree remains intentionally uncommitted. Do not reset, clean, revert,
  overwrite, or discard tracked or untracked files. Do not commit, push,
  release, or deploy without explicit user instruction.

Next task is product work beyond ADR-0020/0021. The previously recorded next
bounded task (verified local-save adapter contract and first RetroArch/ScummVM
adapters) remains valid unless the user reprioritizes it.

## 2026-07-18 — actionable notification follow-through (ADR-0022)

The user reprioritized notification quality. ADR-0022 is accepted and implemented
in the dirty worktree:

- full and background scans snapshot exact found source records before and after
  successful integration reconciliation. Scan reports and `scan_complete` now
  include a bounded list of added/removed titles with their connection id and
  friendly connection label. Addition/removal totals are exact even when one
  removed game is replaced by one added game in the same scan;
- notification history retains optional structured details, renders scan changes
  in a collapsed list, and keeps the existing v1 profile/browser local-storage
  records readable;
- achievement fetch failures preserve the originating integration identity.
  Their profile-scoped `operation_error` event includes the connection id/label
  and game id/title, and the UI replaces the raw plugin error with player-facing
  sign-in guidance;
- connection notifications carry internal actions. Clicking the notification or
  its action opens Settings > Connections, expands the relevant group, scrolls
  the exact connection into view, and highlights it. Device validation and other
  connection events also receive supported deep links where their payload has an
  exact target;
- an event/report is capped at 100 changed-game summaries and reports the omitted
  count. No configuration, credentials, or tokens enter notification payloads.

Persistence: `NO_MIGRATION_NEEDED`. `ScanReport` and browser history only gain
optional JSON fields. Existing SQLite `report_json` and
`mga.notification-history.v1` entries remain readable. Historical notifications
cannot be retroactively enriched because their old events did not retain game or
connection identity; new events use ADR-0022.

Fresh verification:

```text
server:          go test ./...                         PASS
frontend:        npm run test:unit                     PASS (10 tests)
frontend:        npm run build                         PASS
migration guard: server/scripts/check-migration-guard  PASS
quality:         gofmt + git diff --check              PASS
```

Packaged UI verification used the real app, not the npm server. A direct
`?tab=integrations&integration=<id>` navigation expanded, scrolled, and visibly
highlighted the Xbox connection; the focused card was inside the 720px viewport.
The existing browser notification history remained readable after the optional
shape extension. The user-owned in-app tab was returned to the original Devices
URL after testing.

Current runtime after the rebuild:

- packaged portable server PID `29024`, `server\bin\mga_server.exe`, HTTP 200 at
  `/health`, SHA-256
  `DD0F99CC6DD128D9CCE133234E97364A2689494B837D6650E2DEFCF96AAD89F1`;
- installed MGA Client PID `44860`, still connected and shown as **Client ready**;
- server retained `MGA_GOOGLE_DRIVE_DESKTOP_ROOT=G:\My Drive`, the existing
  database, credentials, profiles, and install settings;
- no commit, push, release, deployment, or database migration was performed.

## 2026-07-18 — client-authoritative cross-server installation ownership

This newest section supersedes all earlier runtime, schema, ownership, and
next-task status in this handoff. ADR-0023 is accepted and its managed-install
ownership slice is implemented in the still-intentional, uncommitted worktree.
Do not reset, clean, revert, overwrite, or discard tracked or untracked files.
Do not commit, push, release, deploy, or update TV2 without explicit user
instruction.

Implemented behavior:

- client config schema 3 gives every server binding a random stable local
  `binding_id`. URLs remain changeable endpoints, not ownership credentials.
  The real schema-2 localhost binding migrated only after its protected key was
  verified. It retains endpoint `eaa3b874-bfad-4020-9020-36fd45a04ff9` and now
  has binding `7912672d-639f-4ca3-ad72-9880d3a25302`;
- the versioned per-OS-user ownership catalog is the authority for MGA-managed
  paths. It uses random local installation IDs, owner bindings, lifecycle and
  history, atomic replacement, a cross-process named lock, and an in-process
  operation coordinator. A crashed `installing` record becomes `interrupted`
  on the next agent start and remains non-adoptable;
- new managed archives and GOG/Inno installs use visible per-binding roots:
  `<base>\MGA\<server-label>-<short-binding-id>\<game>`. Two servers choosing
  `C:\Games` therefore do not share the same managed destination;
- archive manifest schema 3 and GOG manifest schema 4 bind the manifest to the
  client-local installation and owner. Install, launch, cleanup, and uninstall
  fail closed unless active binding, catalog, manifest, path/root boundary, and
  product evidence agree. Ambiguous legacy installs are not guessed when more
  than one server is paired;
- unbinding preserves files and ownership by default. The tray and CLI can
  release one managed game or release all games while unbinding. Release keeps
  files and history, clears mutation authority, and makes the game adoptable;
- another paired server can pick up only a released game. `mga://release` and
  `mga://adopt` show a local MGA Client confirmation containing the title,
  folder, and requesting server. Catalog and manifest ownership are both
  changed or the operation rolls back/fails closed. Pairing with a reused host,
  port, or URL does not inherit ownership automatically;
- inventory schema 4 reports bounded, privacy-safe states per binding:
  managed here/elsewhere, released, installing here/elsewhere, interrupted,
  and legacy unclaimed. Another server receives no owner URL, key, profile, or
  install path. Devices shows **Release** only to the owner and **Pick up** only
  for released records;
- server migration 26 additively persists `managed_installations_json` and is
  applied successfully to the real database. Migration 26 must never be edited;
  the next persisted SQLite change is migration 27.

Bounded limitation recorded in ADR-0023: initial GOG/Inno collision evidence is
conservative installer identity. Full Add/Remove Programs and storefront
product linkage, a general **Use existing installation** grant, device-global
storefront uninstall wording, and the separately confirmed save-sync ownership
transfer remain follow-up work. The current catalog is intentionally absent on
this PC because no new managed installation has run since schema introduction;
tests cover release/adoption without altering Plasma Pong or the preserved Duke
legacy attention row.

Fresh verification from final formatted source:

```text
client:          go test ./...                         PASS
client quality:  go vet ./...                          PASS
server:          go test ./...                         PASS
protocol:        go test ./...                         PASS
frontend:        npm run test:unit                     PASS (10 tests)
frontend:        npm run build                         PASS
migration guard: server/scripts/check-migration-guard  PASS
quality:         gofmt + git diff --check              PASS
```

Packaged local E2E:

- packaged portable server PID `50392`, exact
  `server\bin\mga_server.exe`, HTTP 200 `OK` at `/health`, SHA-256
  `C16482D39EB4C859121710E05B80AE092C2B43E854B5A9F87DF9066FCC3EB5E5`;
- installed standard MGA Client PID `55672`, version 0.2.4, connected to the
  preserved endpoint. Installed agent and final build hashes match exactly:
  `25C55F76AEF9A07AEB3377F00B8B1DACA87EEE81D782BB92A7951EA81C4BA1E8`;
- final client installer SHA-256:
  `53ACDF6593023B18DB100590A60401F5AE5EE9C2F829AAF674E973DF98FB4933`;
- the real packaged Devices page was reloaded in the in-app browser. It showed
  **Client ready**, client 0.2.4, endpoint Ready, only local `C:\` storage, and
  no browser console errors. No synthetic ownership record was inserted merely
  to make the new section appear;
- server startup preserved `MGA_GOOGLE_DRIVE_DESKTOP_ROOT=G:\My Drive`, the
  existing database, credentials, profiles, integrations, `C:\Games`, Plasma
  Pong, and Duke's legacy attention row.

Next bounded task: implement richer OS/storefront product observation and the
**Use existing installation** grant before treating native-product collision
handling as complete. Then design explicit save-sync ownership transfer for
released/adopted games; do not couple it silently to installation ownership.

## 2026-07-18 — v0.2.5 release and TV2 update

This section supersedes the preceding git/release/runtime status.

- committed the ADR-0023 ownership implementation to `main` as `66453bcc`
  (`feat: isolate managed installs across MGA servers`) and pushed it;
- fixed `publish-release.ps1` so release preparation advances both repository
  `VERSION` and `client/VERSION`, preventing server/client version drift;
- `publish-release.ps1 --inc` created and pushed release commit `d24ba866`,
  annotated tag `v0.2.5`, and the latest GitHub release with verified client,
  server-installer, portable-server, update-manifest, and checksum assets;
- all protocol, client, server, standalone-plugin, generated API contract, and
  frontend unit checks in the release workflow passed;
- TV2, signed in as admin profile Orr, detected v0.2.5 from its v0.2.4 Updates
  page. The 594.2 MB installer downloaded and verified at
  `C:\ProgramData\MyGamesAnywhere\updates\mga-v0.2.5-windows-amd64-installer.exe`.
  **Apply** was invoked and TV2 went offline for restart. The in-app browser's
  private-network URL policy blocked the post-restart reload, so do not claim a
  verified TV2 version until a user refresh confirms it;
- this PC's packaged local server and installed MGA Client were restored on
  v0.2.5. Local `/health` is `OK`; the client reports commit `d24ba866`.

Next bounded task remains richer OS/storefront product observation and a
locally confirmed **Use existing installation** grant, followed by explicit
save-sync ownership transfer and reconciliation for released/adopted and
non-local storefront games.

## 2026-07-18 — PWA and optional managed HTTPS direction

ADR-0024 records the accepted direction and priority; no runtime implementation
or persisted configuration changed.

- MGA should become an installable PWA, initially as an online shell with a
  bounded static-asset cache—not as an offline server or MGA Client replacement;
- remote-LAN PWA installation depends on HTTPS, so PWA follows the TLS foundation;
- trusted-LAN HTTP remains supported and is not silently disabled for existing
  installs;
- Let's Encrypt does not require an API key. MGA generates an ACME account key.
  The recommended LAN flow uses a real player-owned hostname, DNS-01, split DNS,
  and a narrowly scoped token from the DNS provider;
- TLS modes will cover trusted-LAN HTTP, player-supplied certificates, managed
  ACME, and an explicitly trusted external reverse proxy;
- HTTP-to-HTTPS activation requires a canonical-origin/client-binding migration
  so existing endpoints and ADR-0023 installation ownership are preserved;
- implementation requires migration 27 or later plus a client-config migration.
  ACME account keys, certificate private keys, and DNS-provider tokens belong in
  the protected keystore.

Priority is unchanged: complete OS/storefront product observation, **Use
existing installation**, and explicit save-sync ownership transfer first. Then
implement canonical-origin migration, TLS/custom-certificate/reverse-proxy
support, ACME DNS-01, and finally the installable PWA shell.

## 2026-07-18 — ADR-0025 initial implementation complete

This section supersedes the preceding next-task and local-runtime status. The
worktree remains intentionally uncommitted; do not discard or overwrite it.

ADR-0025 records and the implementation now enforces a bounded native-product
and launch-only reuse model:

- MGA Client observes Windows Add/Remove Programs only for exact install-path
  associations with MGA-known installations. It reports an opaque stable
  product identity, display name, version, publisher, and capabilities; raw
  registry keys, uninstall commands, unrelated programs, server credentials,
  and owning-server identity do not leave the client;
- the client ownership catalog migrates atomically from schema 1 to schema 2
  and persists native-product observations plus per-binding launch grants;
- device inventory schema 5 reports bounded native products and whether the
  receiving binding has a launch grant;
- **Use existing installation** requires local desktop confirmation and grants
  only that stable client binding permission to launch that client-local
  installation. Every launch rechecks the grant, manifest, path, and launch
  candidate under the local operation coordinator;
- the server stores the relationship as `shared_existing` / `shared_launch`.
  Shared records never expose uninstall, cleanup, repair, or update actions;
  a stale grant is actionable in game details, where the user can confirm the
  existing copy again or install a separate MGA-managed copy;
- the Windows observer supports multiple matching native products so base
  games, DLC, and related packages are not collapsed into one registry entry;
- server migration 27, `shared_existing_installation_authority`, additively
  adds `local_installation_id` and `authority_mode`. It is applied successfully
  to the real portable database and preserved all existing rows as `managed`.
  Migration 26 remains unchanged; migration 27 must now also remain immutable.

Verification from final formatted source:

```text
protocol:        go test ./...                         PASS
protocol quality:go vet ./...                          PASS
client:          go test ./...                         PASS
client quality:  go vet ./...                          PASS
server:          go test ./...                         PASS
server quality:  go vet ./...                          PASS
frontend:        npm run test:unit                     PASS (10 tests)
frontend:        npm run build                         PASS
migration guard: server/scripts/check-migration-guard  PASS
quality:         gofmt + git diff --check              PASS
client package:  package-installer.ps1                 PASS
server package:  build.ps1 -WindowsGUI                 PASS
```

Runtime evidence:

- packaged portable server PID `2988`, exact
  `server\bin\mga_server.exe`, HTTP 200 `OK` at `/health`, SHA-256
  `63853FA5D412783107ADA1671716C3125D7A9908E059AE466EB940297EEF08CE`;
- final local client installer SHA-256:
  `67B4FEA1E601CD6648FDCE0D952A8BE00B94FD5D6064812ABCCB28779CDCDADA`;
- real database reports migration 27 successful and has the new authority/local
  installation columns. The startup backup is
  `server\bin\data\migration_backups\20260718-124757\db.sqlite`;
- the real packaged Devices page was reloaded in the in-app browser after the
  final build. It showed **Client ready**, the preserved Duke attention row and
  Plasma Pong ready row, local `C:\` storage, and no browser console warnings or
  errors;
- the currently installed v0.2.5 tray client is deliberately still the prior
  release binary. Do not run the new dirty binary against its real catalog and
  then return to the older agent: the new binary migrates the client catalog to
  schema 2 and unknown newer schemas correctly fail closed. Install/update the
  server and client coherently before doing the real confirmation/launch E2E.

Next bounded task: design and implement explicit save-sync ownership transfer
and reconciliation for released/adopted games and non-local storefront routes.
Do not infer writable save authority from install ownership or a launch-only
grant. After that, continue the canonical-origin/TLS/PWA sequence recorded by
ADR-0024.

## 2026-07-18 — ADR-0026 save-domain authority decision

ADR-0026 now locks the next implementation boundary. This is documentation
only (`NO_MIGRATION_NEEDED`); runtime code and persisted data are unchanged.

- writable save authority belongs to an exact client-resolved local save
  domain and one stable client binding, never to a title, installation owner,
  launch grant, server URL, or profile;
- save authority uses a separate client catalog and lifecycle. Installation
  release/adoption and save release/transfer remain separate explicit choices;
- local confirmation, per-domain operation leases, final-snapshot attempt, and
  three-way reconciliation precede writer transfer. Conflict never defaults to
  newest-wins or overwrites local files;
- lost/disconnected server authority may be released locally without touching
  files. A replacement writer must reconcile before restore;
- provider-opaque storefronts remain visible but receive no MGA writer. An
  emulator's native cloud sync also makes the domain externally managed until a
  cooperative adapter exists;
- the first full vertical slice is one exact ScummVM target using explicit
  target/save-path evidence. RetroArch follows only after its override hierarchy
  and native cloud-sync state are resolved; ROM basename guessing is forbidden.

Implementation must be vertical rather than enabling a catalog-only or UI-only
partial feature. It requires client save-authority catalog schema 1, inventory
schema 6, server migration 28, typed commands, local confirmation, server/UI
actions, conflict-safe snapshot/restore, packaging, and real E2E. Migration 27
is immutable.

## 2026-07-18 — ADR-0025 and ADR-0026 packaged implementation checkpoint

This section supersedes the preceding next-task, migration, and runtime status.
The complete implementation remains intentionally uncommitted; do not discard,
reset, clean, or partially separate it.

ADR-0025 is implemented together with the ADR-0026 ScummVM save-domain vertical
slice:

- the separate strict client save-authority catalog permits one writer binding
  for an exact local domain, preserves prior/pending writer identity locally,
  and requires explicit release plus reconciliation before another server can
  write;
- inventory schema 6 reports only sanitized authority facts. Local paths,
  filenames, writer IDs, server URLs, and credentials never cross bindings;
- typed claim, release, snapshot, restore, and reconciliation commands are
  wired through client, server, OpenAPI, generated frontend contracts, and game
  details;
- exact ScummVM routes are cached by their content fingerprint. Save setup
  requires ScummVM to identify exactly one engine-qualified game target. Once
  managed, launch uses that full target and an explicit per-domain
  `--savepath`; it no longer uses auto-detection for writable saves;
- launch, backup, restore, release, and reconciliation share the local
  operation coordinator. The launcher keeps the lease until the exact emulator
  process exits;
- snapshots use a deterministic ZIP bounded to 64 MiB and 4096 files. Restore
  rejects links, special files, traversal, manifest/archive mismatches, and
  unexpected local changes. Preserve-both uses staging plus a local backup and
  rollback;
- transfer capability tokens live only ten minutes, are carried in the HTTP
  Authorization header rather than logged URLs, and uploads are single use;
- the server persists authority/sync/manifest state and exposes player actions
  for first backup, retry, restore, release, conflict choice, and writer
  reconciliation. Provider-opaque storefront routes remain provider-managed
  and receive no MGA writer.

Migration 28 (`client_save_domain_authority`) is applied successfully to the
real portable database. Its startup backup is
`server\bin\data\migration_backups\20260718-143210\db.sqlite`. Migrations 27
and 28 are now immutable; the next SQLite migration is 29.

Fresh verification from final formatted source:

```text
protocol:        go test ./...                         PASS
protocol quality:go vet ./...                          PASS
client:          go test ./...                         PASS
client quality:  go vet ./...                          PASS
server:          go test ./...                         PASS
server quality:  go vet ./...                          PASS
frontend:        npm run test:unit                     PASS (10 tests)
frontend:        npm run build                         PASS
migration guard: server/scripts/check-migration-guard  PASS
quality:         gofmt + git diff --check              PASS
client package:  package-installer.ps1                 PASS
server package:  build.ps1 -WindowsGUI                 PASS
```

The frontend retains the known approximately 868 KB production chunk warning.

Packaged local runtime evidence:

- packaged portable server PID `34608`, healthy at
  `http://127.0.0.1:8900/health`, SHA-256
  `B4DFF9F0B78E86D45C7042D0894DF13E2C63A12B0F8FA6605CDC72AACB6233A6`;
- installed elevated client PID `48236`, endpoint
  `eaa3b874-bfad-4020-9020-36fd45a04ff9` Ready, version 0.2.5. Installed and
  packaged agent SHA-256 both equal
  `B15616C1D0262984E0883A1F27281D6F17C4954C8D4445DD9051033890D6870E`;
- local client installer SHA-256
  `FA5BE6CBD50BCDAEEC6E030D58E6CFB6B08F598B7A3A92C73A3C8E1F33AF8676`;
- Chrome invoked the installed elevated client through the real MGA web
  interface. The top bar changed from **Connect client** to **Client elevated**,
  the device changed from Offline to Ready, and Chrome reported no warnings or
  errors;
- Plasma Pong remains installed at `C:\Games\Plasma Pong`. The preserved Duke
  row/tree remains `attention_required` with no cleanup marker.

A destructive or synthetic save-transfer E2E was deliberately not fabricated.
The sole profile currently has no Save Sync integration, so **Back up now** is
correctly unavailable until the player selects or creates one. Automated tests
cover bearer transfer, snapshot/restore, conflict, cross-server release and
reconciliation, rollback, catalog privacy/schema, exact target detection, and
concurrent emulator/save exclusion. A real backup E2E should use a player-
selected Save Sync connection and a disposable ScummVM save, not mutate an
arbitrary existing game merely to produce evidence.

Next decision boundary: ADR-0024 records the accepted TLS/PWA direction, but
managed HTTPS implementation still requires selecting the canonical-origin
migration and certificate mode UX. Do not silently redirect trusted-LAN HTTP or
change existing client bindings while implementing it.

## 2026-07-19 — profile credential and storefront isolation checkpoint

This section supersedes the preceding runtime PID and next-task status. The
entire worktree remains intentionally uncommitted; do not reset, clean, revert,
or discard the ADR-0025/0026 work or this new checkpoint. Migration 28 remains
applied and immutable. This change has `NO_MIGRATION_NEEDED`; the next SQLite
migration remains 29.

ADR-0027 is accepted and implemented for the reported cross-profile account
failures:

- an unconfigured selected profile now offers optional password/PIN setup at
  the sign-in boundary from any trusted-LAN computer, followed by automatic
  sign-in. **Continue Without A Password** remains explicit and supported;
- new OAuth connections always start fresh authorization. Browser-supplied
  provider tokens/identity are ignored, and OAuth callback results remain bound
  to the selected profile/plugin/draft or exact saved integration;
- Google and Microsoft authorization request explicit account selection;
- Google Drive no longer reads, writes, or falls back to process-wide
  `tokens.json`. Check, browse, scan, materialize, source-delete, settings sync,
  and save sync all require the exact saved integration config or matching
  profile-bound OAuth draft;
- draft browsing carries only the opaque OAuth state. Saved browsing names the
  exact integration. Tokens are never returned to frontend JavaScript during
  draft validation or browsing;
- OAuth token/Steam identity fields are server-owned on updates, preventing a
  stale edit form from overwriting a just-completed callback or copying another
  connection's account;
- Steam no longer uses its process config or legacy `tokens.json` identity for
  scans/achievements. The exact integration must provide its API key and Steam
  ID; forced new connection authorization ignores any remembered Steam ID;
- Epic startup no longer opens a server-local login or loads its global token
  file. Epic now exposes a per-connection authorization-code field, exchanges
  the code during validation, persists tokens only into that profile's existing
  connection config, and fails scans with `AUTH_REQUIRED` without those tokens.

Existing connections are preserved. MGA cannot safely infer that an already
stored provider account was intended for a different profile; the player must
use **Re-auth/Reconnect** on any contaminated connection. TV2 was not updated
and no release was created in this checkpoint.

Fresh verification:

```text
server:          go test ./...                         PASS
server HTTP:     go test ./internal/http               PASS
Google Drive:    go test ./... (plugin module)          PASS
Xbox:            go test ./... (plugin module)          PASS
Steam:           go test ./... (plugin module)          PASS
Epic:            go test ./... (plugin module)          PASS
frontend:        npm run build                         PASS
quality:         gofmt + git diff --check              PASS
server package:  build.ps1 -WindowsGUI                 PASS
```

The known approximately 870 KB frontend chunk warning remains.

Packaged runtime evidence:

- packaged portable server PID `37240`, HTTP 200 `OK` at
  `http://127.0.0.1:8900/health`, executable SHA-256
  `EBC487C4CC30C467424E2B2F2B95324EB8D29B075C507C078D509DB163731426`;
- packaged Epic plugin SHA-256
  `EF49E2171D1B001F38D5A57D94B8D3253CD7CD5A9613D1A220A7100E2A49F9B6`;
- real portable DB still reports schema 28. Its existing TCs Google Drive,
  settings-sync, Xbox, and Steam rows contain profile-row tokens/identity;
- the packaged UI was reloaded in the in-app browser. Profiles still shows the
  existing protected TCs profile and show-password controls. Connections opens
  normally, and the real Add Connection wizard now labels Epic as **Setup
  required** and shows the profile connection's **Authorization Code** field
  with its Epic help link;
- no real Microsoft, Google, Steam, or Epic provider login was performed. Full
  TV2/Orr E2E requires the user to choose Orr's actual provider accounts, which
  must not be fabricated by an agent.

Next bounded runtime step: install this build on TV2 (or release it when the
user asks), select Orr, verify remote optional PIN/password setup, then reconnect
Orr's Xbox and Google Drive connections and confirm the provider account chooser
shows Orr's account before scanning. Do not reuse or copy TCs connection config.

## 2026-07-19 — Xbox explicit-account and fail-closed isolation follow-up

This section supersedes the preceding packaged PID/hash and Xbox next-task
status. The worktree remains intentionally uncommitted. Migration 28 is still
the latest applied migration. This follow-up has `NO_MIGRATION_NEEDED`: the safe
`provider_identity` object is optional backward-compatible connection JSON.

The Xbox connection contract is now independently enforced by the server and
made explicit in the wizard:

- the Xbox name step says that a Microsoft account must be chosen and that MGA
  does not use the MGA Client, Windows Xbox app, server computer, or initiating
  browser computer as connection identity;
- the creation action is **Sign in with Microsoft account**, not a generic
  **Add connection** action;
- switching services clears any prior draft OAuth state, popup, response, and
  error so one provider draft cannot leak into another selection;
- new Xbox creation fails closed if the plugin ignores forced interactive
  authorization, and a completed profile-bound draft must contain both an Xbox
  user ID and refreshable Microsoft token before the row can be created;
- Xbox ignores any legacy user tokens in plugin `config.json`. Application ID
  and secret remain MGA app-registration credentials only;
- every Xbox scan and achievement request still resets plugin token state from
  the exact server-selected connection config. Plugin concurrency is one;
- successful authorization stores a safe `provider_identity` containing the
  Microsoft/Xbox subject and optional gamertag beside the server-only tokens;
- integration list, create, update, and duplicate responses redact the `tokens`
  object. The safe identity remains visible so the UI can identify the account.

Verification:

```text
server:          go test ./...                         PASS
server HTTP:     go test ./internal/http               PASS
Xbox module:     go test ./...                         PASS
frontend:        npm run build                         PASS
quality:         gofmt + git diff --check              PASS
server package:  build.ps1 -WindowsGUI                 PASS
```

The packaged portable server is running from the exact bin executable as PID
`5740`, with HTTP 200 `OK` at `http://127.0.0.1:8900/health`. SHA-256 values:

- server: `2E6453D63CB0E2C4E975F922B82F04F618B132EFDC9F5CCB96FFFAE51149E847`;
- Xbox plugin: `D63D61A0E4F04E1C69809F1489A42CAF8BA2BFBAAB2AF512AC726CE6E538E166`.

Real packaged UI evidence: Add connection -> Game Connections -> Xbox now shows
the account-isolation explanation and **Sign in with Microsoft account**. The
button entered the common sign-in waiting screen; the test authorization was
then cancelled and the wizard closed. No provider callback completed and no
test integration was created. The real DB still has exactly one Xbox connection
(TCs); it retains server-side tokens and now also has a safe Microsoft/Xbox
provider identity. No token values or account subject were printed.

TV2 remains unchanged. Its existing Orr/TC connections must not be treated as
evidence for this build until the player reconnects them and explicitly chooses
the intended provider accounts.
