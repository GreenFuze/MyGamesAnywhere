# MGA Codex return handoff — 15-07-2026

> **AUTHORITATIVE RETURN HANDOFF.** This is the current live-status entry point
> for the Codex agent that originally handed MGA to Cursor. It supersedes the
> 15-07 Cursor handoff for immediate work while preserving that file as detailed
> implementation history. Do not discard the dirty worktree.

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
- Never reset, clean, revert, overwrite, or discard the dirty worktree.
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
- HEAD/remote baseline:
  `ec698e6 feat: add guarded 7z and RAR installation`
- `main` matched `origin/main` before the current dirty implementation.
- Worktree is intentionally dirty with the uncommitted ADR-0007 GOG Inno
  implementation plus architecture revisions and ADR-0008.
- `git diff --check` passes; only LF→CRLF warnings appear.

Important dirty code includes:

- `protocol/device/v1/gog_inno_installation.go` and tests;
- client `gog_inno_installer.go`, Windows/non-Windows adapters, tests, agent and
  service wiring;
- migration 17 and server installation persistence/state;
- GOG server controller/routes/source-plan/multi-file transfer;
- game-detail API/frontend Install/progress/attention behavior;
- OpenAPI/protocol/architecture/handoff updates.

Run `git status --short` for the exact list. No current dirty implementation
file may be removed merely because it is untracked.

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

## Dirty ADR-0007 implementation already present

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

Dirty code currently treats every non-zero exit as
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

Passed on the current dirty worktree:

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
ZIP/7z/RAR baseline but was not rerun for the current dirty GOG code.

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

1. Read ADR-0007 and inspect the full dirty diff.
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

- external deletion/damage reconciliation: shared connection/periodic/manual
  validator, Missing vs Needs repair, versioned migration;
- profile My Settings install-root default `%USERPROFILE%\Games`, endpoint
  override (`C:\Games` desired here), per-install override;
- standalone typed prerequisite model;
- durable reconnect idempotency, cancellation, repair/update, richer history,
  save-sync hooks.

These are not permission to broaden ADR-0007/0008 while completing them.
