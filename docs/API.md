# Server API Specification

The MyGamesAnywhere server exposes a REST API for client communication.

## Base URL

- **Cloud:** `https://api.mygamesanywhere.io/v1`
- **Local:** `http://localhost:8080/v1`

## Authentication

All endpoints (except `/auth/*`) require JWT authentication.

### Headers

```http
Authorization: Bearer <jwt_token>
Content-Type: application/json
```

### Authentication Flow

```
1. Client requests token
   POST /auth/login
   { "email": "user@example.com", "password": "..." }

2. Server returns JWT
   { "token": "eyJ...", "refresh_token": "...", "expires_in": 3600 }

3. Client includes token in subsequent requests
   Authorization: Bearer eyJ...

4. Token expires, client refreshes
   POST /auth/refresh
   { "refresh_token": "..." }
```

---

## Error Format

All errors follow this format:

```json
{
  "error": {
    "code": "ERROR_CODE",
    "message": "Human-readable message",
    "details": {
      "field": "Additional context"
    },
    "timestamp": "2024-10-02T12:00:00Z"
  }
}
```

**HTTP Status Codes:**
- `200` - Success
- `201` - Created
- `400` - Bad Request (validation error)
- `401` - Unauthorized (missing/invalid token)
- `403` - Forbidden (insufficient permissions)
- `404` - Not Found
- `409` - Conflict (duplicate resource)
- `500` - Internal Server Error
- `503` - Service Unavailable (maintenance)

---

## Endpoints

### Authentication

#### POST /auth/register

Register new user.

**Request:**
```json
{
  "email": "user@example.com",
  "username": "johndoe",
  "password": "SecurePassword123!"
}
```

**Response:** `201 Created`
```json
{
  "user": {
    "id": "uuid",
    "email": "user@example.com",
    "username": "johndoe",
    "created_at": "2024-10-02T12:00:00Z"
  },
  "token": "eyJ...",
  "refresh_token": "...",
  "expires_in": 3600
}
```

#### POST /auth/login

Authenticate user.

**Request:**
```json
{
  "email": "user@example.com",
  "password": "SecurePassword123!"
}
```

**Response:** `200 OK`
```json
{
  "token": "eyJ...",
  "refresh_token": "...",
  "expires_in": 3600
}
```

#### POST /auth/refresh

Refresh expired token.

**Request:**
```json
{
  "refresh_token": "..."
}
```

**Response:** `200 OK`
```json
{
  "token": "eyJ...",
  "refresh_token": "...",
  "expires_in": 3600
}
```

---

### Users

#### GET /users/me

Get current user profile.

**Response:** `200 OK`
```json
{
  "id": "uuid",
  "email": "user@example.com",
  "username": "johndoe",
  "created_at": "2024-10-02T12:00:00Z",
  "settings": {
    "theme": "dark",
    "language": "en"
  }
}
```

#### PATCH /users/me

Update user profile.

**Request:**
```json
{
  "username": "newusername",
  "settings": {
    "theme": "light"
  }
}
```

**Response:** `200 OK`

---

### Libraries

#### GET /libraries

List user's libraries.

**Response:** `200 OK`
```json
{
  "libraries": [
    {
      "id": "uuid",
      "name": "My Games",
      "description": "Main library",
      "is_default": true,
      "game_count": 42,
      "created_at": "2024-10-02T12:00:00Z"
    }
  ]
}
```

#### POST /libraries

Create new library.

**Request:**
```json
{
  "name": "Favorites",
  "description": "My favorite games",
  "icon": "star"
}
```

**Response:** `201 Created`

#### GET /libraries/:id

Get library details with games.

**Query Parameters:**
- `sort` - Sort by: `title`, `added_at`, `last_played`, `play_count`
- `order` - Order: `asc`, `desc`
- `filter` - Filter by: `installed`, `favorite`, `platform:windows`
- `limit` - Results per page (default: 50)
- `offset` - Pagination offset (default: 0)

