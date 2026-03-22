# MyGamesAnywhere — Roadmap

## Completed

### Server Core
- [x] Plugin architecture (IPC, length-prefixed JSON over stdin/stdout)
- [x] Game source plugins: SMB, Steam, Xbox, Epic, Google Drive
- [x] Metadata resolver plugins: IGDB, RAWG, Steam, GOG, LaunchBox, MAME DAT, HLTB, RetroAchievements, TGDB (disabled)
- [x] 3-phase metadata orchestrator (Identify → Consensus → Fill)
- [x] Scanner pipeline (file detection, platform detection, title normalization, role classification, grouping)
- [x] Achievement support (Steam, Xbox, RetroAchievements — on-demand)
- [x] MediaItem model (URL, type, source — download deferred)
- [x] CompletionTime model (HLTB integration)
- [x] Database schema (SQLite, WAL mode, foreign keys)
- [x] GameStore persistence layer (transactional writes, canonical game views, soft deletes, move detection)
- [x] REST API (games, integrations, plugins, scan, achievements, config)
- [x] System tray (Windows)
- [x] Build system (build.ps1 — server + all plugins)

### Bugs Fixed
- [x] HTTP middleware timeout killing long scans (detached context + exempt scan route)
- [x] SMB plugin.check_config not unwrapping `{"config": ...}` wrapper
- [x] SMB Guest credentials → permission denied (switched to real credentials)
- [x] SMB plugin missing plugin.init handler
- [x] Plugin controller not wrapping config for check_config IPC calls

---

## Phase 0 — Backend Prep

### Settings Sync
- [x] JSON export format for sync payload (integrations, user settings, game overrides — NOT source games/metadata)
- [x] Encrypt secrets in sync payload
- [x] `POST /api/sync/push` — export local state → Google Drive (`My Drive/Games/mga_sync/`)
- [x] `POST /api/sync/pull` — download remote state → merge into local DB
- [x] Versioned backups on Drive (timestamped files + `latest.json` pointer)
- [x] Merge logic: add missing, update older, never delete
- [x] Google Drive sync plugin: `sync.push` / `sync.pull` IPC methods

### Server-Sent Events
- [ ] `GET /api/events` SSE endpoint
- [ ] Scan progress events (started, per-integration progress, completed, error)
- [ ] Notification events (integration status changes, errors)

### Static File Serving
- [ ] `go:embed` frontend dist into server binary
- [ ] Serve `/*` (excluding `/api/*`) from embedded SPA
- [ ] SPA fallback: all non-file routes return `index.html`

### Tray Icon
- [ ] Add "Open Web Frontend" menu item → opens `http://localhost:{port}` in default browser

### API Enhancements
- [ ] Duplicate integration prevention (`POST /api/integrations` rejects same plugin_id + same config)
- [ ] `GET /api/games/{id}` — full detail response with metadata_json, media, external IDs
- [ ] `GET /api/games/{id}/play` — stream game file from source (for emulator playback)
- [ ] `GET /api/stats` — library statistics (counts by platform, genre, source, metadata coverage)
- [ ] Frontend preferences: `GET/POST /api/config/frontend` (theme, view mode, sidebar state)

### xCloud Catalog
- [ ] Extend Xbox source/metadata plugin to detect xCloud availability
- [ ] Store `xcloud_available` flag and `xcloud_url` per game
- [ ] xCloud badge in game data

---

## Phase 1 — Frontend Scaffold

### Project Setup
- [ ] Initialize Vite + React + TypeScript in `server/frontend/`
- [ ] Tailwind CSS + PostCSS configuration
- [ ] shadcn/ui component foundation (headless, accessible, owned)
- [ ] React Router (client-side routing)
- [ ] TanStack Query (API data fetching + caching)
- [ ] Auto-generated API client from `openapi.yaml`
- [ ] Vite dev proxy to Go server (`localhost:8900`)
- [ ] Add `server/frontend/node_modules/` and `server/frontend/dist/` to `.gitignore`

