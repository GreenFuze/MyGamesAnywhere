# Headless-first pivot handoff — 2026-08-29

This document is a code-state and verification snapshot for handing
`codex/mga-87-headless-first` to a fresh session. It is not a task list,
roadmap, product specification, or source of current work status.

Before doing anything, query Jira. Jira MGA is the only source of truth for
open work, priority, assignment, acceptance criteria, dependencies, and
progress. Confluence MGA is the source of truth for current product, UX,
architecture, security, and operating decisions. If this file conflicts with
either system, Jira and Confluence win.

## Fresh-session bootstrap

1. Read `AGENTS.md`, `CLAUDE.md`, and `docs/agent-bootstrap.md` completely.
2. Read the Confluence [New Agent Kickoff Prompt](https://greenfuzer.atlassian.net/wiki/spaces/MG/pages/2654209).
3. Read the Confluence pages named by the kickoff prompt, especially the
   [Server-First Product Charter](https://greenfuzer.atlassian.net/wiki/spaces/MG/pages/20086785),
   [ADR-0047](https://greenfuzer.atlassian.net/wiki/spaces/MG/pages/20119553),
   [Architecture Overview](https://greenfuzer.atlassian.net/wiki/spaces/MG/pages/2425048),
   [Management Console UX](https://greenfuzer.atlassian.net/wiki/spaces/MG/pages/21200897),
   [Frontend API Clients](https://greenfuzer.atlassian.net/wiki/spaces/MG/pages/20905993),
   and [Legacy Client Retirement Plan](https://greenfuzer.atlassian.net/wiki/spaces/MG/pages/20709378).
4. Run this JQL rather than relying on the snapshot below:

   ```text
   project = MGA AND statusCategory != Done ORDER BY priority DESC, Rank ASC, created ASC
   ```

5. Read the selected Jira issue and its links fully. Set `Assigned agent` to a
   stable session name and move that issue to In Progress before changing code.
6. Preserve all unrelated or uncommitted work. Do not reset, clean, revert, or
   overwrite it.

Older dated handoffs describe the pre-pivot player/client product and are
historical evidence only.

## Locked product boundary

The MGA Server stays in Go. It owns profiles and authorization, source
connections, canonical game identity and copies, catalog offers and versioned
availability history, authorized game content, metadata and media, compliant
emulator/runtime artifacts, achievements, jobs, storage, audit/recovery data,
the management WebUI, and scoped frontend APIs.

Established frontend applications own device-local download placement,
installation or extraction, emulator configuration, storefront
authentication, and execution. MGA is a store-like library/content source to
Playnite, LaunchBox, Pegasus, and future mobile integrations. The first-party
MGA Client/device agent, `mga://`, browser-emulation player, and MGA-owned
install, repair, uninstall, launch, and elevation workflows are retired.

Google Play or Play Games support must be bounded by supported APIs and lawful
evidence sources; consumer-library discovery is not promised without one.
Protected storefront packages are never redistributed. Runtime delivery fails
closed unless licensing, provenance, integrity, and platform compatibility are
adequately represented.

## UX boundary confirmed by product feedback

The WebUI is a management console, not a place to repeat architectural slogans.
Do not restore permanent copy such as “server-owned execution boundary” or a
“server online” badge. A successfully loaded WebUI already proves the server
responded. Connectivity belongs in actionable request failure, offline, stale,
and recovery states. The shell should prioritize profiles and management work.

Commit `920df6cf` implements this feedback by removing the sidebar boundary
copy, permanent connectivity badge, and redundant overview architecture card.
The refresh action now reflects actual management-query activity.

## Git and implemented pivot baseline

- Repository: `C:\src\github.com\GreenFuze\MyGamesAnywhere`
- Working branch: `codex/mga-87-headless-first`
- Pre-pivot baseline: `5be6c1f3`
- Protected checkpoint tag: `mga-87-pre-headless-first`
- Remote: `origin` (`git@github.com:GreenFuze/MyGamesAnywhere.git`)

Implemented commits before this handoff:

| Commit | Jira | Result |
| --- | --- | --- |
| `d036fd67` | MGA-88 | Catalog offers, versions, availability events, history, and migration 39 |
| `b17c28c7` | MGA-90 | Versioned content-delivery/materialization API |
| `869f4eea` | MGA-94 | Compliance-gated runtime artifact registry and migration 40 |
| `8d0afa51` | MGA-95 | Scoped frontend API clients, capability discovery, and migration 41 |
| `8fdc3cfc` | MGA-97 | Active local install/play authority retired behind typed compatibility responses |
| `d5c33a02` | MGA-100 | Responsive, profile-aware management console shell |
| `920df6cf` | MGA-100 | Product-feedback cleanup of redundant boundary/connectivity chrome |

The final documentation commit follows these commits. Use `git log`, `git
status`, and Jira to verify the branch instead of assuming this snapshot is
still current.

## Code map

The main server-side pivot packages are:

- `server/internal/catalog` — provider offers, versions, availability, history;
- `server/internal/contentdelivery` — authorized manifests and materialization;
- `server/internal/runtimeartifact` — emulator/runtime artifacts and compliance;
- `server/internal/frontendauth` — scoped frontend clients and tokens;
- `server/internal/legacyretirement` — explicit retirement boundary and reports;
- `server/internal/http/router.go` — active API composition and compatibility routes;
- `server/openapi.yaml` — public contracts.

The management frontend is primarily:

- `server/frontend/src/App.tsx`;
- `server/frontend/src/layouts/ManagementShell.tsx`;
- `server/frontend/src/components/management`;
- `server/frontend/src/pages/management`;
- `server/frontend/src/lib/navigationRoutes.ts`;
- `server/frontend/src/api/client.ts`.

The shell has eight management routes: Overview, Profiles, Library, Catalog,
Sources, Artifacts, Achievements, and System. It intentionally provides
summary-level workflows pending the deeper Jira items; do not reinterpret that
as a return to player/install features.

## Compatibility, persistence, and security

Legacy device/install/play URLs remain temporarily discoverable but return a
typed HTTP 410 compatibility response. Protected legacy routes still enforce
the existing session/profile authorization boundary before returning that
response. Historical client data is preserved read-only and exportable through
the administrative `/api/legacy-client-data/report` endpoint.

Migrations 39, 40, and 41 are immutable evidence for catalog, runtime
artifacts, and frontend API clients. MGA-97, MGA-100, this UI feedback change,
and the documentation handoff make no persisted SQLite, JSON, or configuration
change: **NO_MIGRATION_NEEDED** because existing installations keep the same
stored representation and migration sequence.

Do not delete legacy persisted data merely because active execution routes are
retired. Follow the Confluence retirement plan and the Jira acceptance criteria
for any later physical code or data removal.

## Jira snapshot at handoff time

This snapshot was captured on 2026-08-29 and becomes stale immediately. It is
included only so a fresh session can detect a surprising Jira or branch mismatch.
The JQL above is authoritative.

| Priority/status at snapshot | Jira | Scope summary |
| --- | --- | --- |
| Highest / In Progress | MGA-87 | Headless-first pivot parent |
| Highest / Backlog | MGA-98 | Remove local-client and device-agent support |
| Highest / Backlog | MGA-99 | Remove first-party player/browser-emulation surface |
| Highest / Backlog | MGA-106 | Playnite MGA library/store plugin |
| Highest / Backlog | MGA-111 | Migration, security, adapter, and end-to-end release validation |
| High / Backlog | MGA-92 | Harden metadata and media delivery contracts |
| High / Backlog | MGA-96 | Stabilize retained source integrations |
| High / Backlog | MGA-101 | Deeper Library and game-details management UI |
| High / Backlog | MGA-102 | Catalog and provider-availability monitoring UI |
| High / Backlog | MGA-103 | Profiles, access, API clients, and integrations UI |
| High / Backlog | MGA-104 | Artifacts, storage, jobs, and compliance UI |
| High / Backlog | MGA-107 | Frontend adapter SDK and conformance suite |
| High / Backlog | MGA-110 | Server-only packaging and deployment |
| Medium / Backlog | MGA-105 | Bounded Google Play/Play Games catalog support |
| Medium / Backlog | MGA-108 | LaunchBox adapter |
| Medium / Backlog | MGA-109 | Pegasus/mobile metadata and content adapter |

At this snapshot, the active routes were already retired, but physical
client/device packages, the `client/` tree, pairing/grant/command code, and
runtime-composition remnants remained for MGA-98. Old player and browser
emulation components/tests remained compiled but unreachable for MGA-99. The
management pages were deliberately shallow pending MGA-101 through MGA-104.
Packaging still had pre-pivot client expectations pending MGA-110. Read the
individual Jira issue and dependency links before deciding order or scope.

## Verification evidence

The pivot baseline was verified on Windows with:

```powershell
cd C:\src\github.com\GreenFuze\MyGamesAnywhere\server
go test ./...
go vet ./...
go test -race ./internal/http ./internal/db ./internal/legacyretirement

cd frontend
npm run test:unit
npm run build
```

Results at handoff:

- full Go tests passed;
- Go vet passed;
- targeted race tests passed;
- frontend unit tests passed 69/69;
- TypeScript and Vite production build passed;
- a real authenticated Go-server smoke test returned HTTP 200 for management
  statistics, catalog, and artifacts, and typed JSON HTTP 410 for retired play.

The UI feedback cleanup was followed by another 69/69 frontend unit-test pass
and successful production build.

## Isolated local review instance

The review server was started from `server` with:

```powershell
go run ./cmd/server `
  --runtime-mode user `
  --app-dir C:\src\github.com\GreenFuze\MyGamesAnywhere\server `
  --data-dir C:\src\github.com\GreenFuze\MyGamesAnywhere\.codex-review `
  --no-tray
```

Review URL: `http://127.0.0.1:8900/overview`  
Review profile: `MGA Administrator`

The password is intentionally not committed. Ask the user for the temporary
review credential or create a new isolated review data directory. Never reset
the user's normal MGA administrator or data. `.codex-review` is isolated and
git-ignored. The server serves `server/frontend/dist`; rebuilding the frontend
updates the reviewed SPA without changing production user data.

## How to continue safely

Select work only from the current Jira query, claim exactly the executable Jira
item being implemented, and read its acceptance criteria and links. Record a
genuinely unresolved product, protocol, persistence, security, identity,
licensing, elevation, destructive-filesystem, or dependency decision in
Confluence before implementation. Add a versioned migration for persisted
schema/JSON/config changes; otherwise record `NO_MIGRATION_NEEDED` with the
compatibility reason. Verify in proportion to risk, leave exact evidence in
Jira, transition the issue accurately, and put every new open item in Jira—not
in this file.
