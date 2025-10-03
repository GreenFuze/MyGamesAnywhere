# Development Roadmap

This document outlines the planned development phases for MyGamesAnywhere.

## Guiding Principles

1. **Start simple, iterate quickly** - MVP first, features later
2. **End-to-end functionality** - Complete flows over partial features
3. **User feedback driven** - Build what users actually need
4. **Open source friendly** - Easy for contributors to join

---

## Phase 0: Foundation (Complete ✅)

**Goal:** Complete design and prepare for implementation.

**Tasks:**
- [x] Define project goals and scope
- [x] Design architecture (client-server model)
- [x] Choose technology stack (Ionic + Capacitor + Go)
- [x] Design database schema
- [x] Design plugin system (client-side TypeScript modules)
- [x] Document everything
- [x] Decide phase ordering (integrations first!)

**Deliverables:**
- ✅ Complete documentation (ARCHITECTURE.md, DESIGN_DECISIONS.md, PLUGINS.md, etc.)
- ✅ Development setup guide
- ✅ Detailed Phase 1 plan (integration libraries)

---

## Phase 1: Core Integrations (Current 🚀)

**Goal:** Build and test core integration libraries as standalone TypeScript packages. Prove hardest integrations work BEFORE building UI or server.

**Duration:** 5-6 weeks

**Detailed Plan:** See [PHASE1_DETAILED.md](./PHASE1_DETAILED.md) for complete specification.

### Why This First?

**Risk Mitigation:**
- Integrations are the hardest and most unknown part
- Building UI first risks discovering integrations don't work
- Standalone packages are easier to test and debug
- Can reuse these packages later in plugin system

**Validate:**
- Steam VDF parsing actually works
- Google Drive OAuth flow actually works
- IGDB API integration actually works
- Game launching actually works

### Deliverables

**Five Standalone TypeScript Packages:**

1. **@mygamesanywhere/steam-scanner** ✅
   - Detect Steam installation
   - Parse libraryfolders.vdf and .acf files
   - Extract installed game information
   - Steam Web API integration (username-based auth)
   - Steam Client integration (install/uninstall/launch)
   - 88 unit tests with real Steam data

2. **@mygamesanywhere/gdrive-client** ✅
   - OAuth 2.0 authentication flow
   - List files in folders
   - Download files with progress
   - 41 unit tests with mocked APIs

3. **@mygamesanywhere/igdb-client** (placeholder)
   - Twitch OAuth (required for IGDB)
   - Search games by title
   - Fetch detailed metadata
   - Rate limiting (4 req/sec)
   - Unit tests with mocked APIs

4. **@mygamesanywhere/generic-repository** ✅ (Phase 1 complete)
   - Scan local/cloud directories for games
   - Detect installers (.exe, .msi, .pkg, .deb, .rpm)
   - Detect portable games (game directories with executables)
   - Detect ROMs (NES, SNES, GB, PlayStation, etc.)
   - Detect archives (single & multi-part: .zip, .rar, .7z, .part1, .z01, etc.)
   - Detect emulator-required games (DOSBox, ScummVM)
   - Cross-platform support (Windows, Linux, macOS, Android, iOS)
   - Repository adapters (Local filesystem, cloud storage ready)
   - Smart executable detection (main game .exe vs config/uninstaller)
   - Phase 1: Core scanning & detection
   - Phase 2: Archive extraction (planned)
   - Phase 3: Installation management (planned)
   - Phase 4: Metadata fetching & save sync (planned)

5. **@mygamesanywhere/config** ✅
   - Centralized configuration for all integrations
   - Single `~/.mygamesanywhere/config.json` file
   - Environment variable overrides
   - Type-safe with Zod validation

6. **@mygamesanywhere/native-launcher** (planned)
   - Platform detection (Windows, macOS, Linux)
   - Launch executables
   - Monitor running processes
   - Track playtime
   - Unit tests

### Technology Stack

**Core:**
- TypeScript 5
- Node.js 18+
- Vitest (testing)
- ESLint + Prettier

