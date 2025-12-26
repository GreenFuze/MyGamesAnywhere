
# Architecture

## Tech stack
- UI: Ionic + React
- Runtime: Capacitor + Electron (desktop)
- Data: SQLite (local), JSON (cloud sync)
- Plugins: TypeScript modules loaded client-side

## Core architecture principles
- Clean layering: Domain ← Application ← Infrastructure
- Fail-fast errors with context; never silently swallow plugin failures
- Deterministic cleanup patterns (try/finally; explicit resource lifetimes)
- Schema validation at boundaries (cloud JSON, plugin IO)

## Runtime split
- Electron **Main**:
  - filesystem access
  - process launching
  - SQLite access
  - plugin host execution
  - cloud sync (Drive)
  - secrets/keychain access
- Electron **Renderer**:
  - Ionic React UI
  - calls into Main via typed IPC bridge

## IPC (typed bridge)
Expose a narrow set of application use-cases to the renderer, e.g.:
- `library.list(query)`
- `library.get(gameId)`
- `library.upsert(gamePatch)`
- `scan.run(sourcePluginId, options)`
- `sync.pull()`, `sync.push()`, `sync.status()`
- `launch.start(gameId)`
- `plugins.list()`, `plugins.install(zipPath)`, `plugins.enable(id, bool)`, `plugins.configure(id, config)`

Do not expose raw filesystem or arbitrary process execution directly to renderer.

## Monorepo layout (recommended)
- `apps/desktop/` — Capacitor + Electron app, preload bridge
- `packages/core/` — domain entities, invariants, value objects
- `packages/app/` — use-cases (scan, sync, match, launch, playtime)
- `packages/storage/` — SQLite schema/migrations; repositories; FTS
- `packages/sync/` — JSON schemas; merge engine; Drive adapter
- `packages/plugin-api/` — capability interfaces; manifest/config schemas; error types
- `packages/plugin-host/` — plugin loader; permission prompts; circuit breaker
- `packages/platform/` — fs/process/keychain/http abstractions
- `plugins/*` — shipped plugins (also installable)

## Error envelope (standardized)
All cross-boundary errors (IPC, plugin host) should conform to:

- `code`: stable string (e.g., `DRIVE_AUTH_FAILED`)
- `message`: user-friendly summary
- `context`: structured details (ids, paths redacted as needed)
- `cause`: nested error if available
- `hint`: optional operator guidance

## Logging
- Local log file(s) per device profile.
- Plugin logs are tagged with `pluginId`, `capability`, and correlation IDs.
