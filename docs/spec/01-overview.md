
# Overview

## Objective
Build **MyGamesAnywhere**, a desktop Game Manager/Launcher that aggregates a user’s library across sources and reliably launches games.

## Platforms
- **MVP:** Windows desktop
- **Codebase:** cross-platform (desktop + mobile later)

## Hard constraints
- **No backend server.** Everything runs on-device.
- **Sync via user cloud storage:** Google Drive (MVP) using OAuth2.
- **Offline-first:** local SQLite cache + cloud JSON as canonical sync artifacts.
- **Plugins:** source + metadata + media are plugins. MVP must support user-installed plugins.

## MVP sources
1. **Steam**: scan installed games locally; launch via store protocol.
2. **Google Drive “installers”**: scan recursively under *user-selected folders only* (multiple folders supported) for setup files:
   - `.exe`, `.msi`, `.zip`, `.7z`, `.rar`, `.iso`

## MVP metadata & media providers (plugins)
- LaunchBox export importer (local file)
- IGDB (BYO key)
- SteamGridDB (BYO key)

## Launch priority
1. Store protocol (when available, e.g., `steam://...`)
2. Installer-created shortcut (lnk)
3. Direct executable + args + working dir (local-only)

## MVP user stories
1. Connect Google Drive; create/use `.mygamesanywhere/` folder; sync JSON.
2. Select Drive folders; scan recursively for installers; list results.
3. Auto-match installer → game candidates with confidence; user confirms/overrides; mapping persists.
4. Scan installed Steam games; add to library.
5. Browse library (grid/list), search, filter by source/installed, edit tags/categories.
6. Fetch metadata/artwork via provider plugins.
7. Launch and track playtime; sync playtime totals.

## Explicit non-goals (MVP)
- No store purchasing, friends/chat, achievements, streaming, mod management.
- No “scan entire Google Drive”.
- No syncing of local paths, binaries, or cached media.
- Xbox installed scanning is not MVP (can be added later via plugin).

## Key risks
- Drive scanning performance (must be folder-scoped + incremental).
- Matching installers to games (must be explainable + user override).
- Plugin safety (permissions + circuit breaker + diagnostics).