**Integration-Specific:**
- Custom VDF parser (Steam)
- google-auth-library (OAuth)
- axios (HTTP)
- child_process (process management)

### Week-by-Week

**Week 1:** Steam Scanner ✅
**Week 2:** Steam Polish + Google Drive Client ✅
**Week 3:** Generic Repository Scanner (Phase 1) ✅
**Week 4:** IGDB Client (planned)
**Week 5:** Native Launcher (planned)
**Week 6:** Integration & Documentation (planned)

### Success Criteria

- [x] Steam scanner package builds without errors
- [x] Steam scanner has 88 passing tests
- [x] Steam scanner finds games on real Steam installation
- [x] Steam Web API integration with username-based auth
- [x] Steam Client can install/uninstall/launch games
- [x] Google Drive OAuth flow works end-to-end
- [x] Google Drive client has 41 passing tests
- [x] Generic repository scanner builds without errors
- [x] Generic repository detects all 6+ game types
- [x] Multi-part archive detection works
- [x] Cross-platform file classification (Windows/Linux/macOS)
- [x] Centralized config package with type-safe validation
- [ ] IGDB client searches and fetches metadata
- [ ] Native Launcher launches and monitors processes
- [x] Each package has README with examples
- [x] All error cases handled gracefully

---

## Phase 2: Plugin System & Cloud Sync

**Goal:** Wrap Phase 1 integration libraries in client-side plugin system. Implement cloud storage sync. Build minimal client to test plugins end-to-end.

**Duration:** 4-6 weeks

### Client Plugin System

**Wrap Integration Libraries:**
- [ ] Design Plugin interface (TypeScript)
- [ ] Design PluginManager class
- [ ] Wrap @mygamesanywhere/steam-scanner as Steam plugin
- [ ] Wrap @mygamesanywhere/gdrive-client as Google Drive plugin
- [ ] Wrap @mygamesanywhere/igdb-client as IGDB plugin
- [ ] Wrap @mygamesanywhere/native-launcher as Native Launcher plugin
- [ ] Implement plugin loading and initialization
- [ ] Implement encrypted config storage (OS keychain)
- [ ] Plugin error handling and circuit breaker

### Cloud Storage Sync

**Google Drive Sync Service:**
- [ ] Google Drive OAuth 2.0 integration
- [ ] Create `.mygamesanywhere/` folder in user's Drive
- [ ] Write library.json (game library)
- [ ] Write playtime.json (playtime tracking)
- [ ] Write preferences.json (user settings)
- [ ] Write sync-meta.json (sync metadata)
- [ ] Read JSON files from cloud storage
- [ ] Merge cloud data with local cache
- [ ] Conflict resolution (last-write-wins)
- [ ] Background sync service (poll for changes)

**SQLite Local Cache:**
- [ ] SQLite database setup
- [ ] Games table with FTS (full-text search)
- [ ] Playtime tracking tables
- [ ] Metadata cache tables
- [ ] Sync state tracking
- [ ] Migration system for schema updates

### Minimal Client

**Just Enough UI to Test Plugins:**
- [ ] Nx monorepo (packages: core, ui-shared, desktop)
- [ ] Ionic + React + TypeScript
- [ ] Cloud storage authorization (Google Drive OAuth)
- [ ] Plugin manager UI (enable/disable, configure)
- [ ] Plugin configuration forms (Steam path, API keys, OAuth)
- [ ] Simple game list view
- [ ] Launch game button
- [ ] Zustand stores (plugins, games, sync)
- [ ] Cloud sync service integration

**NO Full UI Yet:**
- ❌ No polished design
- ❌ No advanced features
- ❌ Just functional test interface

### End-to-End Integration Flow

Test complete plugin system:

1. **Authorize Cloud Storage**
   - User authorizes Google Drive access
   - App creates `.mygamesanywhere/` folder
   - Initial empty library.json created

