# Data Schema

This document describes the complete data schema for MyGamesAnywhere.

## Storage Technologies

**NO SERVER DATABASE!** MyGamesAnywhere uses:

- **Client Cache:** SQLite 3.35+ (for fast local queries and offline access)
- **Cloud Sync:** JSON files in user's cloud storage (Google Drive, OneDrive)

## Design Principles

1. **User owns data** - All data stored in user's cloud storage or local device
2. **Simple sync format** - JSON files for easy debugging and portability
3. **Fast local access** - SQLite cache for UI performance
4. **Support multi-platform games** - Games can have multiple platform versions
5. **Offline-first** - Full functionality with local cache, sync when online
6. **Conflict resolution** - Last-write-wins with timestamp
7. **Privacy-first** - Sensitive data (API keys, OAuth tokens) never synced to cloud

---

## Cloud Storage Structure

All synced data lives in a `.mygamesanywhere/` folder in the user's cloud storage:

```
User's Cloud Storage (Google Drive / OneDrive)
└── .mygamesanywhere/
    ├── library.json          # Game library (all sources)
    ├── playtime.json         # Playtime tracking
    ├── preferences.json      # User preferences
    ├── sync-meta.json        # Sync metadata
    └── secrets.encrypted     # (Optional) Encrypted config for multi-device setup
```

---

## Cloud Storage Schema (JSON Files)

### library.json

Contains the user's complete game library from all sources.

**Format:**
```json
{
  "version": "1.0",
  "lastModified": "2024-10-03T10:30:00Z",
  "deviceId": "desktop-abc123",
  "deviceName": "John's Desktop",
  "games": [
    {
      "id": "steam-440",
      "title": "Team Fortress 2",
      "source": "steam",
      "sourceId": "440",
      "platform": "windows",
      "installStatus": "installed",
      "lastPlayed": "2024-10-02T18:45:00Z",
      "metadata": {
        "developer": "Valve",
        "publisher": "Valve",
        "releaseDate": "2007-10-10",
        "genres": ["FPS", "Action"],
        "coverUrl": "https://cdn.cloudflare.steamstatic.com/...",
        "igdbId": 472
      },
      "addedAt": "2024-09-15T12:00:00Z",
      "updatedAt": "2024-10-02T18:45:00Z"
    },
    {
      "id": "gdrive-game123",
      "title": "Celeste",
      "source": "gdrive",
      "sourceId": "1a2b3c4d5e",
      "platform": "windows",
      "installStatus": "available",
      "metadata": {
        "developer": "Maddy Makes Games",
        "publisher": "Maddy Makes Games",
        "releaseDate": "2018-01-25",
        "genres": ["Platformer", "Indie"],
        "igdbId": 26842
      },
      "cloudPath": "/Games/Celeste.zip",
      "addedAt": "2024-09-20T14:30:00Z",
      "updatedAt": "2024-09-20T14:30:00Z"
    }
  ]
}
```

**Fields:**
- `version` (string): Schema version for future migrations
- `lastModified` (ISO 8601): Timestamp of last modification (for conflict resolution)
- `deviceId` (string): ID of device that made last change
- `deviceName` (string): Human-readable device name
- `games` (array): Array of game objects

**Game Object:**
- `id` (string): Unique ID (format: `{source}-{sourceId}`)
- `title` (string): Game title
- `source` (string): Source plugin (`steam`, `gdrive`, `epic`, `local`, etc.)
- `sourceId` (string): ID within source system (Steam app ID, Google Drive file ID, etc.)
- `platform` (string): Platform (`windows`, `macos`, `linux`, `android`, `ios`)
- `installStatus` (string): `installed`, `available`, `downloading`, `error`
- `lastPlayed` (ISO 8601, optional): When game was last launched
- `metadata` (object): Game metadata (from IGDB or other sources)
- `cloudPath` (string, optional): Path in cloud storage (for cloud-stored games)
- `addedAt` (ISO 8601): When game was added to library
- `updatedAt` (ISO 8601): When game info was last updated

