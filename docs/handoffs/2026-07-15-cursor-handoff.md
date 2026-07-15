# MGA implementation handoff — 15-07-2026

> **SUPERSEDED FOR IMMEDIATE WORK:** Continue from
> [`2026-07-15-codex-return-handoff.md`](2026-07-15-codex-return-handoff.md).
> This file remains the detailed Cursor-session implementation history.

> **HISTORICAL CURSOR WORKING HANDOFF (15-07-2026).** Read this file before
> changing the current worktree. It supersedes the 14-07-2026 handoff for live
> implementation, test, packaging, runtime, and next-task status. Accepted ADRs
> remain authoritative for design. Do not discard active worktree changes.

## Required reading and precedence

1. Repository `AGENTS.md` and `CLAUDE.md`.
2. This handoff.
3. `docs/architecture/agent-responsibility-boundary.md`.
4. `docs/architecture/0001-mga-client-architecture.md`.
5. `docs/architecture/mga-device-protocol-v1.md`.
6. ADRs `0002`–`0008`, `player-facing-language.md`, and
   `unified-library-and-play-plan.md`.
7. Source code and migrations for actual behavior.

Older roadmaps, screenshots, release prose, and unchecked task lists are
historical where they conflict with this handoff. `docs/releases/next.md` is a
draft summary, not proof of completion.

## Non-negotiable constraints

- Fast-fail and prefer object-oriented boundaries where practical.
- Use SuitCode when available; provide feedback after every SuitCode feature
  call. A non-Cursor agent may fall back to normal repository tooling.
- Every persisted SQLite/JSON/config shape change requires a versioned migration
  or an explicit `NO_MIGRATION_NEEDED` compatibility note.
- Do not reset, clean, revert, overwrite, or discard active worktree changes.
- Do not commit, push, release, deploy to TV2, or rewrite Git history unless the
  user explicitly asks.
- Never overwrite an existing profile credential. Credentials are supplied
  out-of-band and must not be committed.
- Preserve trusted-LAN HTTP support and per-OS-user, non-admin client identity.
- Run the packaged server and installed MGA Client for end-to-end work; never
  substitute an npm dev server or directly launched development agent.
- Preserve `MGA_GOOGLE_DRIVE_DESKTOP_ROOT=G:\My Drive`.
- Use `C:\Games` for this endpoint's installation tests.

## Git snapshot

- Repository: `C:\src\github.com\GreenFuze\MyGamesAnywhere`
- Branch: `main`
- HEAD: `ec698e6 feat: add guarded 7z and RAR installation`
- Remote: `main` is synchronized with `origin/main` at that commit.
- Committed baseline:
  - `0e2d8ba feat: add device-aware library and managed play`
  - `b9c0876 docs: keep credentials out of handoff`
  - `ec698e6 feat: add guarded 7z and RAR installation`
- The ZIP/7z/RAR implementation is committed and pushed.
- Current worktree is intentionally dirty with the largely implemented
  ADR-0007 signed-GOG Inno vertical slice: protocol, migration 17, server,
  client, frontend, tests, OpenAPI/docs, quoting/diagnostics, and failure UX.
- Architecture revisions for exact crash-after-success and marked failed-install
  cleanup/Ignore are now locked in ADR-0007 but not implemented yet.
- Web-authorized install with no MGA Client install popup is now locked in
  ADR-0007; the dirty implementation still has the popup and must remove it.
- The device-selected first Installed Games Play shelf is locked separately in
  ADR-0008 and is not implemented yet.
- Run `git status --short` for the exact implementation/documentation list.
- `git diff --check` passes; only Windows LF→CRLF warnings are emitted.

## Committed baseline now verified

The committed baseline includes:

- web-first library/play UI and player-facing terminology;
- conservative Title/Edition/Library Item identity;
- authoritative manual/background source reconciliation;
- notification history and library deep links;
- per-user paired MGA Client, inventory, capabilities, and device availability;
- managed archive download, transactional extraction, manifest-guarded
  uninstall, staged Download/Install percentages, launch-target discovery and
  selection, and authenticated Play on device;
- server migrations through version 16;
- manifest schema 2 with launch metadata while schema 1 remains uninstallable.

The launch/progress slice was packaged and tested with Plasma Pong through the
real browser/server/client path. The selected target is
`Plasma Pong/Plasma Pong.exe`, not `unins000.exe`.

## Current committed slice: bundled 7z and RAR support

This slice is implemented, documented, packaged, end-to-end verified, committed,
and pushed in `ec698e6`.

### Protocol and server

- `ArchiveInstallRequest.archive_format` accepts `zip`, `7z`, and `rar`.
- The existing `game.install_archive` schema and route remain compatible.
- Server source selection recognizes exactly one supported ZIP/7z/RAR archive.
- Transfer content type is derived safely with an octet-stream fallback.
- UI discovery recognizes `.zip`, `.7z`, and `.rar`.