2. **Configure Steam Plugin**
   - User enters Steam installation path
   - User enters Steam Web API key (stored encrypted in OS keychain)
   - Plugin scans installed games
   - Games stored in local SQLite cache
   - Library synced to cloud (library.json)

3. **Configure Google Drive Plugin**
   - User completes OAuth flow
   - OAuth tokens stored encrypted on client
   - Plugin scans Drive folder for games
   - Games stored in local cache
   - Library synced to cloud

4. **Fetch Metadata**
   - User searches for game in IGDB plugin
   - Plugin fetches metadata and media URLs
   - Metadata cached locally (SQLite)
   - Client downloads cover images
   - Optionally cache metadata in cloud preferences

5. **Launch Game**
   - User clicks "Play" on Steam game
   - Native Launcher plugin launches via Steam
   - Client tracks playtime (SQLite)
   - When game exits, playtime synced to cloud (playtime.json)

6. **Multi-Device Sync** (Test on second device)
   - Install app on phone/laptop
   - Authorize same Google Drive account
   - App downloads library.json from cloud
   - Games appear automatically!

### Success Criteria

- [ ] All 4 plugins load and initialize
- [ ] Plugin config stored encrypted locally (OS keychain)
- [ ] Steam scanner finds installed games
- [ ] Google Drive OAuth flow works
- [ ] IGDB search returns metadata
- [ ] Native Launcher launches games
- [ ] Games sync to cloud storage (library.json)
- [ ] Playtime syncs to cloud (playtime.json)
- [ ] Second device can download and display library
- [ ] Conflict resolution works (last-write-wins)
- [ ] Offline mode works (uses local cache)
- [ ] User can play game end-to-end
- [ ] Minimal UI is functional (not pretty)

### Deliverables

- ✅ Working plugin system (client-side)
- ✅ 4 working plugins (Steam, GDrive, IGDB, Native Launcher)
- ✅ Cloud storage sync (Google Drive)
- ✅ SQLite local cache
- ✅ Minimal client (test UI)
- ✅ End-to-end flow proven
- ✅ Multi-device sync proven
- ✅ Encrypted config storage

---

## Phase 3: Full UI Polish & Desktop Features

**Goal:** Build polished desktop UI with full cloud storage integration and library management. Replace Phase 2's minimal UI with production-ready interface.

**Duration:** 4-6 weeks

### Cloud Storage Enhancements

- [ ] OneDrive support (in addition to Google Drive)
- [ ] Cloud storage provider selection
- [ ] Token refresh handling
- [ ] Better sync conflict UI
- [ ] Sync status indicators
- [ ] Manual sync trigger
- [ ] Sync history view

### Client UI (Desktop)

#### Cloud Storage Setup
- [ ] Beautiful cloud storage authorization flow
- [ ] Provider selection (Google Drive, OneDrive)
- [ ] Authorization status display
- [ ] Re-authorization flow
- [ ] Storage usage display

#### Library Management
- [ ] Grid view with customizable sizes
- [ ] List view option
- [ ] Advanced filtering (genre, platform, installed/uninstalled)
- [ ] Sorting (name, recent, playtime, rating)
- [ ] Search within library
- [ ] Multiple libraries support
- [ ] Library switching
- [ ] Default library indicator

#### Game Details
- [ ] Full-screen game details modal
- [ ] Cover art, screenshots, videos
- [ ] Game description and metadata
- [ ] Play button (launch game)
- [ ] Add to favorites
- [ ] Play history
- [ ] Playtime stats

#### Layout & Navigation
- [ ] Beautiful app header
- [ ] Sidebar navigation
- [ ] Settings page
- [ ] Plugin management page
- [ ] Profile page
- [ ] Responsive layouts

#### Design System
- [ ] TailwindCSS configuration
- [ ] Ionic component customization
- [ ] Dark theme (default)
- [ ] Light theme option
- [ ] Color scheme
- [ ] Typography system
- [ ] Icon system

### Testing & Quality