---

### playtime.json

Tracks playtime for all games across all devices.

**Format:**
```json
{
  "version": "1.0",
  "lastModified": "2024-10-03T10:30:00Z",
  "deviceId": "desktop-abc123",
  "games": [
    {
      "gameId": "steam-440",
      "totalSeconds": 145800,
      "sessions": [
        {
          "sessionId": "session-1",
          "deviceId": "desktop-abc123",
          "startTime": "2024-10-02T18:00:00Z",
          "endTime": "2024-10-02T20:30:00Z",
          "durationSeconds": 9000
        },
        {
          "sessionId": "session-2",
          "deviceId": "laptop-def456",
          "startTime": "2024-10-01T14:00:00Z",
          "endTime": "2024-10-01T15:00:00Z",
          "durationSeconds": 3600
        }
      ],
      "lastPlayed": "2024-10-02T20:30:00Z"
    }
  ]
}
```

**Fields:**
- `version` (string): Schema version
- `lastModified` (ISO 8601): Last modification timestamp
- `deviceId` (string): Device that made last change
- `games` (array): Array of game playtime objects

**Game Playtime Object:**
- `gameId` (string): References game ID from `library.json`
- `totalSeconds` (number): Total playtime across all sessions
- `sessions` (array): Individual play sessions (optional, for detailed tracking)
- `lastPlayed` (ISO 8601): When game was last played

---

### preferences.json

User preferences and UI settings.

**Format:**
```json
{
  "version": "1.0",
  "lastModified": "2024-10-03T10:30:00Z",
  "deviceId": "desktop-abc123",
  "ui": {
    "theme": "dark",
    "language": "en",
    "defaultView": "grid",
    "gridSize": "medium",
    "sortBy": "lastPlayed",
    "sortOrder": "desc",
    "showHidden": false
  },
  "features": {
    "autoSync": true,
    "syncInterval": 300,
    "cacheMetadata": true,
    "notificationsEnabled": true
  },
  "privacy": {
    "telemetryEnabled": false,
    "analyticsEnabled": false
  }
}
```

**Fields:**
- `version` (string): Schema version
- `lastModified` (ISO 8601): Last modification timestamp
- `deviceId` (string): Device that made last change
- `ui` (object): UI preferences
- `features` (object): Feature toggles
- `privacy` (object): Privacy settings

---

### sync-meta.json

Metadata about sync process (for conflict resolution and device tracking).

**Format:**
```json
{
  "version": "1.0",
  "devices": [
    {
      "deviceId": "desktop-abc123",
      "deviceName": "John's Desktop",
      "platform": "windows",
      "lastSeen": "2024-10-03T10:30:00Z",
      "appVersion": "1.0.0"
    },
    {
      "deviceId": "phone-xyz789",
      "deviceName": "John's iPhone",
      "platform": "ios",
      "lastSeen": "2024-10-03T08:15:00Z",
      "appVersion": "1.0.0"
    }
  ],
  "syncHistory": [
    {
      "timestamp": "2024-10-03T10:30:00Z",
      "deviceId": "desktop-abc123",
      "filesChanged": ["library.json", "playtime.json"]
    }
  ]
}
```

**Fields:**
- `version` (string): Schema version
- `devices` (array): List of devices that have synced
- `syncHistory` (array): Recent sync events (last 100)

---

### secrets.encrypted (Optional)

Encrypted configuration blob for multi-device setup. User can opt-in to sync this.

**Format:** Binary encrypted blob (AES-256)

**Decrypted Format:**
```json
{
  "version": "1.0",
  "encryptionMethod": "AES-256-GCM",
  "plugins": {
    "steam": {
      "apiKey": "ABC123..."
    },
    "gdrive": {
      "oauthToken": "ya29.a0...",
      "refreshToken": "1//...",
      "expiresAt": "2024-10-03T12:00:00Z"
    },
    "igdb": {
      "clientId": "...",
      "clientSecret": "..."
    }
  }
}
```

