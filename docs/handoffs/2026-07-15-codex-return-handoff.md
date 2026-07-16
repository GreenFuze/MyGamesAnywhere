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