- [ ] Vitest unit tests (60%+ coverage)
- [ ] E2E tests (Cypress or Playwright)
- [ ] Accessibility (WCAG 2.1 AA)
- [ ] Performance optimization
- [ ] Error boundary components

### DevOps

- [ ] CI/CD with GitHub Actions
- [ ] Automated tests on PR
- [ ] Linting and formatting checks
- [ ] Docker build pipeline
- [ ] Electron packaging

### Success Criteria

- [ ] Beautiful, modern UI
- [ ] Full auth flow works perfectly
- [ ] Library management is intuitive
- [ ] Games display with media
- [ ] Plugins integrate seamlessly
- [ ] Performance is smooth (60fps)
- [ ] All tests pass
- [ ] Ready for alpha users

### Deliverables

- ✅ Production-ready desktop UI
- ✅ Full authentication system
- ✅ Polished library management
- ✅ Beautiful game details
- ✅ Complete testing suite
- ✅ CI/CD pipeline
- ✅ Ready for alpha release

---

## Phase 4: Installation & Downloads

**Goal:** Allow users to download and install games.

**Duration:** 4-6 weeks

### Server Features

#### Installation API
- [ ] Installation endpoints
- [ ] Track installation progress
- [ ] WebSocket events for progress updates

#### Source Plugin: URL-based
- [ ] Add game from URL
- [ ] Download file
- [ ] Verify checksum
- [ ] Extract archives

### Client Features

#### Download Manager
- [ ] Download queue
- [ ] Progress tracking
- [ ] Pause/resume downloads
- [ ] Background downloads (desktop)
- [ ] Download notifications

#### Installation Manager
- [ ] Install games from downloads
- [ ] Uninstall games
- [ ] Verify installations
- [ ] Manage install locations

#### Caching
- [ ] Local media cache (covers, screenshots)
- [ ] Metadata cache
- [ ] Cache management (size limits, eviction)
- [ ] Optional user cloud storage cache

**Deliverables:**
- Full download and installation system
- URL-based game source
- Background downloads
- Cache management

---

## Phase 5: Emulation Support

**Goal:** Add emulator support for retro gaming.

**Duration:** 6-8 weeks

### Server Plugins

#### Platform Plugin: RetroArch
- [ ] Auto-detect RetroArch installation
- [ ] Auto-install RetroArch if missing
- [ ] Download cores automatically
- [ ] Configure core for ROM type
- [ ] Launch games via RetroArch

#### Source Plugin: ROM Folder Scanner
- [ ] Detect ROM files (.nes, .snes, .gba, .iso, etc.)
- [ ] Identify system from file extension
- [ ] Match ROMs to metadata

#### Metadata Plugin: LaunchBox Games DB
- [ ] Integrate LaunchBox API
- [ ] Fetch retro game metadata
- [ ] Match ROMs to games

### Client Features

#### Emulator Management
- [ ] List installed emulators
- [ ] Install/update emulators
- [ ] Configure emulator settings
- [ ] Set default emulator per platform

#### ROM Management
- [ ] Import ROM folders
- [ ] Organize by system
- [ ] Show system-specific views
- [ ] Handle multi-disc games

**Deliverables:**
- RetroArch integration
- ROM library management
- Multiple retro systems supported (NES, SNES, Genesis, PS1, etc.)
- LaunchBox metadata

---

## Phase 6: Advanced Sync & Additional Cloud Providers

**Goal:** Add more cloud storage providers and advanced sync features.

**Duration:** 3-4 weeks

**Note:** Basic multi-device sync via Google Drive already implemented in Phase 2. This phase adds more providers and features.

### Additional Cloud Storage Providers

- [ ] Dropbox integration
- [ ] iCloud Drive support (for Apple users)
- [ ] Self-hosted options (WebDAV, ownCloud, Nextcloud)
- [ ] Local network sync (SMB/NFS shares)

### Advanced Sync Features

