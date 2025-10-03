# CLAUDE.md

This file provides guidance to Claude Code (claude.ai/code) when working with code in this repository.

## Project Overview

**MyGamesAnywhere** is a cross-platform game launcher and manager with a pluggable architecture. It's an open-source project (GPL v3) that aims to provide a unified interface for managing games from multiple sources (storefronts, local files, cloud storage, emulators) across desktop and mobile platforms.

**Key Features:**
- Cross-platform (Windows, macOS, Linux, iOS, Android)
- Unified game library from multiple sources
- Automatic metadata and media download
- Emulator support for retro gaming
- **Serverless!** Sync via user's cloud storage
- Modern, slick UI

**Current Phase:** Phase 0 Complete ✅ → Ready for Phase 1 (Core Integrations)

---

## Quick Start (For Future Claude Sessions)

**Before doing anything:**

1. **Read `docs/CURRENT_STATUS.md`** first - tells you exactly where we are
2. **Read `docs/ARCHITECTURE.md`** - understand the system design
3. **Check with user** - don't start coding without explicit approval

**Key Documentation:**
- **Architecture:** `docs/ARCHITECTURE.md` - **CRITICAL: NO SERVER!**
- **Design Decisions:** `docs/DESIGN_DECISIONS.md`
- **Plugin System:** `docs/PLUGINS.md`
- **Data Schema:** `docs/DATABASE_SCHEMA.md` - JSON + SQLite
- **Roadmap:** `docs/ROADMAP.md`
- **Current Status:** `docs/CURRENT_STATUS.md`

---

## Technology Stack

### Frontend (Clients)
- **Framework:** Ionic + Capacitor
- **UI Library:** React
- **Language:** TypeScript
- **Styling:** TailwindCSS
- **State Management:** Redux or Zustand
- **Desktop Packaging:** Electron (via @capacitor-community/electron)
- **Mobile:** Native iOS and Android (via Capacitor)

**What is Capacitor?**
Capacitor is a cross-platform native runtime that lets you build one web app and deploy to iOS, Android, and desktop (via Electron). It provides JavaScript APIs to access native platform features.

### Backend
- **NO BACKEND SERVER!**
- **Storage:** SQLite (local cache) + JSON files (cloud sync)
- **Sync:** User's Google Drive / OneDrive
- **Zero hosting costs!**

### Plugin System (Client-Side)
- **Language:** TypeScript
- **Runtime:** Client-side (desktop: Electron, mobile: Capacitor)
- **Architecture:** Capability-based TypeScript modules
- **Storage:** Encrypted config on client (OS keychain)
- **Privacy:** User's API keys, OAuth tokens stay on device
- **No server!** All plugin data stays local or syncs to user's cloud

### Project Structure (Monorepo)
```
MyGamesAnywhere/
├── packages/
│   ├── core/          # Shared business logic (100% reused)
│   ├── ui-shared/     # Shared UI components (100% reused)
│   ├── desktop/       # Desktop-specific code (15-20%)
│   ├── mobile/        # Mobile-specific code (15-20%)
│   ├── shared-types/  # TypeScript types (100% reused)
│   └── cloud-sync/    # Cloud storage sync service
├── docs/              # Comprehensive documentation
└── .github/           # CI/CD workflows
```

**No server directory!** Serverless architecture.

**Code Reuse:** 60-80% shared between desktop and mobile via monorepo packages.

---

## Architecture Overview

### Serverless Architecture

**NO SERVER!** Instead:
- Clients sync via user's own cloud storage (Google Drive, OneDrive)
- Data stored as JSON files in `.mygamesanywhere/` folder
- Local SQLite cache for fast queries
- Zero hosting costs!

