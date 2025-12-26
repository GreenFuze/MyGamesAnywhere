
# Plugins

## Goals
- Make sources (Steam, Drive installers) and providers (IGDB, SteamGridDB, LaunchBox export) pluggable.
- Support **user-installed plugins in MVP**.
- Keep the host stable; evolve plugins over time.

## Plugin types (capabilities)
A plugin declares one or more capabilities:
- `source.scan` — find games / installers
- `source.launch` — launch a game (protocol/shortcut/exe)
- `source.monitor` — monitor a running session (optional)
- `metadata.search` / `metadata.fetch`
- `media.search` / `media.fetch`

## Plugin packaging
- A plugin is distributed as a zip or folder:
  - `manifest.json`
  - `dist/index.js` (bundled)
  - optional assets

## Plugin manifest (concept)
- `pluginId` (reverse-DNS recommended)
- `name`, `version`
- `capabilities[]`
- `permissions`:
  - `networkHosts[]` (e.g., `api.igdb.com`)
  - `driveScopes` (read-only file metadata; download optional)
  - `localFsScopes[]` (paths)
  - `processLaunch` (boolean)
  - `secrets` (list of secret keys names)
- `configSchema` (JSON Schema)

Schema: see `schemas/plugin-manifest.schema.json`.

## Host responsibilities
- Validate manifest and config schema (fail fast).
- Provide a stable host API surface (typed).
- Apply permission prompts when plugin requests restricted operations.
- Wrap every plugin call with:
  - timeout
  - structured error capture
  - circuit breaker

## Circuit breaker
- Track consecutive failures per `(pluginId, capability)`.
- After N failures, mark capability as disabled for `breakerDuration`.
- UI shows:
  - reason, last error, “Re-enable” action

## Installation flow
- User selects plugin zip.
- Host unpacks to `{appData}/plugins/{pluginId}/{version}/...`
- Host verifies:
  - manifest validity
  - entry module loads
  - no duplicate active versions unless explicitly allowed
- Plugin appears in “Plugins” UI.

## Versioning & compatibility
- Manifest declares `apiVersion`.
- Host refuses plugins with unsupported major version.

## Security notes
- Full sandboxing is hard inside Electron; MVP focuses on:
  - explicit permission declarations + user prompts
  - restricted host services (no raw Node access from plugin unless explicitly allowed)
  - audits and clear trust model: “plugins run locally and can access local data per permissions”.
