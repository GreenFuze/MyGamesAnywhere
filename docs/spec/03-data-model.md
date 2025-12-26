
# Data Model

## Split brain: cloud canonical vs local device state
Cloud JSON is canonical for:
- library (games, tags/categories, identity links)
- preferences
- playtime totals (and optionally sessions)

Local SQLite holds device-specific info:
- resolved launch targets (exe/lnk/protocol mappings)
- install locations and “installed” status
- cached media paths
- plugin state (enabled, config, breaker status)
- scan cursor/state for incremental Drive scans

## Cloud files (Google Drive)
Stored in `.mygamesanywhere/`:
- `library.json`
- `playtime.json`
- `preferences.json`
- `sync-meta.json`

### `library.json` (conceptual)
- `version` (int)
- `updatedAt` (ISO8601)
- `games[]`
  - `gameId` (UUID)
  - `title`
  - `sourceRefs[]`
    - `{ kind: "steam", appId: string }`
    - `{ kind: "gdriveInstaller", fileId: string }`
  - `tags[]`
  - `categories[]`
  - `identity` (optional):
    - `launchbox` (id or key)
    - `igdb` (id)
  - `updatedAt` (ISO8601)

### `playtime.json` (conceptual)
- `version`
- `updatedAt`
- `totalsByGameId: { [gameId]: seconds }`
- optional `sessions[]` (bounded retention)
  - `sessionId`, `gameId`, `startedAt`, `endedAt`, `seconds`, `source` ("steam" | "exe" | ...)

### `preferences.json`
- view mode, sort order, default filters, theme

### `sync-meta.json`
- `deviceId` (UUID)
- `lastSyncAt`
- `lwwPolicy: "record"`

## Local SQLite (suggested tables)
- `games_local`:
  - `gameId` PK
  - `installed` boolean
  - `installPath` (nullable)
  - `launchProtocol` (nullable)
  - `shortcutPath` (nullable)
  - `exePath` (nullable), `exeArgs` (nullable), `workingDir` (nullable)
- `media_cache`:
  - `gameId`, `kind` (cover/background/icon), `path`, `provider`, `updatedAt`
- `plugin_state`:
  - `pluginId` PK, `enabled`, `configJson`, `failCount`, `breakerUntil`, `lastErrorJson`
- `drive_scan_folders`:
  - `folderId` PK, `displayName`, `enabled`
- `drive_scan_cursor`:
  - `folderId`, `pageToken`/`deltaToken` (provider-specific), `updatedAt`
- `search_fts`:
  - FTS virtual table (title, tags, providers ids)

## Privacy constraints
- Never write local paths or cached media paths into cloud JSON.
- Credentials and API keys must not be in cloud JSON.