**Clients (Desktop + Mobile - Do Everything!):**
- **Execute ALL plugins** (Steam scanner, IGDB, Google Drive, launchers)
- **Sync to cloud storage** (user's Google Drive/OneDrive)
- Store API keys and OAuth tokens (encrypted via OS keychain)
- Scan local filesystem (Steam library, ROM folders)
- Download games directly from sources
- Cache media and metadata locally (SQLite)
- Execute games natively
- Platform-optimized UIs
- Separate apps sharing core logic (60-80% reuse)

### Plugin System (Client-Side TypeScript)

**CRITICAL: Plugins run on CLIENT! No server!**

**Why Client-Side?**
- **Privacy:** User's data stays local (API keys, OAuth tokens, file paths)
- **Functionality:** Plugins need access to local filesystem and can launch games
- **Performance:** Distributed compute, each user has independent API rate limits
- **Cost:** Zero hosting costs (no server!)

**Capability-Based Design:**
Plugins declare what they can do:
- `canScanInstalled` - Scan for installed games (Steam)
- `canScanAvailable` - Scan for available games (Google Drive)
- `canDownload` - Download games
- `canSearchMetadata` - Search for games (IGDB)
- `canFetchMetadata` - Fetch detailed metadata
- `canFetchMedia` - Fetch covers, screenshots
- `canLaunch` - Launch games
- `canMonitor` - Monitor running processes

**Example Plugins:**
- **Steam:** Scans `C:\Program Files\Steam`, uses user's Steam Web API key (local)
- **Google Drive:** Uses user's personal OAuth token (local), downloads from user's GDrive
- **IGDB:** Uses user's IGDB API key (local), fetches metadata
- **Native Launcher:** Launches `.exe`/`.app` files on user's machine

---

## Development Principles

### Code Style

**Always follow these principles:**

1. **Clarity First**
   - Always ensure 100% clarity and confidence
   - If unsure, ask the user for clarification
   - Never proceed with ambiguity

2. **OOP Preferred**
   - Use object-oriented design where possible
   - Classes over functions for stateful logic
   - Interfaces for plugin contracts

3. **RAII Idioms**
   - Use Resource Acquisition Is Initialization patterns
   - TypeScript: Constructors acquire, cleanup methods release
   - Go: Use `defer` for cleanup

4. **Fail-Fast Policy**
   - Use detailed errors with context (error codes, messages, additional data)
   - NO silent fallbacks or swallowing errors
   - Only fallback when explicitly required

5. **Minimize Duplication**
   - Aggressively refactor to eliminate code duplication
   - Extract shared logic into functions/classes
   - Use base classes or composition

6. **Code Paragraphs**
   - Place blank lines between logical code blocks
   - Add comments explaining each paragraph's goal
   - Makes debugging easier

### Example Code Style

**TypeScript:**
```typescript
class GameService {
  constructor(
    private api: APIClient,
    private cache: CacheManager
  ) {}

  async getLibrary(): Promise<Game[]> {
    // Check cache first
    const cached = await this.cache.get('library');
    if (cached) return cached;

    // Fetch from server
    const games = await this.api.getLibrary();
    if (!games) {
      throw new DetailedError(
        'LIBRARY_FETCH_FAILED',
        'Failed to fetch library from server',
        { timestamp: new Date().toISOString() }
      );
    }

    // Update cache
    await this.cache.set('library', games);

    return games;
  }
}
```

**Go:**
```go
func (s *GameService) GetLibrary(ctx context.Context, userID string) ([]Game, error) {
    // Check cache first
    cached, err := s.cache.Get(ctx, "library:"+userID)
    if err == nil && cached != nil {
        return cached, nil
    }

    // Fetch from database
    games, err := s.db.GetLibrary(ctx, userID)
    if err != nil {
        return nil, &DetailedError{
            Code:    "LIBRARY_FETCH_FAILED",
            Message: "Failed to fetch library from database",
            Context: map[string]interface{}{"userID": userID},
            Cause:   err,
        }
    }

    // Update cache
    s.cache.Set(ctx, "library:"+userID, games, 5*time.Minute)

    return games, nil
}
```

---

## Key Design Decisions

### Why Ionic + Capacitor?
- Cross-platform (desktop + mobile) from one codebase
- Modern web technologies (React, TypeScript)
- Lightweight on mobile (~5-10MB vs Electron's 100MB)
- Native performance (uses platform WebView)
- Maximum code reuse (60-80%)

### Why Go for Backend?
- Extremely efficient (~50-100MB memory)
- Single binary deployment
- Perfect for free hosting tiers
- Excellent concurrency
- Fast compilation

### Why Client-Side TypeScript Plugins?
- **Privacy First:** User data (API keys, OAuth tokens, paths) stays local, encrypted
- **Functionality:** Plugins can access local filesystem and launch games
- **Performance:** Distributed compute, independent API rate limits per user
- **Cost:** Server stays ultra-lightweight (free tier viable)
- **Cross-Platform:** TypeScript works on desktop (Electron) and mobile (Capacitor)

### Why Separate Desktop/Mobile Apps?
- Optimal UX for each platform
- Still share 60-80% of code (business logic, components)
- Desktop: Multi-column, keyboard/mouse, advanced features
- Mobile: Touch-optimized, simplified UI, platform-specific gestures

### Why Server-Light, Client-Heavy?
- Free tier hosting viability
- Server only stores metadata URLs (not actual media)
- Clients do heavy lifting (downloads, caching)
- Scalable (server handles many users)

---

## Database Schema (Quick Reference)

**Server Database:**
- `users` - User accounts
- `libraries` - User game collections
- `repositories` - Game sources (Steam, folders, etc.)
- `games` - Universal game registry
- `game_platforms` - Platform-specific versions (e.g., Halo: Windows + xCloud)
- `game_sources` - Links games to repositories
- `library_games` - Games in user's libraries
- `game_instances` - Actual installations
- `play_sessions` - Play time tracking
- `plugin_registry` - Installed plugins

**Client Database (Cache):**
- `cached_media` - Local media files
- `cached_metadata` - Game metadata
- `local_game_scans` - Scan results

See `docs/DATABASE_SCHEMA.md` for complete schema.

---

## API Quick Reference

**Base URL:** `/v1/`

**Authentication:** JWT Bearer token in `Authorization` header

**Key Endpoints:**
- `POST /auth/register` - Register user
- `POST /auth/login` - Login
- `GET /libraries` - Get user's libraries
- `GET /libraries/:id` - Get library with games
- `POST /repositories` - Add game source
- `POST /repositories/:id/scan` - Scan for games
- `GET /games/:id` - Get game details
- `POST /installations` - Install game
- `GET /plugins` - List plugins

**WebSocket:** `/ws` for real-time updates (installation progress, library updates)

See `docs/API.md` for complete API specification.

---

## Development Workflow

### Before Starting Implementation

**IMPORTANT:** Don't start coding without user approval!

1. Confirm current status with user
2. Review `docs/CURRENT_STATUS.md`
3. Check `docs/ROADMAP.md` for current phase
4. Get explicit approval to proceed

### During Implementation

1. **Use TodoWrite tool** - Track all tasks
2. **Follow code style** - OOP, RAII, fail-fast, code paragraphs
3. **Write tests** - Unit tests for all business logic
4. **Check for duplication** - Refactor aggressively
5. **Detailed errors** - Never swallow errors
6. **Document changes** - Update docs if architecture changes

### Testing

**Server (Go):**
```bash
# Run tests
go test ./...

# Run with coverage
go test -cover ./...

# Run specific package
go test ./internal/services
```

**Client (TypeScript):**
```bash
# Run tests
npm test

# Run with coverage
npm run test:coverage

# Run specific test
npm test GameService
```

---

## Common Commands (Once Implemented)

**Not yet implemented - this section will be updated in Phase 1**

Server commands, client commands, and build commands will be documented here once the project structure is set up.

---

## Current Status

**Phase:** Phase 0 Complete ✅ → Ready for Phase 1 🚀

**NEW Phase Order (Risk-First Approach):**
1. **Phase 1:** Core Integrations (5-6 weeks) - Build Steam, Google Drive, IGDB, Native Launcher as standalone TypeScript libraries
2. **Phase 2:** Plugin System + Minimal Server/Client (4-6 weeks) - Wrap integrations in plugin system
3. **Phase 3:** Full Auth + UI Polish (4-6 weeks) - Production-ready desktop app
4. **Phase 4+:** Installation, Emulation, Cloud Services, Store Integrations, etc.

**Why This Order?**
- Validate hardest integrations FIRST (Steam VDF parsing, OAuth flows, IGDB API)
- Prove integrations work BEFORE building full architecture
- Reduce risk by testing unknowns early

**Next Immediate Steps:**
1. Set up `integration-libs/` workspace
2. Begin Week 1: Steam Scanner (VDF parser, scan installed games)
3. Prove integrations work with unit tests

**See `docs/CURRENT_STATUS.md` for detailed status and `docs/PHASE1_DETAILED.md` for complete Phase 1 plan.**

---

## Important Notes for Future Claude Sessions

1. **Always read `docs/CURRENT_STATUS.md` first** - It tells you exactly where the project is
2. **Don't assume implementation has started** - Check with user before coding
3. **Follow the roadmap** - See `docs/ROADMAP.md` for phases
4. **Respect the design** - All major decisions are documented in `docs/DESIGN_DECISIONS.md`
5. **Ask if unclear** - User prefers explicit questions over assumptions
6. **Use the documentation** - Everything is documented, read it first

---

## Getting Help

- **Architecture questions:** See `docs/ARCHITECTURE.md`
- **Why we chose X:** See `docs/DESIGN_DECISIONS.md`
- **How plugins work:** See `docs/PLUGINS.md`
- **Database structure:** See `docs/DATABASE_SCHEMA.md`
- **API endpoints:** See `docs/API.md`
- **What to build next:** See `docs/ROADMAP.md`
- **Where we are:** See `docs/CURRENT_STATUS.md`

---

## Summary

MyGamesAnywhere is a well-designed, ready-to-build cross-platform game launcher with a **privacy-first, client-side plugin architecture**. All architectural decisions are made, technology is chosen, and documentation is comprehensive. The project uses a **risk-first approach**: build and test core integrations FIRST (Phase 1), then wrap in plugin system (Phase 2), then build full UI (Phase 3).

**Technology:**
- **Client:** Ionic + Capacitor + React + TypeScript (plugins run here!)
- **NO SERVER!** Serverless architecture - cloud storage sync
- **Storage:** SQLite (local) + JSON files (cloud)
- **Plugins:** TypeScript modules on client

**Code Style:** OOP, RAII, fail-fast, minimal duplication, code paragraphs with comments

**CRITICAL Architecture Decision:** NO SERVER! Client-only architecture with cloud storage sync for zero costs and ultimate privacy!

**Status:** Design complete, ready to build Phase 1 (Core Integrations)! 🚀