- [ ] Selective sync (choose what to sync)
- [ ] Sync scheduling (hourly, daily, manual only)
- [ ] Bandwidth limiting
- [ ] Compression for large libraries
- [ ] Delta sync (only sync changes, not full files)
- [ ] Conflict resolution UI improvements
- [ ] Sync history and rollback
- [ ] Multiple accounts support (family sharing)

### Data Management

- [ ] Export library to JSON/CSV
- [ ] Import library from JSON/CSV
- [ ] Backup and restore tools
- [ ] Data cleanup tools (remove orphaned entries)
- [ ] Storage usage analytics

**Deliverables:**
- Multiple cloud storage provider support
- Advanced sync features
- Data export/import tools
- Better sync conflict handling

---

## Phase 7: Additional Store Integrations

**Goal:** Integrate major game storefronts.

**Duration:** 8-12 weeks (parallel development of plugins)

### Source Plugins

#### Steam
- [ ] Detect Steam installation
- [ ] Read Steam library
- [ ] Show installed games
- [ ] Launch via Steam

#### Epic Games Store
- [ ] Detect Epic installation
- [ ] Read Epic library
- [ ] Show installed games
- [ ] Launch via Epic

#### GOG Galaxy
- [ ] Detect GOG installation
- [ ] Read GOG library
- [ ] Show installed games
- [ ] Launch via GOG

#### Xbox (PC Game Pass)
- [ ] Authenticate with Microsoft
- [ ] List available games
- [ ] Support install and streaming modes
- [ ] Launch via Xbox app

### Client Features

- [ ] Unified library across all stores
- [ ] Filter by store
- [ ] Show which store owns each game
- [ ] Launch via appropriate store

**Deliverables:**
- 4+ major storefront integrations
- Unified game library
- Store-specific features

---

## Phase 8: Advanced Features

**Goal:** Polish and advanced functionality.

**Duration:** Ongoing

### Feature Plugins

#### Cloud Save Sync
- [ ] Upload saves to user's cloud storage
- [ ] Download saves on new device
- [ ] Detect save conflicts
- [ ] Resolve conflicts

#### Playtime Tracking
- [ ] Track play sessions
- [ ] Statistics dashboard
- [ ] Charts and graphs
- [ ] Export statistics

#### Screenshot Capture
- [ ] Capture screenshots during gameplay
- [ ] Organize screenshots by game
- [ ] Share screenshots
- [ ] Screenshot gallery

#### Controller Mapping
- [ ] Detect controllers
- [ ] Configure button mappings
- [ ] Per-game profiles
- [ ] Community presets

### UI Plugins

#### Big Picture Mode
- [ ] TV-friendly UI
- [ ] Controller navigation
- [ ] Full-screen mode
- [ ] Simplified interface

#### Themes
- [ ] Dark theme (default)
- [ ] Light theme
- [ ] Custom color schemes
- [ ] Community themes

### Mobile Features

#### Widgets
- [ ] Recently played widget (iOS/Android)
- [ ] Quick launch widget
- [ ] Playtime widget

#### Remote Play
- [ ] Launch games on desktop from mobile
- [ ] View game status
- [ ] Install/uninstall remotely

**Deliverables:**
- Feature plugins (cloud saves, tracking, screenshots, etc.)
- UI plugins (Big Picture, themes)
- Mobile-specific features (widgets, remote control)

---

## Phase 9: Community & Ecosystem

**Goal:** Build community, plugin marketplace, documentation.

**Duration:** Ongoing

### Plugin Marketplace

- [ ] Plugin discovery
- [ ] Browse community plugins
- [ ] One-click install
- [ ] Plugin ratings and reviews
- [ ] Plugin updates

### Documentation

- [ ] User documentation
- [ ] Plugin development guide
- [ ] API documentation
- [ ] Video tutorials
- [ ] Example plugins

### Community

- [ ] Forum/Discord
- [ ] Plugin submission process
- [ ] Contributor guidelines
- [ ] Bug bounty program
- [ ] Regular releases

**Deliverables:**
- Plugin marketplace
- Comprehensive documentation
- Active community
- Regular plugin releases

