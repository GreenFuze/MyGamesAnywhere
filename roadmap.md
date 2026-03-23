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
- [x] Server-Sent Events: `GET /api/events`; detailed scan pipeline events (source list, scanner, metadata identify/consensus/fill per resolver, persist, skips); `ts` on payloads; catalog in `server/internal/events/scan_events.md`; event bus closed before HTTP shutdown
- [x] System tray (Windows)
- [x] Build system ([`build.ps1`](server/build.ps1) — server + plugins + frontend → `server/bin/`; [`run.ps1`](server/run.ps1) / [`start.ps1`](server/start.ps1) run `mga_server` from bin; [`build_and_start.ps1`](server/build_and_start.ps1); `-SkipFrontend` on build when Node is unavailable)

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
- [x] `GET /api/events` SSE endpoint
- [x] Scan progress events (started, per-integration, source listing, scanner grouping, metadata phases + per-resolver IPC, persist, skips, completed, error); catalog: [`server/internal/events/scan_events.md`](server/internal/events/scan_events.md)
- [x] Notification events: integration create + status run (`index`/`total`), sync push/pull + key store/clear, achievement fetch errors, unexpected plugin process exit; catalog: [`server/internal/events/notification_events.md`](server/internal/events/notification_events.md)

### API Enhancements

> **Plan (Phase 0)** — *Implemented:* duplicate integrations, `GET /api/games/{id}/detail`, `GET /api/stats`, dedicated frontend config. *`GET /api/games/{id}/play` remains **Phase 6** (in-browser emulation / ROM streaming).*  
> **Design choices:** no premature optimization — full `resolver_matches` (incl. `metadata_json`) always on detail.  
> **Contracts:** [`server/openapi.yaml`](server/openapi.yaml) + [`server/internal/openapi/operations.go`](server/internal/openapi/operations.go) updated; regenerate with `go run ./cmd/openapi-gen` from `server/`.

- [x] **Duplicate integration prevention** — `ListByPluginID` + `reflect.DeepEqual` on decoded JSON objects; **409** with `duplicate_integration`, `integration_id`, and `integration` object.
- [x] **`GET /api/games/{id}/detail`** — Full detail DTO; media includes `local_path`, `hash`, `mime_type`; per–source-game `resolver_matches` always included.
- [x] **`GET /api/stats`** — `GameStore.GetLibraryStats`: single JSON (`LibraryStats` in [`core/entities.go`](server/internal/core/entities.go)).
- [x] **Frontend preferences** — **`GET` / `POST /api/config/frontend`** only for SPA prefs; settings key **`frontend`**; max body 256KiB.

### xCloud Catalog

> **Architecture** — Xbox library data is **`game-source-xbox`** only (no separate Xbox metadata plugin). [`fetchTitleHistory`](server/plugins/xbox-source/main.go) requests **`ProductId`**, **`TitleHistory`**, and **`filterTo=IsStreamable,IsGame`** (plus legacy decorations); [`fetchGames`](server/internal/scan/orchestrator.go) forwards **`is_game_pass`**, **`xcloud_*`**, and **`store_product_id`** into resolver metadata.

**SuitCode:** `get_minimum_verified_change_set_by_path` on `server/plugins/xbox-source/main.go` → `go test` / `go build` that module after edits.

**Title Hub — confirmed approach (2026-03):** Same **`titlehub.xboxlive.com`** + user **XSTS** auth, but change the **decoration list** and **query** to match the xCloud web client, e.g.:  
`.../titles/titleHistory/decoration/GamePass,ProductId,TitleHistory?filterTo=IsStreamable,IsGame&supportedPlatform=StreamableOnly`  
(plus existing `maxItems`; keep **`x-xbl-contract-version: 2`**, **`x-xbl-market`**, **`accept-language`** as today.)

**Fields to parse per title (from live JSON):**
- **`isStreamable`** → **`xcloud_available`** (authoritative for cloud play in this pipeline).
- **`gamePass.isGamePass`** → **Game Pass catalog** entitlement flag (includes GP titles; **not** the same as xCloud — e.g. *Enter the Gungeon* can be `isGamePass: false` and **`isStreamable: true`** if you own it / it’s otherwise streamable).
- **`productId`** (e.g. `9P20JCF7BV93`) → **Store BigId** for web launch; aligns with URLs like `https://www.xbox.com/en-US/play/launch/final-fantasy/9P20JCF7BV93`.
- **`titleId`** in this response is a **decimal string**; align with **`external_id`** / achievements (hex vs decimal — normalize in one place).
- **`name`** → derive **URL slug** for `/play/launch/{slug}/{productId}` (match xbox.com rules: lowercase, hyphens, strip ™/®; verify edge cases against Display Catalog or a small golden-file list).

