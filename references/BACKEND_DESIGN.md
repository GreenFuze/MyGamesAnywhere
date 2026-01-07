# MyGamesAnywhere Server (v1) — Backend Design

## Executive Summary

This document outlines the architecture and implementation plan for **MyGamesAnywhereServer v1**, a multi-user backend server for game discovery, metadata aggregation, and media caching. The server runs as a Docker container, provides an HTTP API and SSR Web UI, and supports user-installable plugins that communicate via JSON over stdin/stdout.

**Key Features:**
- Multi-user authentication with persistent sessions (no expiry)
- Per-user plugin account connections (encrypted token storage)
- Game discovery via source plugins
- Free-form metadata with KV flattening and known-key extraction
- Media asset caching with configurable limits
- Background job system for async operations
- SSR Web UI for administration and browsing

---

## 1. Language Selection

**Selected: Go**

Rationale:
- You already know Go
- Excellent performance and concurrency
- Strong standard library (HTTP, JSON, templates)
- Simple deployment (single binary in Docker)
- Great testing support

---

## 2. Architecture Overview

### 2.1 High-Level Architecture

```
┌─────────────────────────────────────────────────────────────┐
│                    HTTP Server (0.0.0.0:PORT)               │
│              REST API + SSR Web UI + WebSocket               │
└──────────────────────┬──────────────────────────────────────┘
                       │
                       ▼
┌─────────────────────────────────────────────────────────────┐
│                    Application Layer                        │
│  ┌──────────┐  ┌──────────┐  ┌──────────┐  ┌──────────┐  │
│  │   Auth   │  │ Account  │  │Discovery │  │ Catalog  │  │
│  │ Service  │  │ Service  │  │ Service  │  │ Service  │  │
│  └────┬─────┘  └────┬─────┘  └────┬─────┘  └────┬─────┘  │
│       │             │              │              │         │
│  ┌────▼─────────────▼──────────────▼──────────────▼─────┐  │
│  │            Plugin Host Manager                       │  │
│  │  - Spawns plugin processes                           │  │
│  │  - JSON over stdin/stdout                           │  │
│  │  - Timeouts, concurrency, circuit breaker           │  │
│  └────┬─────────────────────────────────────────────────┘  │
│       │                                                     │
│  ┌────▼──────────────┐  ┌──────────────┐  ┌──────────┐  │
│  │ Metadata Service  │  │ Media Service│  │Job Service│  │
│  └────┬──────────────┘  └──────┬───────┘  └────┬─────┘  │
└───────┼─────────────────────────┼───────────────┼─────────┘
        │                         │               │
        ▼                         ▼               ▼
┌─────────────────────────────────────────────────────────────┐
│                    Domain Layer                             │
│  ┌──────────┐  ┌──────────┐  ┌──────────┐  ┌──────────┐    │
│  │   User   │  │  Game    │  │ Metadata │  │  Media   │    │
│  │  Entity  │  │  Entity  │  │  Entity  │  │  Entity  │    │
│  └──────────┘  └──────────┘  └──────────┘  └──────────┘    │
└─────────────────────────────────────────────────────────────┘
        │                         │               │
        ▼                         ▼               ▼
┌─────────────────────────────────────────────────────────────┐
│                  Infrastructure Layer                       │
│  ┌──────────┐  ┌──────────┐  ┌──────────┐  ┌──────────┐    │
│  │Database  │  │FileSystem│  │HTTPClient│  │  Search │    │
│  │ (SQLite) │  │  Storage  │  │          │  │  (FTS5) │    │
│  └──────────┘  └──────────┘  └──────────┘  └──────────┘    │
└─────────────────────────────────────────────────────────────┘
```

### 2.2 Core Components

1. **HTTP Server**: REST API, SSR Web UI (`html/template`), optional WebSocket
2. **Auth Service**: Users, sessions (forever), password hashing, session revocation
3. **Account Service**: Per-user plugin accounts (encrypted tokens, settings)
4. **Discovery Service**: Orchestrates `source.library.list` per user account
5. **Catalog Service**: List/search/browse games (global and per-user)
6. **Metadata Service**: Fetch/store free-form metadata, KV flattening, known-key extraction
7. **Media Service**: Fetch/cache/serve media assets with deduplication
8. **Plugin Service**: Install/configure/enable plugins
9. **Job Service**: Background queue with progress tracking
10. **Plugin Host**: Spawns plugin processes, JSON IPC, supervision

### 2.3 Server Responsibilities