### MGA Client

- `ManagedArchiveInstaller` replaces the ZIP-specific implementation boundary.
- Pinned pure-Go readers are bundled:
  - `github.com/bodgit/sevenzip v1.6.5`
  - `github.com/nwaples/rardecode/v2 v2.2.5`
- No machine-wide 7-Zip/WinRAR prerequisite is required.
- ZIP/7z/RAR share origin-bound bearer download, hashing, staging, disk checks,
  separate progress stages, launch discovery, manifest writing, atomic commit,
  and rollback.
- 7z/RAR reject traversal, symbolic links, unsupported non-regular entries,
  excessive entry counts, unknown/overflowing sizes, encrypted archives, and
  multi-volume archives.
- Installer packaging includes `THIRD_PARTY_NOTICES.md`.
- Licensed fixtures and notices live under
  `client/internal/clientapp/testdata/`.

### Toolchain/security

- The main protocol/client/server modules retain the Go 1.25 language baseline
  and pin build toolchain `go1.26.5`.
- The first vulnerability scan under Go 1.26.3 found standard-library issues.
  After pinning Go 1.26.5, `govulncheck ./...` for the client reported:
  `No vulnerabilities found.`
- Packaged client and server binaries report `go1.26.5`.

### Migration compatibility

`NO_MIGRATION_NEEDED` for 7z/RAR support:

- command schema 1 already carries `archive_format`;
- manifest schema remains 2;
- server SQLite remains migration 16;
- existing ZIP installations, schema-1 uninstall, schema-2 launch metadata,
  pairing identity, and client config remain valid.

## Verification completed for the 7z/RAR slice

All passed after pinning Go 1.26.5:

```powershell
Push-Location protocol; go test ./...; Pop-Location
Push-Location client; go test ./...; Pop-Location
Push-Location server; go test ./...; Pop-Location
Push-Location server\frontend; npm run build; Pop-Location
Push-Location client
go run golang.org/x/vuln/cmd/govulncheck@latest ./...
Pop-Location
git diff --check
```

OpenAPI generation also passed:

```powershell
Push-Location server
go run ./cmd/openapi-gen
go test ./internal/openapi
Pop-Location
```

Known non-blocking frontend packaging warnings remain:

- six npm audit findings: two low, three moderate, one high;
- the primary production JS chunk is approximately 795 KB and exceeds Vite's
  500 KB warning threshold.

## Packaging and real end-to-end evidence

The server portable package and MGA Client installer were rebuilt with
`GOTOOLCHAIN=go1.26.5`. The client was reinstalled through Inno Setup; protocol
registration, startup entry, installed path, notices, and paired identity were
preserved.

Artifacts:

- Client installer:
  `client\release\mga-client-windows-amd64-installer.exe`
  - SHA-256:
    `EF2F2A03EB92FE8F71E5236A9FF9E589127245EFCAFC45EAC927D7183C5CDB91`
- Server portable ZIP:
  `server\release\mga-v0.2.2-windows-amd64-portable.zip`
  - SHA-256:
    `936E7B5AEDC3E1CBD361224F6755CFB146650FB0EE5CA49A38FDE0B00319F833`

Real browser tests used temporary database source records pointing at the
licensed fixtures and the installed client:

- 7z install command:
  `1e86006f-e497-4baa-9605-825e15eb32e3` — succeeded.
- RAR install command:
  `5f594a94-c1c6-4d0d-850f-e95d65164ef6` — succeeded.
- Both persisted `archive_format`, reached Install 100%, wrote schema-2
  manifests under `C:\Games`, and uninstalled through the UI.
- Temporary E2E games, source rows, commands, installations, and folders were
  removed afterward.
- Plasma Pong remains installed and its database installation row is preserved.

## Current runtime state

At handoff time:

- Packaged server:
  - PID `60204`
  - `C:\src\github.com\GreenFuze\MyGamesAnywhere\server\bin\mga_server.exe`
  - portable mode, port `8900`
  - started with `MGA_GOOGLE_DRIVE_DESKTOP_ROOT=G:\My Drive`
- Installed MGA Client:
  - PID `39624`
  - `%LOCALAPPDATA%\Programs\MGA Client\mga-client-agent.exe`
- HTTP health: `http://127.0.0.1:8900` returned 200.
- SQLite schema version: 17 (`executable_installation_state` is already applied;
  do not edit migration 17).
- Plasma Pong installation:
  - root `C:\Games`
  - path `C:\Games\Plasma Pong`
  - target `Plasma Pong/Plasma Pong.exe`
- No test game process is running.
- A Duke GOG row remains `attention_required` from the reproduced
  crash-after-success test. It has no cleanup marker and is not eligible for
  invented bounded deletion; preserve it until the user chooses manual cleanup
  or Ignore under the new policy.