**`xcloud_url`:** Build when **`isStreamable`** (and optionally only when `productId` looks like a launchable id, e.g. `9P…` / `9N…`). Use configurable locale segment (default `en-US`). Do **not** log or store **Authorization** tokens.

**Optional cross-checks (if ever needed):** [Better xCloud](https://github.com/redphx/better-xcloud) references **Display Catalog** (`displaycatalog.mp.microsoft.com`) and **`catalog.gamepass.com/sigls/`** — useful for debugging or slug mismatches, not required if Title Hub keeps returning `productId` + `isStreamable`.

**Implementation plan:**
- [x] **`xbox-source`:** Title Hub uses **ProductId,TitleHistory** + **`filterTo=IsStreamable,IsGame`** (no `StreamableOnly` so the full library is returned); extend `title` / [`gameEntry`](server/plugins/xbox-source/main.go) with `xcloud_available`, `store_product_id`, `xcloud_url`; keep `is_game_pass`; optional config **`xbl_market`**, **`play_launch_locale`**; **`x-xbl-market`** header on Title Hub requests.
- [x] **Orchestrator [`fetchGames`](server/internal/scan/orchestrator.go):** Decode and forward **`is_game_pass`**, **`xcloud_available`**, **`xcloud_url`**, **`store_product_id`** into the initial resolver row from storefront plugins.
- [x] **Persistence:** [`metadataExtra`](server/internal/db/game_store.go) / `metadata_json` carries these flags + URL; unified canonical view merges them in **`computeUnifiedView`**.
- [x] **API:** [`GET /api/games/{id}/detail`](server/internal/http/game_detail.go) exposes unified Xbox/xCloud fields plus per-source **`resolver_matches`**. **`GET /api/games`** and **`GET /api/games/{id}`** include the same unified fields on each summary ([`GameSummary`](server/internal/http/controllers.go)).
- [x] **Tests:** [`titlehistory_decode_test.go`](server/plugins/xbox-source/titlehistory_decode_test.go) fixture (redacted) for `titleId` number/string, streamable URL slug.
- [x] **Local verify script:** [`server/scripts/verify-xbox-xcloud.ps1`](server/scripts/verify-xbox-xcloud.ps1) — with server running from `server\bin` after [`build.ps1`](server/build.ps1), add a **`game-source-xbox`** integration, then `pwsh -File server/scripts/verify-xbox-xcloud.ps1` (optional `MGA_BASE_URL`).

- [x] Extend Xbox **source** plugin + orchestrator (Title Hub `isStreamable` + `productId` + launch URL)
- [x] Store `xcloud_available`, `xcloud_url`, and Game Pass flag in persisted game metadata (`metadata_json`)

---

## Phase 1 — Frontend Scaffold

Phases **1–7** are **frontend / product** milestones (UI, client logic). **Phase 0** is **backend prep** on the Go server; completed Phase 0 items (e.g. SSE scan stream) are dependencies for later UI work, not partial completion of Phase 4 or 5.

### Phase 1 plan (execution)

**Goals** — Ship a **runnable SPA** in `server/frontend/` that talks to the existing Go API (dev: Vite + proxy; prod: optional embed in `mga_server`), with **app shell**, **routing placeholders**, **theme system** + **persistence via** `GET`/`POST /api/config/frontend`, and **11 visual themes**. Real library/game UI stays **Phase 2+**.

**Definition of done** — `pnpm dev` (or npm) serves the UI; `/api/*` proxies to the Go server; a logged-in-less flow can hit `/health` and `/api/games` (or a stub page that uses React Query); theme choice survives reload (server `frontend` setting); production path documented (`npm run build` + server static/embed); tray can open the UI URL (port aligned with `config.json` / Vite).

**Dependencies** — Go server on Phase 0 routes (`openapi.yaml`, CORS if needed for dev). Regenerate OpenAPI client when `server/openapi.yaml` changes (`go run ./cmd/openapi-gen` from `server/`, then regen TS client).

**Recommended sequence**

| Milestone | Focus | Outcome |
|-----------|--------|---------|
| **M1 — Bootstrap** | Vite + React + TS in `server/frontend/`; strict TS; path aliases (`@/`); `.gitignore` for `node_modules`, `dist`, env files | Clean `npm run dev` / `build` |
| **M2 — Styling & UI base** | Tailwind + PostCSS; shadcn/ui init (components live in repo, not `node_modules` only); global `index.css` | Buttons, layout primitives, accessible defaults |
| **M3 — Routing & data** | React Router (shell routes: `/`, `/library`, `/playable`, `/settings`, `/about` — placeholders OK); TanStack Query + shared `api` module; **Vite proxy** → `http://localhost:8900` (or env); OpenAPI-generated types/client or `openapi-typescript` + `fetch` wrapper | End-to-end one real API call from UI (e.g. game count or health) |
| **M4 — App shell** | Sidebar + topbar + outlet; Ctrl+K focuses search (can be noop); theme toggle wired to context; notification bell placeholder; responsive breakpoints; route-level **error boundary** + suspense-friendly loading | Matches “Shell & Layout” checklist |
| **M5 — Theme engine** | `MgaTheme` interface; React context; map theme → **CSS variables** on `:root` or scoped wrapper; load/save **active theme id** + optional overrides via `/api/config/frontend` (merge with localStorage fallback if API fails); theme picker with preview | Matches “Theme Engine” checklist |
| **M6 — Eleven themes** | Implement palettes + typography tokens per theme (Midnight default); respect `prefers-reduced-motion` and initial `prefers-color-scheme` (suggest default theme on first visit) | All rows under “All 11 Themes” |
| **M7 — Branding** | Logo asset pipeline (favicon, About, loading); optional tray icon refresh in Windows tray code when logo exists | Logo & Branding checklist |
| **M8 — Production & tray** | `go:embed` of `frontend/dist`; chi/static file handler + SPA fallback **excluding** `/api/*`; document single-binary workflow; tray “Open Web Frontend” uses same port as server config | Static file serving + Tray checklist |

**Parallelization** — M1→M2→M3 is mostly linear. M6 (themes) can start once M5’s token shape is stable (parallel: author palettes while shell is built). M7 can overlap M4–M6. M8 last (or behind a feature flag).

**Conventions (decide early)** — Package manager (pnpm recommended); ESLint + Prettier; commit hook optional; env: `VITE_API_PROXY_TARGET` for non-default backend port; keep generated API code in a dedicated folder (e.g. `src/api/generated/`) and do not hand-edit.

**Risks** — shadcn + Tailwind **major-version** pairing (pin versions in docs); OpenAPI drift (add CI or pre-commit: regen + diff); CORS only if you ever load UI from a different origin than API without proxy; `go:embed` path relative to server module root.

**SuitCode MCP (for implementation)** — Prefer **`*_by_path`** tools with an **absolute `repository_path`** to the repo root (no pre-opened Cursor workspace required). Verified on this project:
- **`repository_summary_by_path`** — component/test/package-manager counts + previews (e.g. **32** Go components, **20** test targets, **8** nested `go.mod` plugins).
- **`get_file_owner_by_path`** — maps a file → owning Go package/component (e.g. `server/internal/http/controllers.go` → `server/internal/http`).
- **`get_related_tests_by_path`** — related `go test` packages for a file (same example → `go test github.com/.../server/internal/http`).
- **`get_minimum_verified_change_set_by_path`** — minimal authoritative checks for a **Go-owned** file (same example → `go test -buildvcs=false …/internal/http` only). **Non-Go / unowned files** (e.g. `server/openapi.yaml`) may return *unknown repository file owner* — fall back to manual regen (`go run ./cmd/openapi-gen`) + `go test ./...`.
- **`get_repository_by_path`** still required **`workspace_id`** here and failed with *unknown workspace id* even after summary returned ids — treat as optional or use only when your SuitCode UI exposes a stable workspace id.

**Wishlist:** `get_minimum_verified_change_set_by_path` for **OpenAPI / YAML** owners; **Vitest** once `server/frontend` exists; align `get_repository_by_path` with ids returned from `repository_summary_by_path`.

---

### Project Setup
- [x] Initialize Vite + React + TypeScript in [`server/frontend/`](server/frontend/)
- [x] Tailwind CSS + PostCSS configuration
- [x] shadcn-style foundation — [`cn()`](server/frontend/src/lib/utils.ts), [`Button`](server/frontend/src/components/ui/button.tsx) + CVA (full shadcn CLI optional later)
- [x] React Router (client-side routing)
- [x] TanStack Query (API data fetching + caching)
- [ ] Auto-generated API client from `openapi.yaml` (minimal hand types in [`client.ts`](server/frontend/src/api/client.ts) for now)
- [x] Vite dev proxy to Go server ([`vite.config.ts`](server/frontend/vite.config.ts), default `8900`, override `VITE_API_PROXY_TARGET`)
- [x] `server/frontend/node_modules/` and `server/frontend/dist/` in [`.gitignore`](../.gitignore)

### Static file serving (production binary)
*Runtime static files from `FRONTEND_DIST` (default `./frontend/dist`); `build.ps1` copies dist into `bin/frontend/dist`. `go:embed` deferred.*

- [ ] `go:embed` frontend dist into server binary *(optional hardening)*
- [x] Serve static assets + SPA fallback via [`MountSPA`](server/internal/http/spa.go) (`/*` after `/api`, `/health`)
- [x] SPA fallback: non-file routes → `index.html`

### Tray icon
*Windows tray opens the **same** URL as the HTTP server (`PORT` from config).*

- [x] **Open Web Frontend** menu item → default browser ([`tray_windows.go`](server/cmd/server/tray_windows.go))

### Shell & Layout
- [x] App shell: sidebar + topbar + main ([`AppLayout.tsx`](server/frontend/src/layouts/AppLayout.tsx))
- [x] Sidebar: Home, Library, Playable, Settings, About
- [x] Topbar: search (Ctrl+K focus), theme `<select>`, notification placeholder
- [x] Responsive: sidebar `md:` breakpoint; mobile stacks (narrow sidebar hidden — refine later)
- [x] Error boundary ([`ErrorBoundary.tsx`](server/frontend/src/components/ErrorBoundary.tsx)); React Query loading on Home

### Theme Engine
- [x] Theme tokens as CSS variables + TypeScript ids ([`presets.ts`](server/frontend/src/theme/presets.ts)); full `MgaTheme` interface can extend later
- [x] [`ThemeProvider`](server/frontend/src/theme/ThemeProvider.tsx)
- [x] CSS custom properties on `document.documentElement`
- [x] Persistence: `GET`/`POST /api/config/frontend` + `localStorage`
- [x] Theme selector (top bar); live preview = immediate apply
- [x] `prefers-reduced-motion` class; initial theme from `prefers-color-scheme` when no saved theme

### All 11 Themes
- [x] **Midnight** — default dark, charcoal + blue accent, clean professional
- [x] **Daylight** — classic light, white canvas + soft shadows, airy modern
- [x] **Deep Blue** — blue-gray + teal highlights, rounded cards, PC gamer feel
- [x] **Obsidian** — true black + vivid green, frosted glass, console-premium
- [x] **Curator** — purple-slate + warm gold, compact density, power user
- [x] **Big Screen** — deep navy + electric blue glow, oversized cards, TV/gamepad friendly
- [x] **Retro Terminal** — phosphor green/amber, monospace font, pixel-art sensibility, no gimmicky overlays
- [x] **Synthwave** — purple gradient + hot pink/cyan, neon glow, gradient accents, bold
- [x] **Cinema** — pure black + warm gold, ultra-high contrast, minimal chrome, OLED-perfect
- [x] **Frost** — Nord palette, arctic blue-gray, muted aurora accents, calm sophisticated
- [x] **Neon Arcade** — 80s arcade, bright neon palette, chunky display font, uppercase, pixel grid pattern

### Logo & Branding
- [x] [`README.md`](README.md) brand brief (sizes, paths, tray ICO notes)
- [x] **Favicon:** [`server/frontend/public/favicon.ico`](server/frontend/public/favicon.ico) + `<link rel="icon" href="/favicon.ico">` in [`index.html`](server/frontend/index.html)
- [x] **Logo / title art:** [`server/frontend/public/logo.png`](server/frontend/public/logo.png), [`server/frontend/public/title.png`](server/frontend/public/title.png) — shell, Home hero, About (see README)
- [x] **Windows `mga.ico`:** multi-size ICO at [`server/cmd/server/mga.ico`](server/cmd/server/mga.ico) — **system tray** via `//go:embed` in [`tray_windows.go`](server/cmd/server/tray_windows.go); **File Explorer `.exe` icon** via COFF `rsrc_windows_${GOARCH}.syso` generated by [`build.ps1`](server/build.ps1) (or `go generate ./cmd/server` after editing the ICO)

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
- [ ] **"xCloud" badge** (cloud-playable Xbox games) — backend already exposes `xcloud_available`, `xcloud_url`, `store_product_id`, `is_game_pass` on `GET /api/games`, `GET /api/games/{id}`, and [`GET /api/games/{id}/detail`](server/internal/http/game_detail.go)
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
- [ ] Scan progress display (driven by SSE — backend ready in Phase 0)
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
- [ ] **`GET /api/games/{id}/play`** — Stream a **launchable file** to the browser emulator (query param: `path` or `file_id` matching [`GameFile`](server/internal/core/entities.go)). **Phase A:** local disk / paths resolvable on the machine running the server (`http.ServeFile` or `io.Copy` with **Range** support for large ROMs). **Phase B:** remote sources (SMB / Drive) via plugin IPC (e.g. `source.file.read` with range) or server-side FS abstraction. **Security:** reject path traversal; only files that belong to the game’s source games. OpenAPI + CORS/range behavior as needed for EmulatorJS / WASM loaders.
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
- [ ] TGDB disabled due to low API quota
- [ ] Media download background worker (MediaItems have URLs but no local files yet)
- [ ] Schema migration strategy (deferred until after first release)