**Response:** `200 OK`
```json
{
  "library": {
    "id": "uuid",
    "name": "My Games",
    "game_count": 42
  },
  "games": [
    {
      "id": "uuid",
      "title": "Portal 2",
      "slug": "portal-2",
      "platform": "windows",
      "execution_type": "native",
      "cover_url": "https://...",
      "is_favorite": true,
      "play_count": 15,
      "total_play_time": 36000,
      "last_played": "2024-10-01T20:00:00Z",
      "installation": {
        "status": "installed",
        "path": "C:\\Games\\Portal2",
        "size_on_disk": 8589934592
      }
    }
  ],
  "pagination": {
    "total": 42,
    "limit": 50,
    "offset": 0
  }
}
```

#### POST /libraries/:id/games

Add game to library.

**Request:**
```json
{
  "game_platform_id": "uuid"
}
```

**Response:** `201 Created`

#### DELETE /libraries/:id/games/:game_id

Remove game from library.

**Response:** `204 No Content`

---

### Repositories

#### GET /repositories

List user's game sources.

**Response:** `200 OK`
```json
{
  "repositories": [
    {
      "id": "uuid",
      "type": "steam",
      "name": "Steam Library",
      "enabled": true,
      "auto_scan": true,
      "last_scan": "2024-10-02T10:00:00Z",
      "scan_status": "idle",
      "game_count": 120
    }
  ]
}
```

#### POST /repositories

Create new repository.

**Request:**
```json
{
  "type": "local_folder",
  "name": "My Game Folder",
  "config": {
    "path": "D:\\Games",
    "recursive": true,
    "file_patterns": ["*.exe"]
  },
  "enabled": true,
  "auto_scan": true
}
```

**Response:** `201 Created`

#### PATCH /repositories/:id

Update repository configuration.

**Request:**
```json
{
  "enabled": false,
  "config": {
    "path": "E:\\Games"
  }
}
```

**Response:** `200 OK`

#### POST /repositories/:id/scan

Trigger repository scan.

**Response:** `202 Accepted`
```json
{
  "scan_id": "uuid",
  "status": "scanning",
  "started_at": "2024-10-02T12:00:00Z"
}
```

#### GET /repositories/:id/scan/:scan_id

Get scan status.

**Response:** `200 OK`
```json
{
  "scan_id": "uuid",
  "status": "completed",
  "started_at": "2024-10-02T12:00:00Z",
  "completed_at": "2024-10-02T12:05:00Z",
  "games_found": 42,
  "games_new": 5,
  "errors": []
}
```

---

### Games

#### GET /games/search

Search for games.

**Query Parameters:**
- `q` - Search query
- `platform` - Filter by platform
- `limit` - Results limit (default: 20)

**Response:** `200 OK`
```json
{
  "games": [
    {
      "id": "uuid",
      "title": "Portal 2",
      "slug": "portal-2",
      "platforms": ["windows", "mac", "linux"],
      "cover_url": "https://...",
      "release_date": "2011-04-19"
    }
  ],
  "total": 1
}
```

#### GET /games/:id

Get game details.

**Response:** `200 OK`
```json
{
  "game": {
    "id": "uuid",
    "title": "Portal 2",
    "slug": "portal-2",
    "release_date": "2011-04-19",
    "developer": "Valve",
    "publisher": "Valve",
    "description": "...",
    "platforms": [
      {
        "id": "uuid",
        "platform": "windows",
        "execution_type": "native",
        "media_urls": {
          "cover": "https://...",
          "screenshots": ["https://..."],
          "videos": ["https://..."]
        },
        "sources": [
          {
            "repository_id": "uuid",
            "repository_name": "Steam",
            "source_identifier": "620",
            "file_size": 8589934592
          }
        ]
      }
    ]
  }
}
```

---

### Installations

#### POST /installations

Install a game.

**Request:**
```json
{
  "game_platform_id": "uuid",
  "repository_id": "uuid",
  "install_path": "C:\\Games\\Portal2",
  "platform_plugin": "native"
}
```

**Response:** `202 Accepted`
```json
{
  "installation_id": "uuid",
  "status": "downloading",
  "progress": 0
}
```

#### GET /installations/:id

Get installation status.

