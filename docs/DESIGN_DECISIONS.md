# Design Decisions

This document explains the rationale behind key architectural and technology decisions for MyGamesAnywhere.

## Table of Contents

1. [Technology Stack](#technology-stack)
2. [Architecture Patterns](#architecture-patterns)
3. [Code Organization](#code-organization)
4. [Development Practices](#development-practices)
5. [Deployment Strategy](#deployment-strategy)

---

## Technology Stack

### Frontend: Ionic + Capacitor + React

**Decision:** Use Ionic Framework with Capacitor for both desktop and mobile clients.

**Alternatives Considered:**
- ❌ **Electron alone** - Desktop only, no mobile support
- ❌ **Flutter** - Dart language (not preferred), different paradigm
- ❌ **React Native** - Weak desktop support
- ❌ **Neutralinojs** - No mobile support
- ❌ **.NET MAUI** - Requires .NET runtime
- ❌ **Tauri** - Uses Rust (not preferred)

**Why Ionic + Capacitor?**
- ✅ **True cross-platform:** iOS, Android, Windows, macOS, Linux from one codebase
- ✅ **Web technologies:** JavaScript/TypeScript + React (familiar to most developers)
- ✅ **Native performance:** Uses platform WebView (not embedded Chromium on mobile)
- ✅ **Lightweight on mobile:** ~5-10MB base size
- ✅ **Electron integration:** Via @capacitor-community/electron for desktop
- ✅ **Maximum code reuse:** 60-80% shared between desktop and mobile
- ✅ **Modern UI:** React + TailwindCSS for slick, responsive interfaces
- ✅ **Active development:** Regular updates, strong community
- ✅ **Plugin ecosystem:** Access to native features (filesystem, notifications, etc.)

**Trade-offs:**
- ⚠️ Desktop app uses Electron (~80-100MB) - acceptable for gaming app
- ⚠️ Requires Node.js/npm ecosystem knowledge
- ⚠️ Performance not as good as true native, but sufficient for UI work

**What is Capacitor?**
Capacitor is a native runtime that lets you build one web app and deploy to mobile (iOS/Android), desktop (via Electron), and web. It provides JavaScript APIs to access native platform features while keeping your UI in a WebView.

---

### Backend: None! (Serverless Architecture)

**Decision:** NO BACKEND SERVER! Client-only architecture with cloud storage sync.

**Why No Backend?**
- ✅ **Zero hosting costs** - No server to pay for!
- ✅ **Zero maintenance** - No deployments, monitoring, or scaling
- ✅ **Perfect for open-source** - No infrastructure barrier for contributors
- ✅ **Ultimate privacy** - User data stays on their devices/cloud storage
- ✅ **Simpler architecture** - One less entire codebase to maintain
- ✅ **No single point of failure** - Distributed by design

**What Replaces the Backend?**
- **Sync:** User's cloud storage (Google Drive API, OneDrive API)
- **Storage:** Local SQLite cache + JSON files in cloud storage
- **Auth:** OAuth 2.0 directly to cloud storage providers
- **Real-time updates:** Cloud storage change detection (polling or webhooks)

**Originally Considered Go for Backend:**
Go was initially chosen for its efficiency and low resource usage, perfect for a lightweight sync server. However, we realized cloud storage providers already offer everything we need:
- Sync across devices (Google Drive/OneDrive sync)
- Storage (user's cloud storage quota)
- Availability (99.9%+ SLA from providers)
- Backups (automatic by providers)

By using cloud storage, we get all these benefits without writing/maintaining server code!

---

### Plugin System: Client-Side TypeScript Modules

**Decision:** Plugins run on the CLIENT as TypeScript modules, not on the server.

**Alternatives Considered:**
- ❌ **Server-side plugins (hashicorp/go-plugin)** - Privacy concerns, functionality limitations, rate limits
- ❌ **Native Go plugins** - Platform limitations, client needs access to local files
- ❌ **Dynamic libraries** (.dll/.so) - Platform-specific, security issues
- ❌ **Embedded interpreters** (Lua, JavaScript in sandbox) - Limited functionality

**Why Client-Side TypeScript Plugins?**

**Privacy & Security:**
- ✅ **User data stays local:** Steam library paths, local game files stay on user's device
- ✅ **Personal credentials:** OAuth tokens (Google Drive, etc.) stored only on user's device
- ✅ **API keys:** User's IGDB/Steam API keys encrypted on their device
- ✅ **Privacy by design:** No third-party servers see user's local filesystem or credentials
- ✅ **User owns data:** Library synced to user's own cloud storage (Google Drive/OneDrive)

**Functionality:**
- ✅ **Local file access:** Plugins can scan user's Steam installation directory
- ✅ **Game launching:** Execute games on user's machine
- ✅ **Native OS integration:** Access OS keychains, filesystem, process management
- ✅ **Personal OAuth flows:** Each user authenticates with their own Google/OneDrive account

**Performance & Cost:**
- ✅ **Distributed compute:** All work (scanning, downloading) happens on user's device
- ✅ **Independent API limits:** Each user has their own IGDB/Steam API rate limits
- ✅ **Zero hosting costs:** No server to pay for!
- ✅ **Direct downloads:** Client downloads game files directly from source

**Cross-Platform:**
- ✅ **TypeScript everywhere:** Works on desktop (Electron) and mobile (Capacitor)
- ✅ **No compilation needed:** JavaScript runs on all platforms
- ✅ **Single plugin codebase:** One plugin works on Windows, macOS, Linux, iOS, Android
- ✅ **Access to Capacitor APIs:** Filesystem, native features, OS integration

**Capability-Based Design:**
Plugins declare capabilities instead of strict types:
```typescript
interface PluginCapabilities {
  // Source capabilities
  canScanInstalled?: boolean;    // Scan installed games
  canScanAvailable?: boolean;    // Scan available games
  canDownload?: boolean;         // Download games

  // Metadata capabilities
  canSearchMetadata?: boolean;   // Search for games
  canFetchMetadata?: boolean;    // Fetch detailed metadata

  // Media capabilities
  canFetchMedia?: boolean;       // Fetch covers, screenshots, videos

  // Launcher capabilities
  canLaunch?: boolean;           // Launch games
  canMonitor?: boolean;          // Monitor running games
}
```

**Example: Steam Plugin**
- Capabilities: `canScanInstalled`, `canSearchMetadata`, `canFetchMedia`, `canLaunch`
- Scans: `C:\Program Files\Steam\steamapps\` (local access required!)
- Launches: `steam://rungameid/...` (must run on user's machine)
- API calls: Uses user's Steam Web API key (stored locally encrypted)

**How it works:**
1. Client loads plugin as TypeScript module
2. Plugin initializes with encrypted config from local storage
3. Plugin makes API calls with user's personal credentials
4. Plugin accesses user's filesystem directly
5. Results cached locally (SQLite)
6. Library synced to user's cloud storage (Google Drive `.mygamesanywhere/library.json`)

**Configuration Storage:**
```
~/.mygamesanywhere/
├── config.json              # User preferences
├── secrets.encrypted        # API keys, OAuth tokens (encrypted via OS keychain)
├── cache.db                 # SQLite cache
└── plugins/                 # Installed plugins
    ├── steam/
    ├── gdrive/
    └── igdb/
```

**Error Handling:**
- Try/catch around all plugin operations
- Detailed error reporting (fail-fast)
- Circuit breaker for repeatedly failing plugins
- User notifications for plugin failures
- Graceful degradation (app works without failed plugin)

**Trade-offs:**
- ⚠️ Plugins limited to TypeScript/JavaScript (can't use Go, Rust, etc.)
- ⚠️ Client-side compute burden (acceptable for desktop, careful on mobile)
- ⚠️ Plugin distribution more complex (need update mechanism)

---

### Storage: SQLite (Local Cache) + JSON (Cloud Sync)

**Decision:** Use SQLite for local caching and JSON files in cloud storage for sync. No server database!

**Why SQLite for Local Cache?**
- ✅ **Embedded:** No separate server process
- ✅ **Zero configuration:** Just a file
- ✅ **Lightweight:** ~600KB library
- ✅ **Fast:** Excellent read performance for UI
- ✅ **Cross-platform:** Works everywhere (desktop + mobile)
- ✅ **File-based:** Easy backups (just copy the file)
- ✅ **ACID:** Data integrity guarantees
- ✅ **Rich queries:** SQL for complex filters, sorting
- ✅ **Full-text search:** Built-in FTS for searching games

**Why JSON for Cloud Sync?**
- ✅ **Simple:** Human-readable, easy to debug
- ✅ **Versionable:** Easy to version schema (just `"version": "1.0"` field)
- ✅ **User-friendly:** Users can inspect/export their own data
- ✅ **Portable:** Works with any cloud storage provider
- ✅ **No migrations:** Just read JSON, merge changes, write back
- ✅ **Git-friendly:** Users could version control their library if desired
- ✅ **Lightweight:** Typical library.json is <1MB even for 1000+ games

**Data Flow:**
1. **Local:** Client reads/writes SQLite cache (fast)
2. **Sync:** Client periodically syncs to cloud storage (JSON files)
3. **Merge:** On startup, client merges cloud JSON with local SQLite
4. **Conflict:** Last-write-wins based on timestamp

**No Server Database Needed!**
- Originally considered PostgreSQL for cloud server
- Realized cloud storage providers handle storage/sync better
- JSON in cloud storage is simpler and more user-friendly than SQL backups

---

## Architecture Patterns

### Cloud Storage Sync (No Server!)

**Decision:** NO SERVER! Clients sync via user's own cloud storage (Google Drive, OneDrive, etc.).

**Alternatives Considered:**
- ❌ **Server-based sync** - Hosting costs, privacy concerns, infrastructure complexity
- ❌ **Standalone client only** - No sync, no multi-device support
- ❌ **Peer-to-peer** - Complex NAT traversal, no central authority

**Why Cloud Storage Instead of Server?**

**Cost:**
- ✅ **Zero hosting costs** - No server to pay for or maintain!
- ✅ **No infrastructure** - No DevOps, no deployments, no monitoring
- ✅ **Scales for free** - Cloud storage providers handle scaling
- ✅ **Perfect for open-source** - Contributors don't need to run infrastructure

**Privacy:**
- ✅ **User owns their data** - Library stored in user's own Google Drive/OneDrive
- ✅ **No third-party servers** - User data never touches our servers (because we have none!)
- ✅ **Can export anytime** - User can download their library as JSON files
- ✅ **Can delete anytime** - User deletes `.mygamesanywhere/` folder from their cloud

**Simplicity:**
- ✅ **No backend code** - One less entire codebase to maintain!
- ✅ **No database migrations** - Just JSON files, easily versioned
- ✅ **Proven reliability** - Google Drive/OneDrive handle availability, backups
- ✅ **Easy debugging** - User can open `.mygamesanywhere/library.json` and inspect

**Functionality:**
- ✅ **Multi-device sync** - Cloud storage providers sync automatically
- ✅ **Conflict resolution** - Simple last-write-wins with timestamp
- ✅ **Works offline** - Local SQLite cache, sync when online
- ✅ **Free storage** - Most users have Google Drive/OneDrive accounts already

**User Experience:**
- Install app → Authorize Google Drive → Add games → Done!
- Install on phone → Same Google account → Library synced automatically
- No server to configure, no accounts to create (besides cloud storage OAuth)

**Trade-offs:**
- ⚠️ Requires cloud storage account (but most users have one already)
- ⚠️ Less instant sync (cloud storage polling vs WebSocket)
- ⚠️ User must trust cloud storage provider (but they already do for photos/docs)

---

### Client-Only Architecture (No Server!)

**Decision:** Clients do EVERYTHING. No server exists!

**Client Responsibilities (EVERYTHING!):**
- ✅ **Run ALL plugins** (Steam scanner, IGDB client, launchers, etc.)
- ✅ **Store ALL data locally** (encrypted via OS keychain)
- ✅ **Scan local filesystem** (Steam library, ROM folders, etc.)
- ✅ **Download game files** directly from sources
- ✅ **Cache media and metadata** locally (SQLite)
- ✅ **Execute games** natively
- ✅ **Handle installations/uninstallations**
- ✅ **Make API calls** with user's personal credentials
- ✅ **Sync to cloud storage** (Google Drive, OneDrive)

**What Gets Synced to Cloud Storage:**
- ✅ Game library (titles, IDs, platforms, metadata)
- ✅ Playtime tracking
- ✅ User preferences (UI settings, sort order, etc.)
- ✅ Sync metadata (timestamps, device IDs)
- ✅ **(Optional)** Encrypted config blob (for multi-device setup)

**What NEVER Leaves User's Device:**
- ❌ Local file paths (`C:\Program Files\Steam\...`)
- ❌ API keys (stored in OS keychain, not synced)
- ❌ OAuth tokens (re-authorize on each device)
- ❌ Local game files
- ❌ Cached media (too large, each device caches independently)

**Benefits:**
- ✅ **Ultimate privacy** - No third-party servers
- ✅ **Zero costs** - No hosting fees
- ✅ **Simple architecture** - Just clients + cloud storage APIs
- ✅ **Offline-first** - Works fully offline with local cache
- ✅ **User owns data** - Can export/delete anytime

**Cloud Storage Structure:**
```
User's Google Drive (or OneDrive)
└── .mygamesanywhere/
    ├── library.json         # Game library (all sources)
    ├── playtime.json        # Playtime tracking
    ├── preferences.json     # User preferences
    ├── sync-meta.json       # Sync metadata
    └── secrets.encrypted    # (Optional) Encrypted config
```

---

### Separate Desktop and Mobile Apps with Shared Core

**Decision:** Build separate optimized apps for desktop and mobile, sharing 60-80% of code via monorepo.

**Alternatives Considered:**
- ❌ **Single responsive app** - Compromised UX on both platforms
- ❌ **Desktop only** - Misses mobile use cases
- ❌ **Mobile only** - Insufficient for power users
- ❌ **Completely separate apps** - No code reuse, duplicate work

**Why Separate Apps with Shared Core?**
- ✅ **Optimal UX:** Each platform gets tailored experience
- ✅ **Code reuse:** 60-80% shared (business logic, components, design system)
- ✅ **Independent evolution:** Can add platform-specific features
- ✅ **Performance:** Can optimize for each platform
- ✅ **Platform conventions:** Desktop feels like desktop, mobile feels like mobile

**Desktop Focus:**
- Multi-column layouts
- Keyboard shortcuts
- Advanced filters and bulk operations
- File system integration
- System tray
- Detailed game information

**Mobile Focus:**
- Single-column touch-optimized layouts
- Swipe gestures and haptics
- Quick launch and simple navigation
- Notifications and widgets
- Platform-specific UI (Material on Android, Cupertino on iOS)
- Battery optimization

**Shared Components:**
- Business logic (GameService, LibraryService, etc.)
- API client
- Data models
- UI components (buttons, cards, modals)
- Design system (colors, typography, spacing)
- State management

**Monorepo Structure:**
```
packages/
  core/         (100% shared)
  ui-shared/    (100% shared)
  desktop/      (platform-specific)
  mobile/       (platform-specific)
```

**Trade-offs:**
- ⚠️ Two build pipelines
- ⚠️ Need to test both platforms
- ⚠️ Some feature parity work to keep experiences similar

---

## Code Organization

### Monorepo with Workspace Packages

**Decision:** Use monorepo (Nx or Turborepo) with separate packages for shared code.

**Why Monorepo?**
- ✅ **Code sharing:** Easy to share packages between desktop and mobile
- ✅ **Atomic commits:** Changes to shared code update both apps in one commit
- ✅ **Single version:** Dependencies managed in one place
- ✅ **Build caching:** Shared build cache speeds up builds
- ✅ **Easier refactoring:** Change shared code, see all impacts immediately

**Trade-offs:**
- ⚠️ Larger repository
- ⚠️ Need monorepo tooling (Nx/Turborepo)
- ⚠️ Longer CI times (need to build multiple packages)

---

### OOP Design Preferred

**Decision:** Use object-oriented programming patterns where possible.

**Why OOP?**
- ✅ **Encapsulation:** Data and methods together
- ✅ **Inheritance:** Code reuse through class hierarchies
- ✅ **Polymorphism:** Flexible interfaces
- ✅ **Testability:** Easy to mock classes
- ✅ **TypeScript support:** Excellent OOP features (classes, interfaces, abstract classes)

**Example:**
```typescript
// OOP approach (preferred)
class GameService {
  constructor(
    private api: APIClient,
    private cache: CacheManager
  ) {}

  async getLibrary(): Promise<Game[]> {
    // Implementation
  }
}

// vs Functional approach (use when appropriate)
function getLibrary(api: APIClient, cache: CacheManager): Promise<Game[]> {
  // Implementation
}
```

**Use functional when:**
- Pure utility functions
- Simple transformations
- No state management needed

---

### RAII (Resource Acquisition Is Initialization) Idioms

**Decision:** Use RAII patterns where language allows (TypeScript via classes, Go via defer).

**What is RAII?**
Resources (files, connections, locks) are acquired in constructors and released in destructors/cleanup methods.

**TypeScript Example:**
```typescript
class DatabaseConnection {
  private connection: Connection;

  constructor(connectionString: string) {
    // Acquire resource
    this.connection = createConnection(connectionString);
  }

  async close(): Promise<void> {
    // Release resource
    await this.connection.close();
  }

  // Use try-finally or async context managers
  async withTransaction<T>(fn: () => Promise<T>): Promise<T> {
    await this.connection.beginTransaction();
    try {
      const result = await fn();
      await this.connection.commit();
      return result;
    } catch (error) {
      await this.connection.rollback();
      throw error;
    }
  }
}
```

**Go Example:**
```go
func ProcessGame(gameID string) error {
    // Acquire resources
    file, err := os.Open("game.dat")
    if err != nil {
        return err
    }
    defer file.Close()  // RAII: cleanup automatically

    lock := acquireLock(gameID)
    defer releaseLock(lock)  // RAII: cleanup automatically

    // Use resources
    return processFile(file)
}  // Resources automatically released here
```

**Why RAII?**
- ✅ **Prevents resource leaks:** Guaranteed cleanup
- ✅ **Less boilerplate:** No manual cleanup in every return path
- ✅ **Exception safe:** Cleanup happens even if errors occur

---

## Development Practices

### Fail-Fast with Detailed Errors

**Decision:** Errors should fail immediately with comprehensive information, no silent fallbacks.

**Why Fail-Fast?**
- ✅ **Easier debugging:** Errors caught at source, not downstream
- ✅ **Clear error messages:** Developers know exactly what went wrong
- ✅ **No hidden bugs:** Problems surface immediately
- ✅ **Better user experience:** Clear error messages > mysterious failures

**Error Format:**
```typescript
class DetailedError extends Error {
  constructor(
    public code: string,           // ERROR_CODE
    public message: string,         // Human-readable message
    public context?: any,          // Additional context
    public cause?: Error           // Original error
  ) {
    super(message);
  }
}

// Usage
if (!game) {
  throw new DetailedError(
    'GAME_NOT_FOUND',
    `Game with ID ${gameId} not found in library`,
    {
      gameId,
      libraryId,
      availableGames: library.games.map(g => g.id),
      timestamp: new Date().toISOString()
    }
  );
}
```

**Go Example:**
```go
type DetailedError struct {
    Code    string                 // ERROR_CODE
    Message string                 // Human-readable
    Context map[string]interface{} // Additional info
    Cause   error                  // Original error
}

func (e *DetailedError) Error() string {
    return fmt.Sprintf("[%s] %s", e.Code, e.Message)
}

// Usage
if game == nil {
    return &DetailedError{
        Code:    "GAME_NOT_FOUND",
        Message: fmt.Sprintf("Game %s not found", gameID),
        Context: map[string]interface{}{
            "gameId":    gameID,
            "libraryId": libraryID,
            "timestamp": time.Now(),
        },
    }
}
```

**No Silent Fallbacks:**
```typescript
// ❌ Bad: Silent fallback
async function getGame(id: string): Promise<Game | null> {
  try {
    return await api.getGame(id);
  } catch {
    return null;  // Lost error information!
  }
}

// ✅ Good: Fail fast
async function getGame(id: string): Promise<Game> {
  try {
    return await api.getGame(id);
  } catch (error) {
    throw new DetailedError(
      'GAME_FETCH_FAILED',
      `Failed to fetch game ${id} from server`,
      { gameId: id },
      error as Error
    );
  }
}
```

**When to use fallbacks:**
Only when explicitly required (e.g., "use cached version if network fails").

---

### Minimize Code Duplication

**Decision:** Aggressively refactor to eliminate duplication.

**Why?**
- ✅ **Single source of truth:** Changes in one place
- ✅ **Easier maintenance:** Fix bugs once, not N times
- ✅ **Consistency:** Same logic produces same results
- ✅ **Smaller codebase:** Less code to read and understand

**Strategies:**
- Extract shared logic into functions/classes
- Use generics/templates for type-agnostic code
- Create base classes for common patterns
- Use composition over inheritance where appropriate

**Example:**
```typescript
// ❌ Duplication
class SteamRepository {
  async fetchGames() {
    const cached = await this.cache.get('steam-games');
    if (cached) return cached;
    const games = await this.api.fetch();
    await this.cache.set('steam-games', games);
    return games;
  }
}

class EpicRepository {
  async fetchGames() {
    const cached = await this.cache.get('epic-games');
    if (cached) return cached;
    const games = await this.api.fetch();
    await this.cache.set('epic-games', games);
    return games;
  }
}

// ✅ Refactored
abstract class BaseRepository {
  constructor(
    protected cache: CacheManager,
    protected cacheKey: string
  ) {}

  async fetchGames(): Promise<Game[]> {
    // Check cache
    const cached = await this.cache.get(this.cacheKey);
    if (cached) return cached;

    // Fetch fresh data
    const games = await this.fetchFromSource();

    // Update cache
    await this.cache.set(this.cacheKey, games);

    return games;
  }

  protected abstract fetchFromSource(): Promise<Game[]>;
}

class SteamRepository extends BaseRepository {
  constructor(cache: CacheManager, private api: SteamAPI) {
    super(cache, 'steam-games');
  }

  protected async fetchFromSource(): Promise<Game[]> {
    return this.api.fetch();
  }
}
```

---

### Code Paragraph Comments

**Decision:** Place blank lines between "code paragraphs" with comments explaining each paragraph's goal.

**Why?**
- ✅ **Readability:** Clear separation of logical steps
- ✅ **Documentation:** Self-documenting code
- ✅ **Easier debugging:** Quickly identify which step failed

**Example:**
```typescript
async function installGame(gameId: string, destination: string): Promise<void> {
  // Validate game exists and get download info
  const game = await this.gameService.getById(gameId);
  if (!game) {
    throw new DetailedError('GAME_NOT_FOUND', `Game ${gameId} not found`);
  }
  const downloadUrl = game.downloadUrl;

  // Create destination directory
  await fs.mkdir(destination, { recursive: true });

  // Download game file
  const tempFile = path.join(destination, 'download.tmp');
  await this.downloadService.download(downloadUrl, tempFile, {
    onProgress: (progress) => this.emit('progress', progress)
  });

  // Extract archive
  await this.extractService.extract(tempFile, destination);

  // Clean up temporary file
  await fs.unlink(tempFile);

  // Update database
  await this.gameService.markInstalled(gameId, destination);

  // Notify user
  this.notificationService.show(`${game.title} installed successfully`);
}
```

---

## Deployment Strategy

### Docker First

**Decision:** Primary deployment is Docker containers.

**Alternatives Considered:**
- ❌ **Native binaries** - Platform-specific, dependency management issues
- ❌ **One-click installers** - Complex to maintain, different for each platform
- ❌ **Manual installation** - Too technical for most users

**Why Docker?**
- ✅ **Consistent environment:** Works same everywhere
- ✅ **Linux focus:** Only need to support Linux in container
- ✅ **Easy deployment:** `docker run` or `docker-compose up`
- ✅ **Cloud-ready:** All cloud providers support Docker
- ✅ **Dependency management:** Everything bundled
- ✅ **Easy updates:** Pull new image, restart container

**For Non-Technical Users:**
- Provide cloud-hosted option (no Docker needed)
- For local: Docker Desktop with simple `docker-compose.yml`
- Future: One-click installer that wraps Docker

**Trade-offs:**
- ⚠️ Requires Docker knowledge for self-hosting
- ⚠️ ~100MB overhead for Docker image
- ⚠️ Windows/Mac users need Docker Desktop

---

## Summary

| Decision | Choice | Key Reason |
|----------|--------|------------|
| **Frontend** | Ionic + Capacitor + React | Cross-platform (mobile + desktop), web tech |
| **Backend** | None! (Serverless) | Zero hosting costs, ultimate privacy |
| **Plugin System** | Client-side TypeScript modules | Privacy, functionality, distributed compute |
| **Storage** | SQLite (local) + JSON (cloud sync) | Fast cache, simple sync, user owns data |
| **Architecture** | Client-only + Cloud Storage | No server costs, user owns data |
| **Sync** | User's cloud storage (Google Drive/OneDrive) | Free, reliable, automatic |
| **App Strategy** | Separate with shared core | Optimal UX, high code reuse (60-80%) |
| **Code Style** | OOP, RAII, fail-fast | Maintainable, debuggable, reliable |
| **Deployment** | Client install only (no server!) | Simple, no infrastructure needed |

All decisions prioritize:
1. **User Experience** - Modern, slick, platform-appropriate
2. **Privacy** - User owns all data, no third-party servers
3. **Developer Experience** - Maintainable, testable, debuggable
4. **Cost Efficiency** - Zero hosting costs, no infrastructure
5. **Cross-Platform** - One codebase, all platforms
6. **Open Source** - Community-friendly, no backend complexity
