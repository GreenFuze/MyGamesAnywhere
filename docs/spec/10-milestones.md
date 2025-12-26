
# Milestones & Tasks

## M0 — Repo & tooling
- Monorepo workspaces
- Desktop shell (Capacitor + Electron) + typed IPC
- CI: typecheck/lint/tests

## M1 — Core + SQLite
- Domain models + repo interfaces
- SQLite schema + migrations + FTS
- Library UI reads/writes local data

## M2 — Plugin API + Host + UI
- `plugin-manifest` schema + config schema handling
- Loader for built-in + user-installed plugins
- Permission declarations + prompts
- Circuit breaker + diagnostics
- Plugins screen (install zip, enable/disable, configure)

## M3 — Google Drive auth + sync
- Shared OAuth client integration
- `.mygamesanywhere/` folder management
- JSON schema validation
- LWW merge engine + sync coordinator UI

## M4 — Drive installers source plugin
- Drive folder selection (multiple)
- Recursive scan under selected folders
- Installer list UI
- Match flow + persisted mapping

## M5 — Steam source plugin
- Steam installed scan
- Protocol launch
- Playtime session best-effort tracking

## M6 — Metadata/media plugins
- LaunchBox export importer
- IGDB integration + caching
- SteamGridDB artwork + cache + eviction

## M7 — Polish
- Better diagnostics bundle export
- Settings UX refinement
- Performance passes (Drive scan incremental; caching)