**Response:** `200 OK`
```json
{
  "installation_id": "uuid",
  "game_platform_id": "uuid",
  "status": "downloading",
  "progress": 45,
  "install_path": "C:\\Games\\Portal2",
  "downloaded_bytes": 3865470566,
  "total_bytes": 8589934592,
  "download_speed": 10485760,
  "eta_seconds": 450
}
```

#### DELETE /installations/:id

Uninstall a game.

**Response:** `202 Accepted`
```json
{
  "status": "uninstalling"
}
```

#### POST /installations/:id/launch

Launch installed game.

**Request:**
```json
{
  "options": {
    "fullscreen": true,
    "resolution": "1920x1080"
  }
}
```

**Response:** `200 OK`
```json
{
  "process_id": "uuid",
  "started_at": "2024-10-02T12:00:00Z"
}
```

---

### Plugins

#### GET /plugins

List all plugins.

**Response:** `200 OK`
```json
{
  "plugins": [
    {
      "id": "source-steam",
      "name": "Steam Source",
      "version": "1.0.0",
      "type": "source",
      "enabled": true,
      "health_status": "healthy",
      "config": {
        "steam_path": "C:\\Program Files (x86)\\Steam"
      }
    }
  ]
}
```

#### POST /plugins/:id/enable

Enable plugin.

**Response:** `200 OK`

#### POST /plugins/:id/disable

Disable plugin.

**Response:** `200 OK`

#### PATCH /plugins/:id/config

Update plugin configuration.

**Request:**
```json
{
  "config": {
    "steam_path": "D:\\Steam"
  }
}
```

**Response:** `200 OK`

---

### Metadata

#### POST /metadata/refresh

Refresh metadata for a game.

**Request:**
```json
{
  "game_id": "uuid",
  "sources": ["igdb", "launchbox"]
}
```

**Response:** `202 Accepted`
```json
{
  "refresh_id": "uuid",
  "status": "fetching"
}
```

---

## WebSocket Events

Real-time updates are sent via WebSocket at `/ws`.

### Connection

```javascript
const ws = new WebSocket('wss://api.mygamesanywhere.io/ws');
ws.addEventListener('open', () => {
  // Send authentication
  ws.send(JSON.stringify({
    type: 'auth',
    token: 'eyJ...'
  }));
});
```

### Event Format

```json
{
  "type": "event_type",
  "payload": {
    "data": "..."
  },
  "timestamp": "2024-10-02T12:00:00Z"
}
```

### Event Types

#### installation.progress

Download/installation progress.

```json
{
  "type": "installation.progress",
  "payload": {
    "installation_id": "uuid",
    "status": "downloading",
    "progress": 45,
    "download_speed": 10485760
  }
}
```

#### installation.completed

Installation finished.

```json
{
  "type": "installation.completed",
  "payload": {
    "installation_id": "uuid",
    "game_title": "Portal 2"
  }
}
```

#### library.updated

Library changed (game added/removed).

```json
{
  "type": "library.updated",
  "payload": {
    "library_id": "uuid",
    "action": "game_added",
    "game_id": "uuid"
  }
}
```

#### scan.completed

Repository scan finished.

```json
{
  "type": "scan.completed",
  "payload": {
    "repository_id": "uuid",
    "games_found": 42,
    "games_new": 5
  }
}
```

---

## Rate Limiting

- **Default:** 100 requests per minute per user
- **Auth endpoints:** 10 requests per minute per IP
- **Header:** `X-RateLimit-Remaining: 95`

---

## Versioning

API version is in the URL: `/v1/...`

Breaking changes will increment the version: `/v2/...`

---

## Summary

The API provides:

✅ **RESTful design** - Standard HTTP methods
✅ **JWT authentication** - Secure token-based auth
✅ **Detailed errors** - Clear error codes and messages
✅ **WebSocket events** - Real-time updates
✅ **Pagination** - For large result sets
✅ **Filtering & sorting** - Flexible queries
✅ **Rate limiting** - Prevent abuse

See [ARCHITECTURE.md](./ARCHITECTURE.md) for system design and [DATABASE_SCHEMA.md](./DATABASE_SCHEMA.md) for data models.