---

## Future Considerations

Ideas for future development (not committed):

### Performance
- Server-side caching option
- Read replicas for database
- CDN for media assets
- Client-side performance optimizations

### Advanced Integrations
- Achievement tracking (Steam, Xbox, etc.)
- Friend lists and social features
- Streaming to TV (Steam Link style)
- VR game support

### Platform Expansion
- Web client (Progressive Web App)
- Smart TV apps
- Game console integration

### AI/ML Features
- Automatic game genre detection
- Personalized recommendations
- Smart metadata matching
- Duplicate detection

---

## Release Schedule

### Alpha (After Phase 3)
- Invite-only testing
- Core functionality working (plugins + UI)
- Bugs expected
- Breaking changes possible

### Beta (After Phase 5)
- Public testing
- Most features working
- Feature complete for v1.0
- API stable

### v1.0 (After Phase 7)
- Production ready
- All core features working
- Store integrations complete
- Stable API
- Documentation complete

### v1.x (Phase 8+)
- Additional features
- UI improvements
- More plugins
- Community contributions

### v2.0 (Phase 9+)
- Major new features
- API v2
- Performance improvements
- Marketplace launch

---

## Success Metrics

### Phase 1 Success
- [ ] Steam scanner finds installed games on real Steam installation
- [ ] Google Drive OAuth flow works end-to-end
- [ ] IGDB client searches and fetches game metadata
- [ ] Native Launcher launches and monitors processes
- [ ] All 4 packages have 85%+ test coverage
- [ ] Each package has clear API and documentation

### Phase 2 Success
- [ ] Plugins load and initialize successfully
- [ ] Plugin config stored encrypted locally
- [ ] Steam plugin scans and syncs games to server
- [ ] User can launch game end-to-end
- [ ] Server NEVER sees API keys or local paths

### Phase 3 Success
- [ ] Beautiful, polished desktop UI
- [ ] Full authentication flow works perfectly
- [ ] Library management is intuitive and fast
- [ ] Ready for alpha testing

### Phase 5 Success
- [ ] Can play retro games via RetroArch
- [ ] ROM library management works
- [ ] Metadata fetching works for retro games

### Phase 7 Success
- [ ] At least 3 major storefronts integrated
- [ ] Unified library shows all games
- [ ] Can launch games from any store

### v1.0 Success
- [ ] 1000+ active users
- [ ] <5% crash rate
- [ ] Average 4+ star rating
- [ ] Active community (forum/Discord)
- [ ] 10+ community plugins

---

## Contributing

See [CONTRIBUTING.md](../CONTRIBUTING.md) for how to contribute to the project.

Community contributions are welcome at all phases!

---

## Summary

**Current Status:** Phase 0 Complete ✅ → Ready for Phase 1 🚀

**Phase Order (Risk-First Approach):**
1. **Phase 1:** Core Integrations (Steam, Google Drive, IGDB, Native Launcher as standalone libraries)
2. **Phase 2:** Plugin System + Minimal Client/Server (wrap integrations, prove it works)
3. **Phase 3:** Full Auth + UI Polish (production-ready desktop app)
4. **Phase 4:** Installation & Downloads
5. **Phase 5:** Emulation Support
6. **Phase 6:** Cloud Services & Multi-Device Sync
7. **Phase 7:** Additional Store Integrations
8. **Phase 8:** Advanced Features
9. **Phase 9:** Community & Ecosystem

**Next Immediate Steps:**
1. Set up integration libraries workspace
2. Begin Phase 1: Steam Scanner implementation
3. Prove integrations work BEFORE building full architecture

**Estimated Timeline to v1.0:** 10-12 months

See [CURRENT_STATUS.md](./CURRENT_STATUS.md) for current progress, [ARCHITECTURE.md](./ARCHITECTURE.md) for system design, and [PHASE1_DETAILED.md](./PHASE1_DETAILED.md) for detailed Phase 1 plan.