PIDs are only a snapshot. Verify exact executable paths before stopping or
replacing a process.

## Newly accepted planned requirement: detect external removal/damage

The user explicitly requested this be planned, not implemented now:

> If a user manually deletes or uninstalls a managed game outside MGA, MGA must
> detect that it is no longer installed.

The accepted direction is:

1. MGA Client remains filesystem authority.
2. One shared validator runs on authenticated connection, periodically, and on
   manual refresh; do not create separate reconciliation paths.
3. A missing managed directory or manifest becomes **Missing**.
4. A present directory with a mismatched manifest, missing selected executable,
   or missing managed files becomes **Needs repair**.
5. Play is disabled for Missing/Needs repair.
6. UI offers Reinstall, Repair, or Forget with player-facing explanations.
7. Server preserves installation/audit history and records verification time and
   reason; it must not silently erase the row as though MGA performed uninstall.
8. Detection never removes unrelated files or saves.

This requires a typed protocol report/command and a versioned migration adding
installation state, `last_verified_at`, reason/detection timestamps, and any
repair metadata. Exact naming and report cadence should be decided in that
future vertical slice.

## Planned install-root preferences

Persisted defaults are still not implemented:

- future profile-owned **My Settings** path template;
- default `%USERPROFILE%\Games`;
- optional endpoint override;
- per-install override remains highest priority;
- this computer's desired endpoint/default value is `C:\Games`.

That work requires its own versioned persisted-setting migration. The current UI
correctly falls back to `%USERPROFILE%\Games` and allows `C:\Games` per install.

## Next bounded implementation packet: ADR-0007

The architecture decision is locked in
`docs/architecture/0007-locally-confirmed-gog-inno-installation.md`.

Implementation is underway in the dirty worktree (not committed):

- protocol `game.install_gog_inno` / `game.uninstall_gog_inno` contracts;
- migration 17 and installation read-model fields;
- server source selection, multi-file transfer grants, routes, and game-detail
  availability;
- client GOG Inno installer with injected Authenticode/Inno/confirm/process
  adapters and fake-only automated tests;
- frontend Install / Approve-on-device / progress / attention UX (the
  Approve-on-device portion is now superseded and must be removed);
- OpenAPI regenerated.

Still required before calling the slice complete: implement the locked
no-install-popup consent policy, completion-basis classifier, cleanup
marker/command, Ignore/reopen state, migration 18/events, UI/tests; then package,
reinstall the client, and run real signed-GOG E2E, stopping only for Windows UAC
and destructive uninstall/cleanup prompts. Preserve Plasma Pong and Drive
Desktop config. ADR-0008 Installed Games shelf implementation follows as a
separate bounded task.

### Duke Nukem 3D E2E evidence and resolved completion policy

Package: `G:\My Drive\Games\Installers\setup_duke_nukem_3d_1.5_(28044).exe`

Reproduced outside MGA with the ADR-fixed silent flags. Installer log reaches
`Installation process succeeded`, writes `unins000.exe` + GOG registry, then
crashes during deinit:

- exit `-1073740771` / `0xC000041D` (`STATUS_FATAL_USER_CALLBACK_EXCEPTION`)
- log: suppressed message box for `Access violation ... in module 'setup_*.tmp'`
- same outcome with and without spaces in `/DIR`

**Implementation already fixed (still dirty):** Inno `/DIR="..."` and
`/LOG="..."` quoting so destinations with spaces are not truncated (PowerShell
repro without quotes installed into `C:\Games\MGA`); staging is retained after
a started installer fails so `diagnostic_ref` log survives.

ADR-0007 now resolves the blocker narrowly: this install is successful only when
the unsigned-32 exit is exactly `0xC000041D`, the bounded log contains
`Installation process succeeded` with no later failure sentinel, and normal
destination/single-uninstaller/launch/result validation passes. Result and
schema-3 manifest use `completion_basis=validated_post_success_crash` while
retaining the raw exit/PID/diagnostic evidence. Every other non-zero result
remains failed.

Standalone prerequisites, another publisher/family, unsigned setup, arbitrary
arguments, interactive wizard, generic helper/service, cancellation after
process start, external-removal reconciliation, and install-root preferences
remain out of scope. Any need for them is a mandatory architecture escalation.

## Glossary for agents

- **ADR** = **Architecture Decision Record** — a numbered design decision under
  `docs/architecture/` (for example ADR-0007 =
  `docs/architecture/0007-locally-confirmed-gog-inno-installation.md`). It is
  the locked contract for implementation until an architecture-capable agent
  revises it.

## Architecture backlog status (15-07-2026)

### Resolved and returned to the implementation agent

