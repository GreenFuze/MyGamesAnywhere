# MyGamesAnywhere Architecture

## Overview

MyGamesAnywhere is a **serverless**, cross-platform game launcher and manager with a client-side pluggable architecture. The system consists of rich clients (desktop and mobile) that do ALL work locally, syncing data via the user's own cloud storage (Google Drive, OneDrive, etc.).

**NO SERVER REQUIRED!** 🎉

## High-Level Architecture

```
┌─────────────────────────────────────────────────────────────────┐
│                    Desktop Client (Electron)                     │
│                    Mobile Client (iOS/Android)                   │
│                                                                  │
│  ┌────────────────────────────────────────────────────────────┐ │
│  │  UI Layer (Ionic + React + TypeScript)                     │ │
│  └────────────────────────────────────────────────────────────┘ │
│                                                                  │
│  ┌────────────────────────────────────────────────────────────┐ │
│  │  Plugin System (TypeScript)                                │ │
│  │                                                             │ │
│  │  ┌───────────┐  ┌──────────┐  ┌─────────────────┐        │ │
│  │  │  Steam    │  │  Google  │  │  IGDB Metadata  │        │ │
│  │  │  Scanner  │  │  Drive   │  │  Client         │        │ │
│  │  └───────────┘  └──────────┘  └─────────────────┘        │ │
│  │                                                             │ │
│  │  ┌───────────┐  ┌──────────┐  ┌─────────────────┐        │ │
│  │  │  Native   │  │  Local   │  │  More plugins   │        │ │
│  │  │  Launcher │  │  Folder  │  │  ...            │        │ │
│  │  └───────────┘  └──────────┘  └─────────────────┘        │ │
│  └────────────────────────────────────────────────────────────┘ │
│                                                                  │
│  ┌────────────────────────────────────────────────────────────┐ │
│  │  Cloud Sync Service                                        │ │
│  │  - Google Drive sync                                       │ │
│  │  - OneDrive sync (future)                                 │ │
│  │  - Conflict resolution                                     │ │
│  │  - Last-write-wins merge                                   │ │
│  └──────────────────────┬─────────────────────────────────────┘ │
│                         │                                        │
│  ┌─────────────────────┴──────────────────────────────────────┐ │
│  │  Local Storage (Encrypted via OS Keychain)                │ │
│  │                                                             │ │
│  │  ~/.mygamesanywhere/                                       │ │
│  │  ├── config.json             # User preferences            │ │
│  │  ├── secrets.encrypted       # API keys, OAuth tokens      │ │
│  │  ├── cache.db                # SQLite (metadata, media)    │ │
│  │  └── plugins/                # Installed plugins           │ │
│  └────────────────────────────────────────────────────────────┘ │
└──────────────────┬───────────────────────────────────┬───────────┘
                   │                                   │
         OAuth 2.0 │                         Direct API calls
                   │                                   │
┌──────────────────┴────────────┐   ┌─────────────────┴──────────┐
│  User's Cloud Storage         │   │  External Services         │
│  (Google Drive / OneDrive)    │   │                            │
│                               │   │  - Steam Web API           │
│  .mygamesanywhere/            │   │  - IGDB API               │
│  ├── library.json             │   │  - Other metadata sources  │
│  ├── playtime.json            │   │                            │
│  ├── preferences.json         │   │  (Client connects directly │
│  └── sync-meta.json           │   │   with user's own keys)    │
│                               │   │                            │
│  Syncs across all devices!    │   └────────────────────────────┘
└───────────────────────────────┘

Multiple devices sync automatically via cloud storage!
Desktop ←→ Cloud Storage ←→ Mobile
```

## **CRITICAL: Client-Only Architecture**

### Why Client-Only (No Server)?

**Privacy & Ownership:**
- ✅ ALL user data stays on their devices or their cloud storage
- ✅ No third-party servers storing user's game library
- ✅ User owns their data (in their Google Drive/OneDrive)
- ✅ API keys never leave user's device

**Cost:**
- ✅ Zero hosting costs (no server to pay for!)
- ✅ No infrastructure to maintain
- ✅ Perfect for open-source (no backend required)