**Security:**
- Encrypted with user's password (never stored)
- Cloud storage provider cannot decrypt
- User must enter password on new device to decrypt
- Re-encrypts with same password on each device

---

## Local Cache Schema (SQLite)

The client maintains a local SQLite cache for fast queries and offline access.

### games

Cached game library (mirrors `library.json` but queryable).

```sql
CREATE TABLE games (
    id                TEXT PRIMARY KEY,  -- e.g., "steam-440"
    title             TEXT NOT NULL,
    source            TEXT NOT NULL,      -- "steam", "gdrive", etc.
    source_id         TEXT NOT NULL,
    platform          TEXT NOT NULL,
    install_status    TEXT NOT NULL,
    last_played       TEXT,               -- ISO 8601
    metadata_json     TEXT,               -- JSON blob of metadata
    cloud_path        TEXT,
    added_at          TEXT NOT NULL,
    updated_at        TEXT NOT NULL,

    -- Denormalized for fast queries
    developer         TEXT,
    publisher         TEXT,
    release_date      TEXT,
    igdb_id           INTEGER
);

CREATE INDEX idx_games_source ON games(source);
CREATE INDEX idx_games_platform ON games(platform);
CREATE INDEX idx_games_last_played ON games(last_played);
CREATE INDEX idx_games_title ON games(title COLLATE NOCASE);

-- Full-text search
CREATE VIRTUAL TABLE games_fts USING fts5(
    title,
    developer,
    publisher,
    content='games',
    content_rowid='rowid'
);
```

---

### playtime

Cached playtime data (mirrors `playtime.json`).

```sql
CREATE TABLE playtime (
    game_id           TEXT PRIMARY KEY REFERENCES games(id),
    total_seconds     INTEGER NOT NULL DEFAULT 0,
    last_played       TEXT,
    session_count     INTEGER NOT NULL DEFAULT 0
);

CREATE TABLE playtime_sessions (
    session_id        TEXT PRIMARY KEY,
    game_id           TEXT NOT NULL REFERENCES games(id),
    device_id         TEXT NOT NULL,
    start_time        TEXT NOT NULL,
    end_time          TEXT,
    duration_seconds  INTEGER,

    FOREIGN KEY (game_id) REFERENCES games(id) ON DELETE CASCADE
);

CREATE INDEX idx_sessions_game ON playtime_sessions(game_id);
CREATE INDEX idx_sessions_start ON playtime_sessions(start_time);
```

---

### metadata_cache

Cache for game metadata (IGDB data, cover images, etc.).

```sql
CREATE TABLE metadata_cache (
    igdb_id           INTEGER PRIMARY KEY,
    data_json         TEXT NOT NULL,  -- Full IGDB response
    cached_at         TEXT NOT NULL,
    expires_at        TEXT NOT NULL
);

CREATE TABLE media_cache (
    id                INTEGER PRIMARY KEY AUTOINCREMENT,
    game_id           TEXT NOT NULL,
    media_type        TEXT NOT NULL,  -- "cover", "screenshot", "video"
    url               TEXT NOT NULL,
    local_path        TEXT,           -- Path to cached file
    cached_at         TEXT,

    FOREIGN KEY (game_id) REFERENCES games(id) ON DELETE CASCADE
);

CREATE INDEX idx_media_game ON media_cache(game_id);
CREATE INDEX idx_media_type ON media_cache(media_type);
```

---

### sync_state

Tracks sync state for cloud storage.

```sql
CREATE TABLE sync_state (
    key               TEXT PRIMARY KEY,
    value             TEXT NOT NULL
);

-- Example rows:
-- ('last_sync_time', '2024-10-03T10:30:00Z')
-- ('cloud_library_hash', 'sha256:abc123...')
-- ('pending_changes', '["library", "playtime"]')
```