### Shell & Layout
- [ ] App shell: sidebar + topbar + main content area
- [ ] Sidebar navigation (Library, Playable, Settings, About)
- [ ] Topbar: search bar (Ctrl+K), theme toggle, notification bell
- [ ] Responsive breakpoints (desktop, tablet, mobile)
- [ ] Loading states and error boundaries

### Theme Engine
- [ ] `MgaTheme` TypeScript interface (colors, typography, shape, elevation, card, layout, motion, effects, badge, scrollbar)
- [ ] Theme context provider
- [ ] CSS custom property injection from theme object
- [ ] Theme persistence via `/api/config/frontend`
- [ ] Theme selector component with live preview
- [ ] `prefers-reduced-motion` and `prefers-color-scheme` respect

### All 11 Themes
- [ ] **Midnight** — default dark, charcoal + blue accent, clean professional
- [ ] **Daylight** — classic light, white canvas + soft shadows, airy modern
- [ ] **Deep Blue** — blue-gray + teal highlights, rounded cards, PC gamer feel
- [ ] **Obsidian** — true black + vivid green, frosted glass, console-premium
- [ ] **Curator** — purple-slate + warm gold, compact density, power user
- [ ] **Big Screen** — deep navy + electric blue glow, oversized cards, TV/gamepad friendly
- [ ] **Retro Terminal** — phosphor green/amber, monospace font, pixel-art sensibility, no gimmicky overlays
- [ ] **Synthwave** — purple gradient + hot pink/cyan, neon glow, gradient accents, bold
- [ ] **Cinema** — pure black + warm gold, ultra-high contrast, minimal chrome, OLED-perfect
- [ ] **Frost** — Nord palette, arctic blue-gray, muted aurora accents, calm sophisticated
- [ ] **Neon Arcade** — 80s arcade, bright neon palette, chunky display font, uppercase, pixel grid pattern

### Logo & Branding
- [ ] MGA logo design (used as favicon, loading screen, tray icon, about page)
- [ ] Tray icon update with logo

---

## Phase 2 — Game Library

### Library Views
- [ ] Grid view (cover art cards with metadata badges)
- [ ] List view (compact table with sortable columns)
- [ ] View mode toggle (persisted in preferences)

### Game Cards
- [ ] Cover art with lazy loading and placeholder
- [ ] Platform icon badge (Steam, GBA, PS1, Arcade, ScummVM, DOS, etc.)
- [ ] Source badge (which integration found it)
- [ ] Achievement progress ring (if available)
- [ ] HLTB time estimate badge
- [ ] Metadata confidence indicator (number of resolvers matched)
- [ ] "Playable" badge (browser-emulatable platforms)
- [ ] "xCloud" badge (cloud-playable Xbox games)
- [ ] Play button on playable games vs. "View" on others

### Search & Filtering
- [ ] Full-text search with fuzzy matching
- [ ] Keyboard shortcut (Ctrl+K) to focus search
- [ ] Filter sidebar: platform, genre, year range, developer/publisher, source, playable-only
- [ ] Sort by: title, release date, recently added, HLTB time, platform

### Library Sections
- [ ] **All Games** — complete library
- [ ] **Playable** — filtered to browser-emulatable platforms
- [ ] **xCloud** — Xbox Game Pass cloud-playable games

---

## Phase 3 — Game Detail Page

### Metadata Display
- [ ] Full-bleed hero banner (cover art, blurred background)
- [ ] Description, release date, developer, publisher, genres, rating
- [ ] In-context attribution: source logos next to data they provided (IGDB logo next to description, etc.)

### Media Gallery
- [ ] Screenshot viewer (lightbox)
- [ ] Video embeds (if available)
- [ ] Media source attribution

### External Links
- [ ] IGDB, RAWG, Steam Store, GOG, LaunchBox links with service favicons/logos

### Achievements
- [ ] Achievement list with icons, descriptions, progress
- [ ] Source attribution (RetroAchievements logo, Steam logo, Xbox logo)
- [ ] Overall progress bar

### Completion Times
- [ ] HLTB main story / completionist / combined display
- [ ] HLTB logo attribution