**Simplicity:**
- ✅ No server code to write, deploy, or maintain
- ✅ No database migrations or backups
- ✅ Sync handled by mature cloud storage providers
- ✅ Users already have Google Drive/OneDrive

**Reliability:**
- ✅ No single point of failure
- ✅ Cloud storage providers handle availability
- ✅ Works offline (local cache)

### How It Works

**Example: User Adds Steam Library**

1. User clicks "Add Steam Library" in app
2. **Steam plugin runs ON CLIENT** (user's computer)
3. Plugin scans `C:\Program Files\Steam` (local access)
4. Plugin parses Steam library files
5. Plugin extracts game list
6. **Client writes to local cache** (SQLite)
7. **Client syncs to user's cloud storage** (Google Drive `.mygamesanywhere/library.json`)
8. Other devices automatically sync from cloud storage
9. All devices show same library!

**No server involved!**

**Example: User Connects Google Drive**

1. User clicks "Connect Google Drive"
2. **Google Drive plugin runs ON CLIENT**
3. Plugin initiates OAuth flow (browser popup)
4. User authorizes access to their Drive
5. **OAuth token stored on client** (encrypted in OS keychain)
6. Plugin scans user's Drive for game files
7. **Client writes to local cache**
8. **Client syncs to cloud storage** (`.mygamesanywhere/library.json`)
9. When user installs app on another device:
   - User re-authorizes Google Drive on new device (one-time)
   - App syncs library from cloud storage
   - User sees all games from all sources!

**No server! Just client ↔ cloud storage ↔ client**

---

## System Components

### 1. Clients (Desktop & Mobile) - Do Everything

**Purpose:** ALL functionality. Plugins, scanning, launching, API calls, AND cloud sync.

#### Desktop Client

**Technology:**
- **Framework:** Ionic + Capacitor + Electron
- **UI Library:** React 18
- **Language:** TypeScript 5
- **Plugin Runtime:** TypeScript/JavaScript
- **Styling:** TailwindCSS
- **State Management:** Zustand

**Platforms:**
- Windows (x64, ARM64)
- macOS (Intel, Apple Silicon)
- Linux (AppImage, .deb, .rpm)

**Key Features:**
- Multi-column layouts optimized for mouse/keyboard
- Plugin execution environment
- Local encrypted storage
- File system access
- Process management (launch games)
- System tray integration
- Background operations

#### Mobile Client

**Technology:**
- **Framework:** Ionic + Capacitor
- **UI Library:** React 18 (same as desktop)
- **Language:** TypeScript 5
- **Plugin Runtime:** TypeScript/JavaScript (same as desktop!)
- **Styling:** TailwindCSS (same design system)
- **State Management:** Zustand (shared)

**Platforms:**
- iOS 13+
- Android 8.0+

**Key Features:**
- Touch-optimized single-column layouts
- Same plugin system as desktop (cross-platform!)
- Platform-specific UI patterns (Material/Cupertino)
- Push notifications
- Biometric authentication
- Quick launch widgets

#### Client Responsibilities (Both Desktop & Mobile)

**Plugin Management:**
- Load and execute plugins
- Plugin lifecycle (install, enable, disable, update)
- Plugin isolation and error handling
- Plugin configuration UI

**Data Operations:**
- Scan local sources (Steam, folders)
- Connect to user's cloud storage (Google Drive, OneDrive)
- Fetch metadata from external APIs
- Download media (covers, screenshots)
- Cache data locally (SQLite)
- Sync library to user's cloud storage (`.mygamesanywhere/` folder)

**Game Management:**
- Install/uninstall games
- Launch games
- Monitor running games
- Track playtime

**Security:**
- Store API keys encrypted
- Manage OAuth tokens
- Use OS keychains where available

**Cloud Sync:**
- Sync library to user's cloud storage
- Conflict resolution (last-write-wins with timestamp)
- Background sync on changes
- Multi-device support

---

### 2. Cloud Storage (User's Own Account)

**Purpose:** Cross-device sync storage (user owns their data!)

**Supported Providers:**
- ✅ Google Drive (Phase 1)
- ✅ OneDrive (Phase 2)
- ✅ Dropbox (Future)

**Data Structure:**
```
User's Cloud Storage Root
└── .mygamesanywhere/
    ├── library.json          # Game library (all sources)
    ├── playtime.json         # Playtime tracking
    ├── preferences.json      # User preferences
    └── sync-meta.json        # Sync metadata (timestamps, device IDs)
```

**File Formats:**

`library.json`:
```json
{
  "version": "1.0",
  "lastModified": "2024-10-03T10:30:00Z",
  "deviceId": "desktop-abc123",
  "games": [
    {
      "id": "steam-12345",
      "title": "Half-Life 2",
      "source": "steam",
      "platform": "windows",
      "metadata": { ... }
    }
  ]
}
```

**Conflict Resolution:**
- Last-write-wins based on timestamp
- Client merges local and cloud data
- User notified of conflicts if timestamps are very close

**Access:**
- Client uses OAuth 2.0 to access user's cloud storage
- OAuth token stored in OS keychain (encrypted)
- Client reads/writes JSON files directly

---

### 3. Plugin System (Client-Side TypeScript)

**Architecture:** Native TypeScript modules (NOT separate processes on client)

**Why TypeScript, not Go plugins?**
- ✅ **Cross-platform:** Works on desktop AND mobile
- ✅ **Same language:** TypeScript everywhere (UI + plugins)
- ✅ **Easier development:** No compilation, hot reload
- ✅ **Mobile compatible:** Can't run Go plugins on iOS/Android
- ✅ **Sandbox-able:** Can use Web Workers for isolation
- ✅ **NPM ecosystem:** Leverage existing packages

**Plugin Structure:**

```typescript
// Plugin interface
interface Plugin {
  id: string;
  name: string;
  version: string;
  capabilities: PluginCapabilities;

  // Lifecycle
  initialize(config: PluginConfig): Promise<void>;
  destroy(): Promise<void>;

  // Capability implementations (optional based on capabilities)
  scanInstalled?(config: ScanConfig): Promise<Game[]>;
  scanAvailable?(config: ScanConfig): Promise<Game[]>;
  download?(game: Game, destination: string): Promise<void>;
  searchMetadata?(query: string): Promise<Metadata[]>;
  fetchMetadata?(gameId: string): Promise<Metadata>;
  fetchMedia?(gameId: string, type: MediaType): Promise<string>;
  launch?(game: Game, options: LaunchOptions): Promise<Process>;
  monitor?(processId: string): Promise<ProcessStatus>;
}
```

**Plugin Types (by capability):**

1. **Source Plugins** - Where games come from
   - Steam (scan installed + metadata + launch)
   - Epic Games Store
   - GOG Galaxy
   - **Generic Repository Scanner** (local/cloud directories with mixed game formats)
     - Installers (.exe, .msi, .pkg, .deb, .rpm)
     - Portable games (ready-to-run directories)
     - ROMs (NES, SNES, PlayStation, etc.)
     - Archives (single & multi-part: .zip, .rar, .7z, .part1, .z01)
     - Emulator-required games (DOSBox, ScummVM)
     - Cross-platform detection (Windows/Linux/macOS/Android/iOS)
   - Google Drive (scan + download)
   - OneDrive
   - Network shares

2. **Metadata Plugins** - Game information
   - IGDB (primary)
   - LaunchBox Games DB
   - SteamGridDB
   - ScreenScraper
   - MobyGames

3. **Launcher Plugins** - How to execute
   - Native launcher (Windows .exe, macOS .app, Linux)
   - Steam launcher (steam://)
   - RetroArch (emulation)
   - Wine/Proton (Windows games on Linux)
   - Web/JS emulators

4. **Feature Plugins** - Extra functionality
   - Cloud save sync
   - Achievement tracking
   - Screenshot capture
   - Controller mapping

**Plugin Discovery:**
```
~/.mygamesanywhere/
├── plugins/                    # Installed plugins
│   ├── steam/
│   │   ├── plugin.json        # Manifest
│   │   ├── index.ts           # Main entry point
│   │   └── scanner.ts, metadata.ts, launcher.ts
│   ├── gdrive/
│   │   ├── plugin.json
│   │   ├── index.ts
│   │   └── auth.ts, scanner.ts, downloader.ts
│   └── igdb/
│       ├── plugin.json
│       ├── index.ts
│       └── client.ts, matcher.ts
```

**Plugin Manifest (plugin.json):**
```json
{
  "id": "steam",
  "name": "Steam",
  "version": "1.0.0",
  "author": "MyGamesAnywhere Team",
  "description": "Integration with Steam library",
  "capabilities": {
    "canScanInstalled": true,
    "canScanAvailable": false,
    "canDownload": false,
    "canSearchMetadata": true,
    "canFetchMetadata": true,
    "canFetchMedia": true,
    "canLaunch": true,
    "canMonitor": true,
    "requiresAuth": true
  },
  "config_schema": {
    "api_key": {
      "type": "string",
      "description": "Steam Web API key (optional)",
      "optional": true
    },
    "steam_path": {
      "type": "string",
      "description": "Path to Steam installation",
      "default": "C:\\Program Files (x86)\\Steam"
    }
  }
}
```

**Plugin Execution:**

```typescript
// Client loads plugin
const plugin = await pluginManager.load('steam');

// Client configures plugin
await plugin.initialize({
  api_key: userConfig.steamApiKey,  // From encrypted local storage
  steam_path: 'C:\\Program Files (x86)\\Steam'
});

// Client calls plugin capability
const games = await plugin.scanInstalled({
  // Scan config
});

// Client syncs to cloud storage
await cloudSync.syncLibrary(games);
```

**Error Handling:**
- Plugin errors caught by client
- User notified of errors
- Plugin can be disabled if repeatedly failing
- No server involvement

---

## Data Flow

### Adding Steam Library (Complete Flow)

```
1. User clicks "Add Steam Library"
   ↓
2. Client UI → Plugin Manager → Steam Plugin (ALL ON CLIENT)
   ↓
3. Steam Plugin scans C:\Program Files\Steam (local access)
   ↓
4. Plugin parses libraryfolders.vdf
   ↓
5. Plugin reads .acf manifests for each game
   ↓
6. Plugin extracts: title, appid, install dir, size
   ↓
7. Plugin returns Game[] array to client
   ↓
8. Client stores in local cache (SQLite)
   ↓
9. Client renders games in UI
   ↓
10. Client writes to cloud storage (Google Drive `.mygamesanywhere/library.json`)
    ↓
11. Cloud storage provider syncs file to cloud
    ↓
12. Other devices detect cloud file change (polling or webhooks)
    ↓
13. Other devices download updated library.json
    ↓
14. Other devices merge with local library
    ↓
15. Other devices render games (but can't launch - games not installed there)
```

**No server! Just client ↔ cloud storage ↔ client**

### Fetching Game Metadata

```
1. User views game details
   ↓
2. Client checks local cache
   ├─→ If cached: display immediately
   └─→ If not cached:
       ↓
3. Client → IGDB Plugin (ON CLIENT)
   ↓
4. Plugin calls IGDB API (user's API key from encrypted local storage)
   ↓
5. Plugin returns metadata
   ↓
6. Client caches locally (SQLite)
   ↓
7. Client renders metadata
   ↓
8. (Optional) Client can cache metadata in cloud storage for other devices
```

**All metadata stays on client or client's cloud storage!**

### Launching Game

```
1. User clicks "Play" button
   ↓
2. Client → Steam Plugin → launch() (ALL ON CLIENT)
   ↓
3. Plugin executes: steam://rungameid/12345
   ↓
4. Steam client launches game
   ↓
5. Plugin monitors game process
   ↓
6. Client tracks playtime locally (SQLite)
   ↓
7. When game exits, client syncs playtime to cloud storage (playtime.json)
```

**All playtime tracking local or synced via cloud storage!**

---

## Configuration Storage

### Client Configuration (Encrypted)

**Location:**
- Windows: `%APPDATA%\MyGamesAnywhere\`
- macOS: `~/Library/Application Support/MyGamesAnywhere/`
- Linux: `~/.config/mygamesanywhere/`

**Files:**
```
MyGamesAnywhere/
├── config.json              # User preferences
├── secrets.encrypted        # API keys, OAuth tokens (encrypted)
├── cache.db                 # SQLite cache (metadata, media)
└── plugins/                 # Installed plugins
    ├── steam/
    ├── gdrive/
    └── igdb/
```

**Encryption:**
- **Best:** OS keychain
  - Windows: DPAPI (Data Protection API)
  - macOS: Keychain Access
  - Linux: libsecret (Secret Service API)
- **Fallback:** AES-256 with key derived from user password

**secrets.encrypted (decrypted format):**
```json
{
  "plugins": {
    "steam": {
      "api_key": "ABC123..."
    },
    "gdrive": {
      "oauth_token": "ya29.a0...",
      "refresh_token": "1//...",
      "expires_at": "2024-10-03T12:00:00Z"
    },
    "igdb": {
      "client_id": "...",
      "client_secret": "..."
    }
  }
}
```

**Optional: Sync Encrypted Config to Cloud Storage**
- User can opt-in to sync encrypted config to their cloud storage
- Enables easy multi-device setup
- Encrypted blob stored in `.mygamesanywhere/secrets.encrypted`
- User enters password on new device to decrypt
- Cloud storage provider can't decrypt (AES-256)

---

## Deployment (Client-Only!)

**NO SERVER TO DEPLOY!** 🎉

### Client Installation

**Desktop:**
- Download installer from GitHub releases
- Run installer
- Configure cloud storage provider (Google Drive, OneDrive)
- Add game sources (Steam, local folders, etc.)
- Done!

**Mobile:**
- Download from App Store / Play Store
- Log in with same cloud storage account
- Library automatically syncs from cloud
- Done!

**Cloud Storage Setup:**
1. User authorizes app to access their cloud storage (OAuth 2.0)
2. App creates `.mygamesanywhere/` folder in user's cloud storage
3. App writes library, playtime, preferences as JSON files
4. Cloud storage provider handles sync automatically
5. Other devices read from same folder

**System Requirements:**
- Desktop: Windows 10+, macOS 11+, or Linux (Ubuntu 20.04+)
- Mobile: iOS 13+ or Android 8.0+
- Cloud Storage: Google Drive account (free tier sufficient)

---

## Platform-Specific Considerations

### Windows
- DPAPI for secrets
- Steam typically: `C:\Program Files (x86)\Steam`
- Launch .exe files
- System tray support

### macOS
- Keychain for secrets
- Steam typically: `~/Library/Application Support/Steam`
- Launch .app bundles
- Menu bar integration

### Linux
- libsecret for secrets
- Steam typically: `~/.steam/steam`
- Launch executables, AppImages
- System tray (AppIndicator)

### iOS
- Keychain for secrets
- Limited file system access (sandboxed)
- No game launching (can browse/manage only)

### Android
- Keystore for secrets
- Storage Access Framework
- Limited game launching (Android games only)

---

## Security Model

### Client Security
- API keys encrypted at rest (OS keychain)
- OAuth tokens in OS keychain
- Sensitive operations require user confirmation
- Plugin code review (future: signature verification)
- Encrypted config optionally synced to user's cloud storage

### Cloud Storage Security
- User's cloud storage accessed via OAuth 2.0
- OAuth tokens encrypted in OS keychain
- Cloud storage provider handles HTTPS/TLS
- Data in cloud storage is JSON (readable by user if needed)
- Optional: Encrypt sensitive data before uploading

### Privacy
- **NO THIRD-PARTY SERVERS!**
- User data stays on their devices or their own cloud storage
- User owns all their data (can export, delete anytime)
- No telemetry without opt-in
- No tracking, no analytics (unless user enables)

---

## Summary

**Key Architectural Decisions:**

1. **✅ NO SERVER!** (zero hosting costs, ultimate privacy)
2. **✅ Cloud storage sync** (user owns their data)
3. **✅ Client-side plugins** (TypeScript, cross-platform)
4. **✅ Client does everything** (scanning, API calls, launching, syncing)
5. **✅ Open-source friendly** (no infrastructure required)

**This architecture:**
- ✅ **Zero hosting costs** (no server!)
- ✅ **Ultimate privacy** (user owns all data)
- ✅ **Scales naturally** (distributed compute)
- ✅ **Simple deployment** (just install client)
- ✅ **Offline functionality** (local cache)
- ✅ **Works on mobile AND desktop**
- ✅ **Perfect for open-source** (no backend complexity)

See [PLUGINS.md](./PLUGINS.md) for detailed plugin specifications and [PHASE1_DETAILED.md](./PHASE1_DETAILED.md) for implementation plan.