**In Scope (v1):**
- Manage users + sessions
- Manage plugins (install/enable/configure)
- Run discovery per user account (source plugins)
- Maintain local cache:
  - Discovered games
  - Ownership ("which user owns which game via which source plugin")
  - Raw metadata blobs (free-form)
  - Flattened metadata key/value index
  - Extracted known keys for consistent UI + search
  - Cached media files
- Provide HTTP API + SSR Web UI

**Not in Scope (v1):**
- Install/uninstall/run games (clients do on-device)
- Device inventory (server doesn't track installs)
- Remote launching / orchestration

---

## 3. Docker Runtime

### 3.1 Container Layout

```
/app/server          # Go binary
/data                # Mounted volume
  /data/db.sqlite
  /data/media/
  /data/plugins/
  /data/config.json  # Optional
```

### 3.2 Configuration

- **Bind**: `0.0.0.0:${PORT}` (default 8080)
- **TLS**: Recommend termination via reverse proxy (sessions are long-lived)
- **Hardening**:
  - Run as non-root
  - Request size caps + timeouts
  - Rate limiting (basic)
  - Session cookies: `HttpOnly`, `SameSite=Lax` (or `Strict`), `Secure` when behind TLS

### 3.3 Environment Variables

- `PORT` (default: 8080)
- `CONFIG_ENCRYPTION_KEY` (optional, 32 bytes; if not set, no encryption)
- `LOG_LEVEL` (default: info)

---

## 4. Authentication Model

### 4.1 Bootstrap Behavior

On first run (no users), create:
- Username: `admin`
- Password: `admin`
- Role: `admin`

Server logs a warning on startup until `admin` password is changed.

### 4.2 Sessions

- **Stored in DB forever** (no expiry)
- Multiple active sessions per user allowed
- Support:
  - Revoke a session
  - Revoke all sessions for a user
  - View all sessions per user (admin; user can view own sessions)

**Transport:**
- Browser: cookie `mga_session=<token>`
- Optional for non-browser clients: header `Authorization: Session <token>`

### 4.3 Password Hashing

- Use `golang.org/x/crypto/bcrypt` or `golang.org/x/crypto/argon2`
- Store hash in `users.password_hash`

---

## 5. Data Model (SQLite)

### 5.1 Users / Sessions

**users**
- `id` TEXT PK (uuid)
- `username` TEXT UNIQUE
- `password_hash` TEXT
- `role` TEXT (`admin|user`)
- `created_at` INT
- `last_login_at` INT

**sessions**
- `id` TEXT PK (uuid)
- `user_id` TEXT FK → users.id
- `session_secret_hash` TEXT (store hash only)
- `created_at` INT
- `last_seen_at` INT
- `revoked_at` INT NULL
- `revoked_reason` TEXT NULL
- `user_agent` TEXT NULL
- `ip` TEXT NULL

### 5.2 Plugins

**plugins**
- `plugin_id` TEXT PK
- `version` TEXT
- `api_major` INT
- `manifest_json` TEXT
- `enabled` INT (boolean)
- `installed_at` INT
- `updated_at` INT

**plugin_health**
- `plugin_id` TEXT PK
- `status` TEXT (`healthy|unhealthy|crashed|disabled`)
- `last_error_json` TEXT NULL
- `updated_at` INT

**plugin_global_config** (optional)
- `plugin_id` TEXT PK
- `config_json` TEXT
- `updated_at` INT

### 5.3 Per-User Plugin Accounts

**user_plugin_accounts**
- `id` TEXT PK (uuid)
- `user_id` TEXT FK → users.id
- `plugin_id` TEXT FK → plugins.plugin_id
- `label` TEXT (e.g., "Steam main")
- `config_ciphertext` BLOB (encrypted JSON, or plaintext if no encryption key)
- `config_nonce` BLOB (for AES-GCM, NULL if not encrypted)
- `created_at` INT
- `updated_at` INT
- `disabled_at` INT NULL

**Encryption:**
- Use `CONFIG_ENCRYPTION_KEY` env var (32 bytes)
- AES-GCM encrypt per-account config JSON before storing
- If key missing: store plaintext (fail-fast only if encryption is required by policy)
- If key provided: fail-fast at startup if key is invalid length

### 5.4 Games + Ownership + Discovery Cache

**games**
- `game_id` TEXT PK (deterministic)
- `display_name` TEXT
- `provider_ids_json` TEXT (JSON object)
- `created_at` INT
- `updated_at` INT

**game_sources**
- `game_id` TEXT FK → games.game_id
- `source_plugin_id` TEXT FK → plugins.plugin_id
- `source_game_key` TEXT
- `source_payload_json` TEXT (free-form)
- `first_seen_at` INT
- `last_seen_at` INT
- PK (`game_id`, `source_plugin_id`)

**user_game_ownership**
- `user_id` TEXT FK → users.id
- `game_id` TEXT FK → games.game_id
- `source_plugin_id` TEXT FK → plugins.plugin_id
- `account_id` TEXT FK → user_plugin_accounts.id
- `first_seen_at` INT
- `last_seen_at` INT
- PK (`user_id`, `game_id`, `source_plugin_id`, `account_id`)

**discovery_runs**
- `id` TEXT PK (uuid)
- `user_id` TEXT FK → users.id
- `account_id` TEXT FK → user_plugin_accounts.id
- `source_plugin_id` TEXT FK → plugins.plugin_id
- `status` TEXT (`queued|running|success|failed`)
- `started_at` INT
- `finished_at` INT
- `error_json` TEXT NULL

**Deterministic `game_id` Rule:**
- Prefer `provider_ids` (stable across plugins) → normalized string → UUIDv5 or hash
- Fallback: `source_plugin_id + ":" + source_game_key` if provider ids missing

### 5.5 Metadata (Free Keys + KV + Known Keys)

**metadata_blobs**
- `id` TEXT PK (uuid)
- `game_id` TEXT FK → games.game_id
- `plugin_id` TEXT FK → plugins.plugin_id
- `facet` TEXT (`core|achievements|time_to_beat|...`)
- `raw_json` TEXT (free-form JSON object)
- `fetched_at` INT
- `ttl_seconds` INT NULL
- `expires_at` INT NULL
- UNIQUE (`game_id`, `plugin_id`, `facet`)

**metadata_kv**
- `id` TEXT PK (uuid)
- `game_id` TEXT FK → games.game_id
- `plugin_id` TEXT FK → plugins.plugin_id
- `facet` TEXT
- `key_path` TEXT (e.g., `title`, `achievements[0].name`, `time_to_beat.main_hours`)
- `value_json` TEXT
- `value_type` TEXT (`string|number|bool|null|object|array`)
- `updated_at` INT
- INDEX (`game_id`)
- INDEX (`key_path`)

**metadata_known**
- `game_id` TEXT PK FK → games.game_id
- `title` TEXT
- `summary` TEXT
- `genres_json` TEXT
- `release_year` INT
- `developers_json` TEXT
- `publishers_json` TEXT
- `updated_at` INT

**Known-Key Behavior:**
- Extract known keys from KV view using priority policy
- Priority order configurable in settings (v1: fixed order, but stored in settings)
- Everything else remains accessible via KV and raw blobs

**games_fts** (FTS5 virtual table)
- `game_id` TEXT
- `title` TEXT
- `summary` TEXT
- `providers` TEXT
- `all_text` TEXT (built from selected KV string values, bounded by max chars per game)

### 5.6 Media (Structured + Cached)

**media_assets**
- `id` TEXT PK (uuid)
- `game_id` TEXT FK → games.game_id
- `plugin_id` TEXT FK → plugins.plugin_id
- `kind` TEXT (`cover|hero|icon|screenshot|trailer|video`)
- `source_uri` TEXT
- `mime` TEXT
- `width` INT
- `height` INT
- `sha256` TEXT (for deduplication)
- `local_path` TEXT NULL
- `fetched_at` INT NULL
- `ttl_seconds` INT NULL
- `expires_at` INT NULL
- UNIQUE (`game_id`, `plugin_id`, `kind`, `source_uri`)

**Filesystem:**
- `/data/media/<game_id>/<asset_id>.<ext>`

**Deduplication:**
- Use `sha256` to detect duplicate assets
- If same `sha256` exists, reuse `local_path` instead of downloading again

**Cache Limits (Settings):**
- `media_cache_limit_per_asset` (bytes, -1 = unlimited)
- `media_cache_limit_per_game` (bytes, -1 = unlimited)
- `media_cache_limit_total` (bytes, -1 = unlimited)
- v1 defaults: all set to -1 (unlimited)

### 5.7 Jobs

**jobs**
- `id` TEXT PK (uuid)
- `type` TEXT (`discovery|metadata_fetch|media_fetch|plugin_install|...`)
- `status` TEXT (`queued|running|success|failed|cancelled`)
- `created_by_user_id` TEXT FK → users.id
- `payload_json` TEXT
- `progress` INT (0-100)
- `message` TEXT
- `created_at` INT
- `started_at` INT NULL
- `finished_at` INT NULL
- `error_json` TEXT NULL

### 5.8 Settings

**settings**
- `key` TEXT PK
- `value_json` TEXT
- `updated_at` INT

**Settings Keys:**
- `media_cache_limit_per_asset` (int, default: -1)
- `media_cache_limit_per_game` (int, default: -1)
- `media_cache_limit_total` (int, default: -1)
- `metadata_known_keys_priority` (array of plugin IDs, default: fixed order)

---

## 6. Plugin System

### 6.1 Plugin Format

**Directory Structure:**
```
/data/plugins/<plugin_id>/
  plugin.json
  bin/plugin          # Executable (Linux inside Docker)
  assets/              # Optional
```

### 6.2 Manifest (`plugin.json`)

```json
{
  "plugin_id": "com.mga.steam",
  "plugin_version": "1.0.0",
  "api_major": 1,
  "kinds": ["source", "metadata", "media"],
  "provides": [
    "source.library.list",
    "metadata.fetch:core",
    "metadata.fetch:achievements",
    "metadata.fetch:time_to_beat",
    "media.fetch"
  ],
  "exec": "bin/plugin",
  "default_timeout_ms": 15000,
  "max_concurrency": 4,
  "config": {
    "account_schema": {
      "type": "object",
      "properties": {
        "api_token": { "type": "string", "x-secret": true }
      },
      "required": ["api_token"]
    }
  }
}
```

**Notes:**
- `account_schema`: JSON Schema for per-user account config (required)
- `global_schema`: Optional JSON Schema for global plugin config (non-secret tuning)

### 6.3 IPC: Length-Prefixed JSON

**Framing:**
- **stdout**: Protocol frames only
- **stderr**: Logs only (redirected to server logs with plugin ID + name)
- Frame format: 4-byte big-endian length + JSON payload bytes

**Envelope:**

Request:
```json
{ "id": "req-uuid", "method": "plugin.info", "params": {} }
```

Response:
```json
{ "id": "req-uuid", "result": {} }
```

Error:
```json
{ "id": "req-uuid", "error": { "code": "NOT_SUPPORTED", "message": "..." } }
```

### 6.4 Required Methods

**Common:**
- `plugin.info` → `{ plugin_id, plugin_version, api_major, kinds, provides }`
- `plugin.init` → `{ ok: true }`

**Optional (Recommended):**
- `plugin.validate_config` (lets UI test user token quickly)

### 6.5 Source Discovery Method

**Method:** `source.library.list`

**Params:**
```json
{
  "account": {
    "account_id": "acc-uuid",
    "config": { "api_token": "..." }
  }
}
```

**Result:**
```json
{
  "games": [
    {
      "source_game_key": "steam:570",
      "display_name": "Dota 2",
      "provider_ids": { "steam": "app:570" },
      "source_payload": { "free": "json" }
    }
  ]
}
```

**Server Behavior:**
- Upsert `games` (deterministic `game_id`)
- Upsert `game_sources`
- Upsert `user_game_ownership` for scanning user+account
- Record discovery run + job progress

### 6.6 Metadata Method

**Method:** `metadata.fetch`

**Params:**
```json
{
  "game": { "provider_ids": { "steam": "app:570" } },
  "facets": ["core", "achievements", "time_to_beat"]
}
```

**Result:**
```json
{
  "facets": {
    "core": { "any": "json" },
    "achievements": { "any": "json" },
    "time_to_beat": { "any": "json" }
  },
  "ttl_seconds": 86400
}
```

**Server Behavior:**
- Store raw blob in `metadata_blobs`
- Flatten to KV in `metadata_kv`
- Extract known keys (priority from settings) into `metadata_known`
- Update FTS index

### 6.7 Media Method

**Method:** `media.fetch`

**Params:**
```json
{
  "game": { "provider_ids": { "steam": "app:570" } },
  "kinds": ["cover", "screenshot"]
}
```

**Result:**
```json
{
  "assets": [
    {
      "kind": "cover",
      "source_uri": "https://...",
      "mime": "image/jpeg",
      "width": 600,
      "height": 900,
      "sha256": "optional"
    }
  ],
  "ttl_seconds": 604800
}
```

**Server Behavior:**
- Check deduplication by `sha256`
- Download/cache with limits (from settings)
- Store DB with `local_path`
- Serve cached file via `/media/...`

### 6.8 Plugin Supervision

- **Long-lived process** (v1)
- Per-request timeouts (from manifest or default)
- Per-plugin concurrency limit (from manifest)
- Restart on crash with exponential backoff
- Circuit breaker after repeated failures (cooldown)
- **stderr handling**: Redirect to server logs with `plugin_id` and plugin name tags

---

## 7. HTTP API

### 7.1 Auth (Sessions Forever)

**Public:**
- `POST /api/auth/login` (sets session cookie)
  - Body: `{ "username": "...", "password": "..." }`
  - Response: `{ "session_id": "...", "user": {...} }`

**Authenticated:**
- `POST /api/auth/logout` (revokes current session)
- `GET /api/auth/me` (current user info)

**Admin:**
- `GET /api/users`
- `POST /api/users` (create user)
- `POST /api/users/{id}/password` (change password)
- `GET /api/users/{id}/sessions`
- `POST /api/sessions/{id}/revoke`
- `POST /api/users/{id}/sessions/revoke_all`

**User (Self):**
- `GET /api/me/sessions`
- `POST /api/me/sessions/{id}/revoke`

### 7.2 Per-User Connected Accounts

- `GET /api/accounts` (current user's accounts)
- `POST /api/accounts`
  - Body: `{ "plugin_id": "...", "label": "...", "config": { ... } }`
- `POST /api/accounts/{accountId}/validate` (calls `plugin.validate_config` if supported)
- `DELETE /api/accounts/{accountId}`

### 7.3 Discovery

- `POST /api/discovery/run`
  - Body: `{ "account_id": "acc-uuid" }`
  - Server looks up `plugin_id` from account and runs `source.library.list`
- `GET /api/discovery/runs?account_id=...`

### 7.4 Catalog Browsing

- `GET /api/games?query=&limit=&offset=`
- `GET /api/games?mine=true&query=&limit=&offset=`
  - `mine=true` filters by `user_game_ownership` for current user
- `GET /api/games/{gameId}`
- `GET /api/games/{gameId}/sources`
- `GET /api/games/{gameId}/metadata`
  - Returns: known keys, KV view, raw blobs grouped by plugin+facet
- `GET /api/games/{gameId}/media`

**Media Serving:**
- `GET /media/{gameId}/{assetId}` (serves cached file)

### 7.5 Refresh Jobs

- `POST /api/games/{gameId}/refresh`
  - Body: `{ "metadata_facets": ["core","achievements","time_to_beat"], "media_kinds": ["cover","screenshot"] }`
  - Triggers background jobs

### 7.6 Plugin Admin

- `GET /api/plugins`
- `GET /api/plugins/{pluginId}`
- `PUT /api/plugins/{pluginId}/enabled`
- `POST /api/plugins/{pluginId}/reload`
- `POST /api/plugins/install` (upload zip; validate; extract to `/data/plugins`)

### 7.7 Jobs

- `GET /api/jobs?status=&type=`
- `GET /api/jobs/{jobId}`
- `POST /api/jobs/{jobId}/cancel`

**Optional:**
- WebSocket `/ws` to push job progress + plugin health changes

### 7.8 Settings

- `GET /api/settings`
- `PUT /api/settings/{key}` (admin only)
  - Body: `{ "value": ... }`

---

## 8. SSR Web UI

### 8.1 Pages

- `/login` - Login page
- `/` - Catalog (global)
- `/mine` - My Library (filters `mine=true`)
- `/games/{gameId}` - Game details:
  - Media gallery
  - Known keys panel
  - Key/value table view
  - Raw JSON blobs (collapsible per plugin+facet)
  - Buttons: refresh metadata/media
- `/accounts` - Connected accounts (current user):
  - Connect new account (form generated from `account_schema`)
  - Validate token
  - Run discovery
- `/discovery` - Discovery history for user (and admin overview)
- `/jobs` - Jobs list/progress
- `/plugins` - Admin: install/enable/reload
- `/users` - Admin: user management + sessions overview
- `/settings` - Admin: server settings

### 8.2 Implementation

- Go `html/template` SSR
- Minimal JS for:
  - Discovery trigger
  - Refresh buttons
  - Job polling (or WebSocket)
  - Form validation

---

## 9. Caching Strategy

### 9.1 Discovery Cache

- Always stored in DB
- Each discovery run updates:
  - `games`
  - `game_sources`
  - `user_game_ownership`

### 9.2 Metadata Cache

- Stored per plugin+facet with TTL if provided
- When serving metadata:
  - If cached and not expired → serve
  - If expired → allow "refresh" action and/or background refresh

### 9.3 Media Cache

- Stored on disk with DB records
- TTL controls refetch
- Enforce size limits (from settings):
  - Per asset (bytes, -1 = unlimited)
  - Per game per kind (bytes, -1 = unlimited)
  - Total cache cap (bytes, -1 = unlimited)
- Deduplication by `sha256`

---

## 10. Go Project Layout

```
cmd/server/main.go

internal/
  app/              # Wiring (dependency injection)
  config/           # Load + validate config
  http/             # Router, middleware, handlers
  auth/             # Users, sessions, hashing, revoke
  db/               # SQLite, migrations, repos
  domain/           # Entities/value objects
  services/         # Accounts, plugins, discovery, catalog, metadata, media, jobs
  plugins/          # Manifest, process host, framing, supervision
  storage/          # Media cache, downloader, limits
  search/           # FTS indexing
  settings/         # Settings management

web/                # Templates + static
  templates/
  static/

migrations/         # SQL migrations
```

### 10.1 Suggested Libraries

- **Router**: `chi` (or stdlib `net/http`)
- **SQLite**: `modernc.org/sqlite` (pure Go) or `mattn/go-sqlite3`
- **Migrations**: `golang-migrate/migrate`
- **Logging**: `log/slog` (Go 1.21+)
- **JSON Schema**: `github.com/xeipuuv/gojsonschema` (for plugin config validation)
- **Encryption**: `golang.org/x/crypto/chacha20poly1305` or `crypto/aes` (AES-GCM)
- **Password Hashing**: `golang.org/x/crypto/bcrypt` or `golang.org/x/crypto/argon2`

---

## 11. Testing Strategy

### 11.1 Unit Tests

- Deterministic `game_id` computation
- Framing encode/decode
- KV flattener (key paths stable)
- Known-key extractor (priority logic)
- Session create/verify/revoke
- Encryption/decryption (if key provided)

### 11.2 Integration Tests

- Server + temp SQLite
- Test plugin executable fixture:
  - `source.library.list` (returns fixed games)
  - `metadata.fetch` (returns core + achievements + time_to_beat)
  - `media.fetch` (returns URLs served by local `httptest`)
- Test scenarios:
  - Connect account → validate → discovery job → `mine` view shows games
  - Metadata refresh stores blobs + kv + known keys
  - Media fetch caches and serves files
  - Deduplication works (same `sha256` reuses file)

---

## 12. Implementation Milestones

### M1 — Skeleton + DB + SSR Shell + Bootstrap Admin

- Config + logging
- SQLite migrations
- SSR layout
- Create `admin:admin` if DB empty
- Log warning if admin password unchanged

### M2 — Auth (Sessions Forever, Multi-Session)

- Login/logout
- Session middleware
- Revoke session + revoke all sessions
- Admin user management
- Session list (admin + self)

### M3 — Plugin Host

- Scan plugin dir, parse manifests
- Spawn/init/info
- Framing + request/response
- Supervision (timeouts, restarts, circuit breaker)
- stderr redirection to server logs

### M4 — Accounts (Per-User Tokens)

- Account schema UI rendering
- Encrypted storage (if key provided)
- `validate_config` hook
- Connect/disconnect accounts

### M5 — Discovery + Ownership

- Discovery jobs per account
- Upsert games + ownership
- "My library" view
- Deterministic `game_id` computation

### M6 — Metadata (Free Keys + KV + Known Keys + Search)

- Metadata jobs
- Flatten KV
- Known keys extraction (priority from settings)
- FTS search
- Settings API for priority order

### M7 — Media Cache

- Media jobs
- Download/cache/serve
- Deduplication by `sha256`
- Cache limits (from settings)
- UI gallery

### M8 — Admin + Hardening

- Plugin install via zip + enable/disable
- Settings management (cache limits, metadata priority)
- Limits + basic rate limiting
- Clearer error surfaces and audit logs

---

## 13. Default Metadata Facets (v1)

Treat these as facets under `metadata.fetch`:
- `core`
- `achievements`
- `time_to_beat`

New sources later can introduce new facets without changing server architecture.

---

## 14. Settings Defaults (v1)

- `media_cache_limit_per_asset`: -1 (unlimited)
- `media_cache_limit_per_game`: -1 (unlimited)
- `media_cache_limit_total`: -1 (unlimited)
- `metadata_known_keys_priority`: Fixed order (e.g., `["igdb", "steam", "launchbox"]`), but stored in settings for future configurability

---

**End of Design Document**