0. **GOG Inno crash-after-success**
   - ADR-0007 now accepts only exact exit `0xC000041D` when the bounded Inno log
     contains `Installation process succeeded`, no later failure sentinel
     exists, and destination/uninstaller/launch/result validation all pass.
   - Result/manifest record `completion_basis=validated_post_success_crash`;
     raw non-zero exit/PID/diagnostic evidence remains.
   - Any other exit/sentinel/validation outcome stays failed.

1. **GOG recognition authority**
   - Server filtering remains structural candidate selection only:
     `setup_*.exe` + matching BINs, base-game, unambiguous EXE set.
   - Client immediately before execution remains authoritative:
     `WinVerifyTrust`, exact `GOG Sp. z o.o.` signer identity, bounded Inno
     marker.
   - No server preflight, manual trust override, persisted source tag, or
     filename-only “verified” claim is added.

4. **Failed-install cleanup / Ignore**
   - True post-start failures write a random schema-1 cleanup marker before
     execution and persist `cleanup_required`; failures without a valid marker
     remain `attention_required`.
   - User-selected typed `game.cleanup_gog_inno_failed` runs a validated
     publisher uninstaller first. Uninstaller failure preserves files. With no
     uninstaller, or after successful uninstall leaves files, a no-follow
     deleter may remove only the exact marked destination.
   - **Ignore** is a server-side Manage action: state `ignored_failure`, files
     untouched, Play blocked, actor/time/event retained, cleanup reversible.
   - **Retry** performs cleanup first and only reopens Install after cleanup
     succeeds; it does not automatically execute another installer.
   - Migration 17 is already applied and immutable. Add migration 18 for cleanup
     marker/Ignore metadata and `device_installation_events`.
   - Existing Duke attention state has no marker and is not eligible for
     synthesized/automatic folder deletion.

All exact contracts, states, migration 18, UX, tests, packaged E2E, and
stop/escalation conditions are recorded in ADR-0007.

### Additional decisions now locked

2. **No MGA Client install confirmation popup**
   - ADR-0007 now treats the authenticated Manage-authorized web Install dialog
     as consent. The client performs GOG signature/Inno validation and fixed
     execution without a second MGA popup.
   - Windows UAC may still appear. Native destructive confirmation remains for
     uninstall/failed cleanup.
   - Remove install confirmer code, `Approve on device`, and install-only local
     confirmation failures/tests. Do not weaken signature/family/argument
     constraints.

3. **Play page Installed Games shelf**
   - ADR-0008 is the bounded packet.
   - Existing profile-scoped browser endpoint association is authoritative;
     one-device fallback is allowed, multiple devices require explicit choice,
     and devices are never aggregated/inferred.
   - Installed Games is first on root Play, followed by Continue Playing and
     existing shelves.
   - Only state Installed is included; attention/cleanup/missing states are
     excluded and counted in a compact attention link.
   - Direct Play uses the selected endpoint/source and existing typed launch.
   - `NO_MIGRATION_NEEDED`; use the existing association key and installation
     data.

### Done by implementation agent (15-07-2026, still dirty)

- Full ADR-0007 vertical slice code (protocol, migration 17, server, client,
  frontend, OpenAPI) — not committed.
- Richer player-facing GOG failure / `attention_required` copy (exit hex, bad
  signature, incomplete install) in `GameDetailPage.tsx`.
- Install dialog notes client will verify GOG signature + Inno before approve.
- Client exit/uninstall errors include `0xXXXXXXXX`; failed GOG installs persist
  `state_reason` as `code: message` (NO_MIGRATION_NEEDED — same TEXT column).
- `/DIR="..."` `/LOG="..."` quoting + keep staging after failed started install.
- Packaged E2E now awaits implementation of the locked completion-basis and
  cleanup/Ignore packet; Plasma Pong and Drive Desktop root must be preserved.

## First actions for the next agent

1. Read the required documents above (including glossary: ADR = Architecture
   Decision Record).
2. Run SuitCode warmup/context if available.
3. You are a **regular implementation agent** for the now-bounded ADR-0007
   completion packet. Implement web-authorized/no-popup install, crash basis,
   locked recognition wording, cleanup/Ignore, migration 18, tests, packaging,
   and E2E exactly as recorded.
4. ADR-0008 is a separate bounded implementation packet for the device-selected
   first Installed Games Play shelf. Implement it after ADR-0007 completion or
   in a separate regular-agent session; do not merge their architecture.
5. Inspect `git status` and the full dirty worktree. Preserve all implementation
   work and applied migration 17; add migration 18 rather than editing 17.
6. Rebuild packaged server/client, rerun Duke install → launch → typed uninstall,
   and run synthetic marker cleanup/Ignore E2E from ADR-0007.
7. Do not commit/push unless the user asks.
8. Keep packaged server + installed client running after verification.
9. The existing Duke attention row/tree has no cleanup marker. Do not synthesize
   one or recursively delete it; preserve it for user manual cleanup or Ignore.
