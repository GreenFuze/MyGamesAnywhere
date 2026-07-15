# MGA implementation handoff — 15-07-2026

> **AUTHORITATIVE WORKING HANDOFF (15-07-2026).** Read this file before
> changing the current worktree. It supersedes the 14-07-2026 handoff for live
> implementation, test, packaging, runtime, and next-task status. Accepted ADRs
> remain authoritative for design. Do not discard the dirty worktree.

## Required reading and precedence

1. Repository `AGENTS.md` and `CLAUDE.md`.
2. This handoff.
3. `docs/architecture/agent-responsibility-boundary.md`.
4. `docs/architecture/0001-mga-client-architecture.md`.
5. `docs/architecture/mga-device-protocol-v1.md`.
6. ADRs `0002`–`0006`, `player-facing-language.md`, and
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
- Do not reset, clean, revert, overwrite, or discard the dirty worktree.
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
- HEAD: `b9c0876 docs: keep credentials out of handoff`
- Remote: `main` was synchronized with `origin/main` before the current dirty
  slice.
- Committed baseline:
  - `0e2d8ba feat: add device-aware library and managed play`
  - `b9c0876 docs: keep credentials out of handoff`
- Current worktree is intentionally dirty with the completed but uncommitted
  ZIP/7z/RAR slice. It includes modified protocol/client/server/frontend/docs,
  new archive extractor code, third-party notices, and licensed binary test
  fixtures. Run `git status --short` for the exact list.
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

## Current uncommitted slice: bundled 7z and RAR support

This slice is implemented, documented, packaged, and end-to-end verified but
not committed.

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

## Verification completed for the dirty slice

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
  - PID `90164`
  - `C:\src\github.com\GreenFuze\MyGamesAnywhere\server\bin\mga_server.exe`
  - portable mode, port `8900`
  - started with `MGA_GOOGLE_DRIVE_DESKTOP_ROOT=G:\My Drive`
- Installed MGA Client:
  - PID `45996`
  - `%LOCALAPPDATA%\Programs\MGA Client\mga-client-agent.exe`
- HTTP health: `http://127.0.0.1:8900` returned 200.
- SQLite schema version: 16.
- Plasma Pong installation:
  - root `C:\Games`
  - path `C:\Games\Plasma Pong`
  - target `Plasma Pong/Plasma Pong.exe`
- No test game process is running.

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

## Next implementation decision and order

The next planned architecture slice is native EXE/BIN installers and
prerequisites:

This begins with an **architecture-capable/expensive agent**, not a regular
implementation agent:

1. Decide the first supported installer families and silent/interactive policy.
2. Define typed command schemas; never expose generic shell/exec.
3. Define local-interaction, UAC/elevation-helper, timeout, cancellation, and
   rollback semantics.
4. Model shared prerequisites separately from game-owned files.
5. Record migration/compatibility/security decisions and a bounded
   implementation packet.

Only after those decisions are recorded should a **regular implementation
agent** add server authorization, persistence/migration, client implementation,
progress, UI, tests, packaging, and real E2E. It must stop and escalate any
ambiguity rather than extending the architecture itself.

After that, planned work includes persisted install-root preferences,
external-removal reconciliation, disk estimates, cancel/retry, richer logs,
repair/update, and save-sync hooks. Do not guess unresolved installer or
elevation policy; record the decision first.

## First actions for the next agent

1. Read the required documents above.
2. Run SuitCode warmup/context if available.
3. Identify whether the assigned task is architecture or bounded implementation
   using `agent-responsibility-boundary.md`. A regular agent must stop before
   unresolved architecture.
4. Inspect `git status`, the complete dirty diff, and especially:
   - `client/internal/clientapp/archive_extractors.go`
   - `client/internal/clientapp/archive_installer.go`
   - `protocol/device/v1/installation.go`
   - `server/internal/http/archive_install_controller.go`
   - `server/frontend/src/pages/GameDetailPage.tsx`
   - the test fixtures/notices and packaging changes.
5. Re-run the focused archive tests before editing.
6. Do not commit/push the completed 7z/RAR slice unless the user asks.
7. Keep the packaged server and installed client running unless replacement is
   required for the next verified build.