### Play
- [ ] "Play in Browser" button for supported platforms
- [ ] "Play on xCloud" button → opens xCloud URL
- [ ] Source info: file paths, integration details, resolver match data

---

## Phase 4 — Settings & Admin

### Integrations
- [ ] List active integrations with status indicators
- [ ] Add new integration (plugin selector, config form, test connection)
- [ ] Remove integration
- [ ] Edit integration config

### Plugins
- [ ] Discovered plugins list with version, capabilities, enabled/disabled status

### Scanning
- [ ] Trigger full scan button
- [ ] Trigger per-integration scan
- [ ] Scan progress display (driven by SSE)
- [ ] Last scan results summary

### Sync
- [ ] Push to Google Drive button with confirmation dialog
- [ ] Pull from Google Drive with merge preview
- [ ] Backup history browser (timestamped snapshots)
- [ ] "Last synced" indicator

### Theme Selector
- [ ] Visual theme previews (thumbnails or live mini-preview)
- [ ] One-click theme switching with instant feedback

---

## Phase 5 — Real-Time Updates

- [ ] SSE client integration (auto-reconnect, event parsing)
- [ ] Scan progress bar (global, in topbar or toast)
- [ ] Toast notification system (non-blocking)
- [ ] Notification types: scan complete, scan error, integration status change, sync complete

---

## Phase 6 — In-Browser Emulation

### Emulator Engines
- [ ] EmulatorJS integration (NES, SNES, GBA, N64, PS1, Genesis, Arcade/MAME)
- [ ] js-dos integration (MS-DOS games)
- [ ] ScummVM WASM integration (point-and-click adventures)

### Player UI
- [ ] Fullscreen mode
- [ ] Save states
- [ ] Controller mapping / keyboard overlay
- [ ] Exit back to library

### ROM Streaming
- [ ] `GET /api/games/{id}/play` serves game files from source (SMB, Drive, etc.)
- [ ] Platform-to-emulator-core mapping

### xCloud
- [ ] xCloud launch (iframe or external link to `xbox.com/play/launch/{titleId}`)
- [ ] Evaluate Better xCloud enhancements (open source)

---

## Phase 7 — Polish & Advanced Features

### Animations & Motion
- [ ] Page transition animations
- [ ] Staggered grid item entrance
- [ ] Smooth filter/sort transitions
- [ ] Skeleton loading states

### Gamepad Navigation
- [ ] Navigate entire UI with controller
- [ ] Focus management, visual focus indicators
- [ ] Optimized for Big Screen theme

### Dashboard / Stats
- [ ] Games by platform (chart)
- [ ] Games by decade
- [ ] Top genres
- [ ] Metadata coverage (% with descriptions, cover art, achievements)
- [ ] Recent scan activity

### Collections
- [ ] User-created groupings ("Couch co-op", "Childhood favorites")
- [ ] "What should I play?" random picker (factors in HLTB and mood)

### About Page
- [ ] MGA version, build date, author credits
- [ ] "Powered By" grid with logos and one-liner descriptions for all services:
  IGDB, RAWG, Steam, GOG, LaunchBox, MAME, HowLongToBeat, RetroAchievements,
  TheGamesDB, EmulatorJS/RetroArch, js-dos/DOSBox, ScummVM, Xbox/xCloud,
  Epic Games, Google Drive
- [ ] "View Open Source Licenses" link
- [ ] In-context attribution throughout the app (service logos next to their data)

### Additional Ideas
- [ ] Keyboard shortcuts (Vim-style navigation, quick actions)
- [ ] Import/export library (JSON/CSV)
- [ ] Game comparison view (side-by-side metadata)
- [ ] Timeline view (library by release year)
- [ ] Responsive mobile/tablet design

---

## Known Issues / Deferred

- [ ] HLTB API returning 404 (endpoint may have changed — needs investigation)
- [ ] RetroAchievements integration needs username in config
- [ ] Duplicate integration prevention not yet implemented
- [ ] TGDB disabled due to low API quota
- [ ] Media download background worker (MediaItems have URLs but no local files yet)
- [ ] Schema migration strategy (deferred until after first release)