---

## Data Flow

### Startup Sequence

1. **Load Local Cache**
   ```
   Client reads SQLite cache → Displays UI immediately (offline-first)
   ```

2. **Check Cloud Sync**
   ```
   Client checks cloud storage for updates
   ↓
   If cloud file newer than local:
     Download cloud JSON → Merge with local cache → Update SQLite
   ↓
   If local cache newer:
     Upload SQLite → Generate JSON → Write to cloud storage
   ```

3. **Conflict Resolution**
   ```
   If both changed since last sync:
     Compare timestamps (lastModified field)
     ↓
     Last-write-wins: Newer timestamp wins
     ↓
     Merge games: Keep union, prefer newer updatedAt per game
     ↓
     User notified if timestamps very close (< 5 seconds)
   ```

---

### Adding a Game

1. **Plugin Scan**
   ```
   Steam plugin scans local files
   ↓
   Returns game objects
   ```

2. **Store Locally**
   ```
   Client writes to SQLite cache
   ↓
   INSERT INTO games (...)
   ```

3. **Sync to Cloud**
   ```
   Client reads all games from SQLite
   ↓
   Generates library.json
   ↓
   Writes to cloud storage (.mygamesanywhere/library.json)
   ```

4. **Other Devices**
   ```
   Other devices detect cloud file change
   ↓
   Download library.json
   ↓
   Merge with local SQLite
   ↓
   UI updates automatically
   ```

---

### Playing a Game

1. **Launch**
   ```
   User clicks "Play"
   ↓
   Client creates playtime session
   ↓
   INSERT INTO playtime_sessions (...)
   ```

2. **Track Time**
   ```
   Client monitors game process
   ↓
   Updates session end_time and duration_seconds
   ```

3. **Sync Playtime**
   ```
   When game exits:
   ↓
   Client reads playtime from SQLite
   ↓
   Generates playtime.json
   ↓
   Writes to cloud storage
   ```

---

## Migration Strategy

### Version Updates

When schema changes, the `version` field in JSON files allows for migrations:

```typescript
async function migrateLibrary(data: any): Promise<LibraryV1_1> {
  if (data.version === "1.0") {
    // Migrate from 1.0 to 1.1
    return {
      version: "1.1",
      ...data,
      newField: "default value"
    };
  }
  return data;
}
```

### Adding Fields

- **Backward compatible:** Add optional fields with defaults
- **Breaking changes:** Increment major version, provide migration

---

## Privacy and Security

### What's Synced

✅ **Safe to sync (in cloud storage):**
- Game titles, IDs, platforms
- Playtime data
- UI preferences
- Metadata (IGDB IDs, cover URLs)

❌ **Never synced (local only):**
- API keys (stored in OS keychain)
- OAuth tokens (re-authorize on each device)
- Local file paths (`C:\Program Files\Steam\...`)
- Cached media files (too large)

### Optional Encrypted Sync

User can opt-in to sync encrypted config:
- Encrypted with user's password
- Allows easy multi-device setup
- Cloud storage provider cannot decrypt
- User must remember password!

---

## Benefits of This Design

**For Users:**
- ✅ Own their data (in their cloud storage)
- ✅ Can export anytime (just JSON files)
- ✅ Can inspect their data (human-readable)
- ✅ Works offline (local cache)

**For Developers:**
- ✅ No server database to maintain
- ✅ No migrations to run
- ✅ Easy debugging (inspect JSON files)
- ✅ Simple versioning (version field in JSON)

**For Open Source:**
- ✅ No infrastructure required
- ✅ Contributors don't need server access
- ✅ Easy to test (mock JSON files)
- ✅ Clear data ownership (user's cloud)

---

See [ARCHITECTURE.md](./ARCHITECTURE.md) for overall system design.
