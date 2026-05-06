# MyGamesAnywhere — Roadmap

## Current Status — 2026-05-01
- Server startup now requires a complete config and supports `LISTEN_IP`, with released portable ZIP defaults kept local-only on `127.0.0.1:8900`; generated local URLs stay loopback even when the bind address is `0.0.0.0`.
- Manual metadata search is now platform-aware and fuzzy: title cleanup, N64/platform filtering, broader ranked provider results, and concurrent provider lookup make cases like `pokemon stadium 2 (u) [!]` repairable from Undetected Games.
- Manual metadata result descriptions are compact by default and expand/collapse on click, keeping long provider summaries from dominating the review page.
- Undetected Games now supports deleting candidate files after a plugin-backed dry-delete preview, instead of only archiving a false positive as "not a game".
- Filesystem-backed real deletes are safer: Google Drive moves explicit files to Drive trash, SMB deletes only explicit file targets, directory entries are rejected, and the UI requires a final checkbox before the real delete action is enabled.
- Working tree now includes the April 15 session changes on top of `19cd0dc` (`dosbox fix`): active Undetected Games candidates are hidden from Library / Play-facing surfaces and library-facing counts.
- Browser play now has automated end-to-end proof via `tools/browser-play`, including EmulatorJS/js-dos/ScummVM save import/export coverage, bundle-backed js-dos, explicit unsupported-state fast-fail for plain-file js-dos launches, ambiguous source selection, invalid remembered source handling, and same-title/different-source-record labeling.
- Filesystem-backed SMB/Google Drive integrations use `include_paths[]` with per-include `recursive`, source-identity duplicate detection, and scope-aware final-scan cleanup.
- Undetected Games manual apply now reuses the shared refresh/persist/media-enqueue path used by metadata refresh flows, preserving manual-selection precedence while avoiding a second enrichment pipeline.
- Metadata execution now uses a selective fail-fast policy: full scans remain tolerant but emit degraded scan metadata state, while metadata-only refresh, per-game refresh, and manual review fail fast on provider errors.
- Game detail now supports source-record-scoped hard delete for eligible filesystem-backed SMB/Google Drive source records, with strict confirmation, provider-backed destructive file deletion, source-row cleanup, and canonical recomputation.
- Media URL refs are now downloaded by a background worker into `MEDIA_ROOT` from existing pending rows at startup and after scan/manual-review persistence.
- HLTB-hosted cover downloads now use browser-style request headers in the media worker so `howlongtobeat.com/games/*.jpg` assets no longer fail with `403` during background caching.
- Shared frontend media selection is now centralized, so Library / Play cards and the game detail page agree on cover fallback behavior when a game has screenshots/artwork but no explicit `cover` media row.
- The Settings → Undetected Games reclassify path now normalizes nullable candidate fields before rendering, so direct `Reclassify` deep-links from the game page no longer crash on `null` arrays/text fields.
- Game detail now exposes `Refresh Metadata & Media`, backed by a shared game-scoped metadata refresh path that persists refreshed matches/media refs before enqueueing background media downloads.
- Frontend build vulnerabilities are addressed by the Vite 7 toolchain upgrade and lockfile refresh; `npm audit` is clean again.
- Shared title normalization now strips common dump/region suffix noise such as trailing `(...)` and `[...]`, with regression coverage for cases like `aladdin (u) [!]`.
- TGDB has been removed from plugin/runtime discovery and product-facing branding/about references.
- Scan preparation and metadata-provider fetching now run with bounded concurrency, while persistence remains serialized on the shared scan path to avoid partial-write races and SQLite lock churn.

## Tasks

These are the next committed tasks after the completed Phase 7 / issue-cleanup work. Execute in this order unless a release blocker appears.

1. **Configurable bind host / LAN access**
   - [x] Add a server config key for bind host/address, defaulting to loopback (`127.0.0.1`) for the current local-only behavior.
   - [x] Support explicit LAN binding (`0.0.0.0` or a concrete interface IP) without changing the existing `PORT` contract.
   - [x] Keep local tray/open-browser and OAuth callback URLs on loopback unless a separate public/base URL setting is intentionally introduced.
   - [x] Ship released portable ZIP config with `PORT: "8900"` and `LISTEN_IP: "127.0.0.1"`.
   - [ ] Add a Settings → General surface for host/port visibility and bind-host selection, with a clear restart-required state if live rebinding is not implemented.
   - [ ] Decide installer/service/firewall behavior for admin-required LAN exposure scenarios.
   - [ ] Verification completed for server config tests, HTTP server address tests, focused Go tests, frontend build, and portable packaging config checks.

2. **Auto-update check and release manifest**
   - [x] Extend the release pipeline from artifact-only output to a versioned update source: GitHub Release assets or a static update manifest with version, URL, SHA256, and release notes URL.
   - [x] Add a server-side update checker that compares current `VERSION` with the latest available release and exposes update status/check/download/apply APIs.
   - [x] Add an About UI card showing current version, latest version, release notes, and explicit update actions.
   - [x] Keep portable updates conservative: download/verify the package and guide restart/replacement instead of attempting in-place replacement of the running executable.
   - [ ] Verification: manifest parser tests, version-compare tests, checksum failure test, frontend build.

3. **Achievements refresh instead of cached-only confusion**
   - [ ] Keep `/api/achievements` and `/api/achievements/explorer` read-only over cached rows for fast page load.
   - [ ] Add an explicit `Refresh Achievements` job for all eligible games, reusing `AchievementFetchService` and existing achievement-capable integrations.
   - [ ] Show refresh progress and last refreshed state in the Achievements page so "cached" means "last fetched", not "incomplete by design".
   - [ ] Persist provider failures per game/source and surface degraded state without blocking unrelated games.
   - [ ] Verification: achievement service/controller tests, SSE job progress tests, frontend build, focused manual refresh proof with one configured provider.

4. **Duplicate games review in Settings**
   - [ ] Add a Settings tab for duplicate candidates across source records.
   - [ ] Support two modes: title-normalized duplicates ignoring platform/version, and stricter duplicates including platform/version/source metadata.
   - [ ] Reuse existing canonical/source-game link data and shared title normalization instead of adding a separate duplicate model first.
   - [ ] Provide review-only grouping in v1; defer destructive merge/split actions unless the report proves useful.
   - [ ] Verification: duplicate-query tests with same-title/different-platform cases, frontend build, manual UI proof on seeded data.

5. **Richer Library / Gamer statistics**
   - [ ] Split Home stats conceptually into Library Statistics and Gamer Statistics while keeping the Home page as a concise summary.
   - [ ] Library statistics should extend the current `/api/stats` surface: platform, source, decade, genre, metadata/media coverage, duplicates, and scan activity.
   - [ ] Gamer statistics should build on the achievements dashboard and play history: achievement progress, points, recently played, favorites, completion-style summaries where data exists.
   - [ ] Verification: stats aggregation tests, chart-empty-state proof, frontend build.

Deferred until after the above: multi-user/profile support. It is a larger architecture change because profiles need ownership boundaries for integrations, games, saves, settings, achievement cache, scan jobs, and admin-only Settings access.

## Completed

### Server Core
- [x] Plugin architecture (IPC, length-prefixed JSON over stdin/stdout)
- [x] Game source plugins: SMB, Steam, Xbox, Epic, Google Drive
- [x] Metadata resolver plugins: IGDB, RAWG, Steam, GOG, LaunchBox, MAME DAT, HLTB, RetroAchievements
- [x] 3-phase metadata orchestrator (Identify → Consensus → Fill)
- [x] Scanner pipeline (file detection, platform detection, title normalization, role classification, grouping)
- [x] Achievement support (Steam, Xbox, RetroAchievements — on-demand)
- [x] MediaItem model (URL, type, source, local cache metadata)
- [x] Background media download worker (`media_assets` startup sweep + post-persist enqueue into `MEDIA_ROOT`)
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
- [x] **`GET /api/stats`** — `GameStore.GetLibraryStats`: single JSON (`LibraryStats` in [`core/entities.go`](server/internal/core/entities.go)); includes `by_metadata_plugin_id` for per-provider enrichment counts.
- [x] **`GET /api/integrations/{id}/games`** — Canonical games discovered by a source integration (lightweight `GameListItem[]`).
- [x] **`GET /api/integrations/{id}/enriched-games`** — Canonical games enriched by a metadata provider's plugin (resolves integration → plugin_id → `metadata_resolver_matches`).
- [x] **Frontend preferences** — **`GET` / `POST /api/config/frontend`** only for SPA prefs; settings key **`frontend`**; max body 256KiB.
- [x] **`POST /api/scan` `metadata_only` flag** — When `true`, skips source discovery and re-enriches existing visible/detected source games via `RunMetadataRefresh`. Loads the DB-backed visible source set, groups by integration, runs 3-phase metadata pipeline, re-persists.
- [x] **`GET /api/scan/reports`** — Returns last N scan reports (newest first); each includes diff summary (games added/removed), per-integration breakdown, duration. Stored in `scan_reports` table as JSON.
- [x] **`GET /api/scan/reports/{id}`** — Single scan report by ID.
- [x] **Cascade delete on source integration removal** — `DeleteGamesByIntegrationID` removes all child rows (achievements, achievement_sets, source_game_media, metadata_resolver_matches, game_files, canonical_source_games_link, source_games) in a single transaction. Called from `DeleteIntegration` handler for source-capability plugins only.

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
- [x] **API:** [`GET /api/games/{id}/detail`](server/internal/http/game_detail.go) exposes unified Xbox/xCloud fields plus per-source **`resolver_matches`**. **`GET /api/games`** (paginated) and **`GET /api/games/{id}`** return the same full detail shape; **`POST /api/scan`** still returns lightweight summaries for a smaller payload.
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
- [x] Generated frontend API contracts now live under [`server/frontend/src/api/generated/`](server/frontend/src/api/generated/) while the fetch facade in [`client.ts`](server/frontend/src/api/client.ts) remains hand-maintained for now
- [x] Vite dev proxy to Go server ([`vite.config.ts`](server/frontend/vite.config.ts), default `8900`, override `VITE_API_PROXY_TARGET`)
- [x] `server/frontend/node_modules/` and `server/frontend/dist/` in [`.gitignore`](../.gitignore)

### Static file serving (production binary)
*Runtime static files from `FRONTEND_DIST` (default `./frontend/dist`); `build.ps1` copies dist into `bin/frontend/dist`. `go:embed` deferred.*

- [x] Serve static assets + SPA fallback via [`MountSPA`](server/internal/http/spa.go) (`/*` after `/api`, `/health`)
- [x] SPA fallback: non-file routes → `index.html`

### Tray icon
*Windows tray opens the **same** URL as the HTTP server (`PORT` from config).*

- [x] **Open Web Frontend** menu item → default browser ([`tray_windows.go`](server/cmd/server/tray_windows.go))

### Shell & Layout
- [x] ~~App shell: sidebar + topbar + main~~ *(replaced by top-tab layout in Phase 2 rework)*
- [x] ~~Sidebar: Home, Library, Playable, Settings, About~~ *(replaced by horizontal tabs)*
- [x] Topbar: search (Ctrl+K focus), theme `<select>`, notification placeholder
- [x] ~~Responsive: sidebar `md:` breakpoint~~ *(rework needed for tab layout)*
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

## Phase 2 — UI Rework & Game Library

### App Shell Rework
*Current stage: keep the header/topbar, use a persistent desktop playable-games sidebar alongside the shell pages, and keep mobile/tablet on the top-nav fallback until a dedicated mobile drawer exists.*

- [x] **Top tab bar:** Logo + horizontal tabs (Home, Play, Library, Settings, About) + search (Ctrl+K) + theme selector
- [x] Remove the original full-height app-navigation sidebar from the first shell iteration
- [x] Game detail route (`/game/:id`) renders **outside** the tab layout (back button replaces tabs)
- [x] Responsive: tabs collapse on narrow viewports (hamburger or scrollable tab strip)
- [x] **Persistent desktop playable sidebar:** left rail stays visible across the shell pages and launches actionable games without duplicating app navigation
- [x] **Desktop tab simplification:** top-level `Play` tab is hidden in desktop layouts where the playable sidebar is present; mobile/tablet keeps the top-nav fallback for now
- [x] **Sidebar affordances:** desktop sidebar includes a quick filter input and per-platform collapsible groups whose last state is remembered; groups default collapsed on first load
- [x] **Recent Played launcher section:** render only when real recent-play data exists; do not synthesize it from scan/title order

### Navigation & Pages

| Tab | Route | Content |
|-----|-------|---------|
| Home | `/` | Dashboard / hero — keep current for now |
| Play | `/play` | Actionable games route retained, but desktop-primary access to playable titles is via the persistent sidebar |
| Library | `/library` | All games in the collection |
| Settings | `/settings` | Integrations, plugins, appearance (unchanged) |
| About | `/about` | Credits, attributions |

### Shelf-Based Browsing
*Both Play and Library use the same shelf section UI. Each page has its own persisted configuration.*

- [x] **Default view:** shelf sections, each showing one row of game cards
- [x] **Library add-shelf affordance:** `Library` shelf view uses a bottom `+` button after the last shelf; it opens the existing picker flow for Platform, Genre, Developer, Publisher, Source, and Year shelves
- [x] **"All Games" section** — special ungrouped option showing every game; serves as default/fallback
- [x] **Remove section** (X button on each shelf header); if none remain → auto-fallback to "All Games"
- [x] **Single-row preview:** each collapsed shelf shows a fixed preview row sized to avoid horizontal scrolling
- [x] **Overflow affordance:** when more games exist than fit in the preview row, the last visible tile is a centered `...` indicator that expands the shelf
- [x] **Expand/collapse:** clicking the shelf header or overflow tile reveals the full shelf contents; expanded shelves expose a `Collapse` action
- [x] **Single-expand policy:** expanding one shelf collapses any other expanded shelf
- [x] **Empty sections hidden:** sections with 0 matching games are not rendered
- [x] **Global search** (top bar) filters within all visible shelves in real-time
- [x] **View toggle:** Shelf view (default) vs Grid view (flat full-collection grid)
- [x] **Persist configuration** per page (Play vs Library) in `FrontendConfig` (server + localStorage)
- [x] **Library CTA behavior:** `Add Shelf` is not shown in `Library` grid view
- [x] **Visual refresh:** borderless shelf presentation, fixed-height image-first cards, branded source/platform badges in Library/Play surfaces

### Game Cards (carried from prior work)
- [x] Cover art with lazy loading and placeholder
- [x] Platform icon badge (Steam, GBA, PS1, Arcade, ScummVM, DOS, etc.)
- [x] Source badge (which integration found it)
- [x] Achievement progress ring (if available) — rendered from cached achievement summaries when present
- [x] HLTB time estimate badge
- [x] Metadata confidence indicator (number of resolvers matched)
- [x] "Playable" badge (browser-emulatable platforms)
- [x] **"xCloud" badge** (cloud-playable Xbox games)
- [x] Play button on playable games vs. "View" on others
- [x] Detached Netflix-style hover cards with 16:9 expanded overlays that do not push shelves/layout
- [x] Hover-card media area opens the game details page directly
- [x] Hover-card open/close animation with staged media/tray reveal
- [x] Hover-media selection now respects explicit hover override before fallback media selection

### Search
- [x] Full-text search with fuzzy matching
- [x] Keyboard shortcut (Ctrl+K) to focus search
- [x] Sort by: title, release date, recently added, HLTB time, platform

---

## Phase 3 — Game Detail Page

*Full-page route (`/game/:id`) rendered **outside** the tab layout. Back button returns to the originating page (Play or Library) with scroll position preserved. Design inspired by Steam / Xbox game pages.*

### Navigation
- [x] Route `/game/:id` with dedicated layout (no tab bar)
- [x] Back button ("< Library" / "< Play") preserving scroll position on return

### Metadata Display
- [x] Full-bleed hero/banner treatment with Steam/Xbox-style cinematic composition
- [x] Title, description, release date, developer, publisher, genres, rating
- [x] Source-aware attribution for description and metadata facts when resolver data is reliable
- [x] Full per-field/logo attribution coverage for every metadata provider
- [x] Unified `Metadata gathered from ...` attribution row for metadata/media providers
- [x] Hero background override with suitability-aware rendering fallback for non-cinematic images
- [x] Legacy lazy backfill/persist of explicit cover / hover / background override selections for older games

### Media Gallery
- [x] Screenshot viewer (lightbox)
- [x] Non-image media surfaced separately from the screenshot gallery
- [x] Video embeds when the current media URL is browser-renderable
- [x] Manuals / documents surfaced with view/open actions
- [x] Media source attribution
- [x] Rich inline handling for every supported video/document format
- [x] Dedicated game media page with source/type filters and `Open Gallery` entry from game detail
- [x] Representative featured-media rail on game detail with video-aware type-first ordering and dedupe
- [x] Cover / hover / background selection actions from the media gallery
- [x] Image-dimension probing and persistence for media with missing width/height metadata
- [x] Background suitability warnings for gallery selection, including warning affordance and confirmation flow

### External Links
- [x] Branded external links for known providers when `external_ids` include URLs
- [x] Complete icon coverage for every external link provider

### Achievements
- [x] Achievement list with icons, descriptions, and per-source groupings
- [x] Source attribution (RetroAchievements, Steam, Xbox, etc. when source is known)
- [x] Overall progress bar
- [x] Achievement unlocked-state normalization and cached summary aggregation (mixed states preserved; `unlocked_count` and `unlocked_at` stay semantically correct)

### Completion Times
- [x] HLTB main story / completionist / combined display
- [x] HLTB attribution

### Play
- [x] "Play in Browser" button for supported platforms
- [x] "Play on xCloud" button → opens xCloud URL
- [x] Source info: file paths, integration details, resolver match data

---

## Phase 4 — Settings & Admin

*Settings consolidated from 5 tabs (Integrations, Plugins, Scanning, Sync, Appearance) to 3 tabs (Integrations, Plugins, Appearance). Scanning and Sync controls merged into their respective integration cards.*

### Integrations
- [x] List active integrations with status indicators
- [x] Add new integration (guided wizard: category → plugin → config → label)
- [x] Remove integration
- [x] Edit integration config (with secret masking)
- [x] Grouped by capability (Game Sources, Metadata Providers, Achievements, Sync) with collapsible accordion
- [x] Per-integration and "Check All" status checks (SSE-driven real-time updates)
- [x] Duplicate integration detection (409 with existing label)
- [x] Library stats summary at top of Integrations tab
- [x] Save Sync integrations grouped separately with active integration selection and server-driven migration actions

### Integrations — Game Sources
- [x] Per-source game count badge (from `LibraryStats.by_integration_id`)
- [x] Per-source "Scan" button with inline progress bar
- [x] "Scan All Sources" button on accordion header
- [x] "Rescan All" naming and total scan progress bar for global source rescans
- [x] Expandable card: scrollable games list (lazy-loaded via `GET /api/integrations/{id}/games`)
- [x] SSE-driven scan progress per integration

### Integrations — Metadata Providers
- [x] Per-provider enrichment count badge (from `LibraryStats.by_metadata_plugin_id`)
- [x] "Refresh Metadata" button on accordion header (metadata-only refresh, no re-discovery)
- [x] Expandable card: scrollable enriched games list (lazy-loaded via `GET /api/integrations/{id}/enriched-games`)
- [x] Per-integration `Refresh` action for provider-scoped known-game derived refresh jobs (metadata/media/achievements as supported)

### Integrations — Sync
- [x] Push / Pull buttons on sync integration card
- [x] Encryption passphrase management (store key / clear key) in expandable section
- [x] Last push/pull timestamps displayed
- [x] Push confirmation dialog
- [x] SSE-driven sync operation status

### Integrations — Scan Summary & History
- [x] Persistent scan reports stored server-side (`scan_reports` table, JSON blob)
- [x] Pre/post game count diff computed by orchestrator (games added/removed per integration)
- [x] `ScanSummary` component: most recent scan at a glance (+N added, -N removed, duration)
- [x] Expandable per-integration breakdown in report cards
- [x] "View history" toggle to browse last N scans
- [x] SSE event name alignment: frontend uses correct backend event names (`scan_complete`, `scan_integration_complete`, `scan_metadata_phase`, etc.)
- [x] Rich scan progress UI: deterministic progress bar (completed/total integrations), detailed status text from all scan pipeline events
- [x] Auto-check integration status on page load (status dots green/red instead of gray)
- [x] Cascade delete: removing a source integration deletes its source games and orphaned canonical entries

### Plugins
- [x] Discovered plugins list with version, capabilities, enabled/disabled status
- [x] Responsive multi-column grid (1/2/3 columns at sm/md/lg breakpoints)

### Appearance
- [x] Visual theme previews (thumbnails or live mini-preview)
- [x] One-click theme switching with instant feedback
- [x] Date format setting (d/M/yyyy or M/d/yyyy) with live preview
- [x] Time format setting (12-hour or 24-hour) with live preview
- [x] Format preferences persisted via `FrontendConfig` (server + localStorage)

---

## Phase 5 — Real-Time Updates

- [x] SSE client integration (auto-reconnect, event parsing)
- [x] Scan progress bar (global, in Integrations tab with deterministic progress + status text)
- [x] Toast notification system (non-blocking)
- [x] Notification types: scan complete, scan error, integration status change, sync complete

### Scan Progress Improvements
*Current progress UI feels stuck during long metadata phases. Add granular per-game events and a visible event log.*

- [x] **Backend:** `scan_metadata_game_progress` SSE event — `{plugin_id, game_index, game_count, game_title}` emitted after metadata resolver batch responses are processed, with one progress step per game in the batch (client-agnostic, any consumer can use)
- [x] **Frontend:** Rolling event log below progress bar — last 3–5 events as a mini-timeline with timestamps
- [x] **Frontend:** Per-game status text during metadata enrichment (e.g. "IGDB: 15/200 — Portal 2")

---

## Phase 6 — In-Browser Emulation

### Generic Server Contract
- [x] Shared browser-play truth derived from supported platform plus launchable source-file metadata
- [x] Generic `play` metadata in [`GET /api/games/{id}/detail`](server/internal/http/game_detail.go) / [`GET /api/games/{id}`](server/internal/http/game_detail.go)
- [x] [`GET /api/games/{id}/play`](server/internal/http/play_controller.go) streams owned files by `file_id` with Range support and ownership/path validation
- [x] No raw-path selector in the public browser-play API contract

### Web Client Runtime Tracks
- [x] Dedicated browser-player route: `/game/:id/play`
- [x] Self-hosted EmulatorJS integration for the conservative browser-strong console set: NES, SNES, GB/GBC, GBA, Genesis-family, PS1, Arcade/MAME
- [x] Self-hosted js-dos integration for MS-DOS
- [x] ScummVM WASM target assembly from generic file metadata
- [x] xCloud stays external; no embedded iframe plan in Phase 6

### Player UI
- [x] Dedicated player shell outside the main tab layout
- [x] Fullscreen-capable browser player route; ScummVM uses parent-level fullscreen because the runtime has no internal fullscreen affordance
- [x] Exit path back to the game detail page
- [x] Explicit browser save `Load` / `Save` UI with remote slots (`autosave`, `slot-1` … `slot-5`)
- [x] Browser-local save persistence where the selected runtime already supports it
- [x] EmulatorJS player sizing fixed via definite-height player shell and absolute iframe/runtime sizing instead of relying on an indefinite `min-h-screen` percentage-height chain

### Runtime Wiring
- [x] Client-owned platform-to-runtime/core mapping
- [x] Self-hosted runtime assets under the frontend app stack
- [x] DOS launch preparation performed in the web client from generic file metadata; plain-file sessions now mount relative to the launch root and use a persistent `.exe`/`.com`/`.bat` picker with Apply + restart confirmation
- [x] ScummVM launch preparation performed in the web client from generic file metadata; runtime now infers/preloads engine plugins, preloads runtime data, uses an HTTP-backed filesystem, hides the ready status pill, and keeps the canvas height-driven at 4:3
- [x] Runtime save bridge for EmulatorJS, js-dos, and ScummVM via `postMessage`
- [x] Standalone ScummVM harness exists under `tools/scummvm-harness` and was used to prove non-MGA runtime launch paths before applying the runtime fixes to MGA

### Playability Corrections
- [x] Browser play truth tightened to require launchable source metadata, not platform heuristics alone
- [x] Nintendo DS no longer falls back to GBA browser play detection
- [x] ScummVM launchability requires recognized self-contained game signatures, not merely a non-empty folder

### xCloud
- [x] xCloud launch remains an external link to `xbox.com/play/launch/{titleId}`

---

## Phase 6.5 — Save Sync

- [x] Generic server-first save-sync contract for browser runtimes (opaque files + manifest metadata, no MGA save-state abstraction)
- [x] Explicit `Load` / `Save` flow only; no auto-sync
- [x] Active save-sync integration stored in frontend config and selectable from Settings
- [x] Save slot identity keyed by canonical game, source game, runtime, and slot id
- [x] Browser player exports/imports whole runtime save snapshots
- [x] Save conflict handling: last-writer-wins with warning
- [x] Save-sync migration jobs (`all` or `game`) run server-side with SSE progress
- [x] Save-sync integration: `save-sync-google-drive`
- [x] Save-sync integration: `save-sync-local-disk`

---

## Phase 7 — Polish & Advanced Features

### Animations & Motion
- [x] Page transition animations
- [x] Staggered grid item entrance
- [x] Smooth filter/sort transitions
- [x] Expand skeleton/loading coverage beyond the Home and library surfaces that already have loading placeholders
- [x] Cover art sizing tightened across cards, rows, and sidebar thumbnails while keeping contain-based fallbacks
- [x] The shelf overflow affordance now sits as a slim right-edge expander instead of reading like a regular card, and collapsed shelves stay visually single-line
- [x] "matches" count stated for each game (on the card) should be only source matches.
- [x] Game page need to be redesigned. should be much more pretty.
- [x] Fix scanning/refetching to show detailed progress (or/and least a progress bar). Async scan jobs now return immediately, publish `job_id` over SSE, and the Settings UI can recover progress after reload.
- [x] Browser-play route now shows explicit runtime/session diagnostics and uses skeleton loading instead of abrupt blank states

### display
- [x] Remaining visible platform labels now route through shared display helpers so raw ids like `xbox_series` do not leak into the UI
- [x] GBA now uses a compact in-app mark instead of the old wide wordmark/title-style asset

### Game page improvements
- [x] Show all relevant game files (installer files / ROM / directory, whatever is relevant)
- [x] Reclassify a game option, goes through the same mechanism as in `Undetected Games tab under settings` section.

### Dashboard / Stats
- [x] Games by platform (chart)
- [x] Games by decade
- [x] Top genres
- [x] Metadata coverage (% with descriptions, cover art, achievements)
- [x] Recent scan activity

### play sidebar
- [x] Recent Played in the sidebar supports removing individual entries


### scanning log in settings page - integrations tab
- [x] if the last line is displayed (scroller at the bottom), the scorller should auto-scroll.

### About Page
- [x] MGA version, build date, author credits
- [x] "Powered By" grid with logos and one-liner descriptions for currently integrated services:
  IGDB, RAWG, Steam, GOG, LaunchBox, MAME, HowLongToBeat, RetroAchievements,
  ScummVM, Xbox/xCloud, Epic Games, Google Drive, SMB
- [x] Extend "Powered By" coverage for future emulation/runtime services:
  EmulatorJS/RetroArch, js-dos/DOSBox
- [x] "View Open Source Licenses" link
- [x] In-context attribution throughout the app (service logos next to their data)
- [x] About page icon/vendor credits preserve source locations for shipped brand assets

### Home Page
- [x] Should contains statistics using graphs/diagrams (games, achivements, etc)

### Undetected Games tab under settings
- [x] Read-only Undetected Games review inventory exists in Settings, and `Reclassify` deep-links into a specific review candidate.
- [x] Read-only metadata search exists for review candidates: MGA can query relevant configured metadata providers, show normalized results, and let the user refine the search.
- [x] Manual metadata search now runs provider lookups concurrently and returns broader ranked fuzzy results, with platform-compatible matches first.
- [x] Auto/manual metadata lookup now shares title variants such as raw and normalized titles, and uses platform hints when available without loosening auto-identify safety.
- [x] Manual metadata result descriptions truncate long summaries and expand/collapse on click.
- [x] A page that displays the undetected games. User should be able to either detect the game or mark as not a game
- [x] To detect the game, the user can enter the game name (or a substring of it), MGA would use all the RELEVANT metadata providers to find the closest matches and display them to the user with some details. The user can select one of them or refine the search.
- [x] If a game was selected by a user, MGA applies the manual match, runs configured metadata lookup/fill, persists resolver matches, and stores media refs returned by metadata providers.
- [x] If a game is selected as "not a game" it is not displayed, but placed in "not games archive"
- [x] The screen should have a button of no games archives where the user can "unarchive" if a "not a game" was misclassified
- [x] File-backed false positives can be cleaned up through a delete-candidate-files flow that first asks the source plugin for a dry-delete preview.


# Issues
- [x] Steam source scans now fail explicitly with configuration/auth-required status instead of silently returning an empty list, and the scan continues past that integration
- [x] Unknown-platform installer titles now fall back to Windows LaunchBox matching (e.g. "Plasma Pong", "I Am Fish")
- [x] HLTB lookup now uses the current live auth/init + `/api/find` flow, with legacy fallback and clearer diagnostics on provider failures
- [x] HLTB image downloads now use browser-style `User-Agent` / `Referer` headers in the background media worker so direct `howlongtobeat.com/games/*.jpg` fetches do not fail with `403`
- [x] RetroAchievements integration config already requires `username` in the plugin manifest/code and the settings UI schema-driven forms expose/persist it
- [x] remove TGDB - it has too low API quota
- [x] Media download background worker (pending `media_assets` rows are swept at startup and enqueued again after scan/manual-review persistence; successful downloads write relative `local_path` + `hash` under `MEDIA_ROOT`)
- Cancelled: keep the generic `rating` field. Steam/RAWG already populate it from Metacritic-style scores where available, and no separate first-class Metacritic field is planned right now.
- [x] Fix npm build vulnerabilities detected during build
- [x] Concurrency on scanning/metadata-fetching
- [x] Browser-play launch proof: EmulatorJS launches with corrected full-parent sizing after the definite-height shell/absolute iframe fix
- [x] Browser-play launch proof: js-dos launches plain-file games from the corrected launch-root-relative mount and executable selection flow
- [x] Browser-play launch proof: ScummVM launches through the generic file-list runtime with plugin preload and HTTP-backed FS
- [x] Browser-play manual proof: save import/export across EmulatorJS, js-dos, and ScummVM where supported
  - 2026-04-15 session result: not executable in-session. There was no browser binary/automation path available, the repo does not bundle EmulatorJS/js-dos save-proof fixtures, and `tools/scummvm-harness` still requires an external ScummVM game tree.
  - 2026-04-17 follow-up: automated end-to-end proof now runs from `tools/browser-play/proof-runner.mjs` via `npm run proof:e2e` against the built frontend and real runtime bridge.
  - 2026-04-20 follow-up: the dedicated proof runner now also covers source ambiguity, invalid remembered source rejection, same-title/different-source-record labels, and the js-dos plain-file unsupported save-sync fast-fail path. `server/frontend` build remains build-only validation; browser-play acceptance depends on `tools/browser-play`.
- [x] Selective metadata fail-fast policy: full scans continue past source/provider failures while surfacing degraded/error state, and metadata-only refresh, per-game refresh, and manual-review persistence abort on provider failure through a shared metadata policy/coordinator path.
- [x] Playable-games sidebar now uses compact platform icon rendering without duplicating text-style badges like the in-app GBA mark
- [x] The playable games sidebar now has its own desktop-only scrolling container, separate from the main page scroller, with a theme-colored thumb and transparent track.
- [x] EmulatorJS expands to the whole parent player component after repairing the definite-height layout chain and absolute iframe/runtime sizing.
- [x] Unknown/undetected games are no longer shown in "Library"; active Undetected Games candidates are hidden from Library / Play-facing surfaces and library-facing counts until they are matched.
- [x] Google Drive source integrations now scope traversal to the selected `root_path` for new scans.
- [x] Filesystem-backed scope edits no longer leave out-of-scope rows as long-lived `not_found` trash; the final successful scan persist hard-deletes rows outside the configured filesystem scope and soft-deletes truly missing in-scope rows.
- [x] Filesystem-backed scope changes support safe hard cleanup of source-owned rows that are no longer covered by the configured scan scope.
- [x] Reworked filesystem-backed source config away from "many integrations to the same backend connection" toward one backend connection with explicit scan scopes:
  - include paths
  - recursive flag per include
  - no full exclude rule engine in v1
- Cancelled: exclude rules remain intentionally deferred until there is a concrete need after multi-include. If they are introduced later, prefer a normalized path-glob design over a single ad hoc `exclude_glob` string so behavior stays deterministic across SMB/local/Drive path forms.
- [x] Scope-aware persistence rules are implemented for the current filesystem model: `found` rows remain, in-scope missing rows are soft-deleted, and out-of-scope rows are hard-deleted instead of being persisted with an explicit `out_of_scope` status.
- [x] Duplicate-integration rules were revisited for filesystem-backed plugins: SMB/Google Drive use plugin-reported `source_identity` to prevent duplicate backend connections while scan scope is modeled through `include_paths[]`.
- [x] in platforms view, it shows "ms_dos" and "ps1" and such. It should have a "display text". Maybe the DB should hold also a "display name" for some tables? think critically! I'm not sure this is the right solution.
- [x] I think we should add to the flow of detecting a game, running the whole metadata/media etc workflow. you should reuse the same code used while detecting game via scanning. if you need - refactor.
- [x] also to "game page" add "refresh metadata and media" to manually trigger this workflow for that game.
- [x] I think the automatic detection algorithm of a game can also try, as part of the "tryouts to detect a game" remove parenthesis "()" and brackets "[]". for example, the non-detected game `aladdin (u) [!]`, would become "aladdin" if we remove (u) and [!] and trim the whitespaces.
- [x] undetected game should not be shown in the library (or "playable games")
- [x] Game-page `Reclassify` deep-links no longer crash when the selected Undetected Games candidate contains nullable arrays or text fields; the Settings manual-review UI now normalizes that payload before reading `.length`, `.find()`, or `.trim()`
- [x] Library / Play cards now use a squarer full-bleed cover area and share the same cover fallback selection as the game detail page when no explicit `cover` media item exists
- [x] Browser-play source/version ambiguity: launch choice is source-record-scoped, ambiguous launchable sources require explicit selection, invalid remembered selections do not silently fall through to another source, same-title/different-source-record options render distinct labels, and js-dos executable selection stays separate from source selection.
- [x] Source-record-scoped hard-delete from the game page for filesystem-backed sources: eligible SMB/Google Drive source records expose a strict confirmation flow, destructive provider deletion runs behind a backend deletion service, only the selected source record and dependent rows are removed, and canonical membership is recomputed without implicitly deleting sibling sources.
- [x] Source-record and Undetected Games file deletes require a source-plugin dry-run preview before the UI exposes the real delete action.
- [x] Google Drive source deletes move explicit candidate/source files to Drive trash and never call permanent Drive delete for this flow.
- [x] SMB source deletes remove only explicit file targets and reject directory entries for this flow.
- [x] Real file-backed delete dialogs require a checkbox confirmation before the final delete button is enabled.
- [x] In library, we can use "right click" to open a menu with game specific actions (same actions available in the games page, but also add "change cover photo").
- [x] Collapsed shelves in library should take almost the whole row. the "..." card should not look like a card, but as text, without the background and frame of a card. also it should be much smaller (and centerlized vertically).
  - 2026-04-17 follow-up: the overflow affordance now renders as compact text/expander UI rather than a peer card.
- [x] Library / Play section shelves now use bounded preview paging on the parent page, with explicit `Open Shelf` focused routes (`/library/section/:sectionId`, `/play/section/:sectionId`) instead of mounting full free-scroll rows for every section.
- Cancelled: the original free-scroll shelf-arrow plan was superseded by bounded preview paging plus focused shelf routes, which solved the page-responsiveness problem without keeping many full horizontal rows mounted on the main Library / Play pages.
- [x] Achivements page (tab to the right of the "Library"). This tab should show display and dashboard all achivements from all achivements integrations set in MGA. Per-achivement system and per-game.
- [x] in settings -> undetected games, we should have a button to re-detect only the undetected games. This is useful for cases where the detection algorithm has been updated or the external databases where updated and we want to retry to redetect the games. Also, the per-game manual process should have "try re-detect" to use the detection algorithm.
- [x] Remove the "recent played" and "playable" side bar. Instead, in the library, there's an auto-shelf (can be empty) with the latest played games ordered from latest played game to the latest. it should also have a "remove" button from the list (hovering remove icon that appears when cursor hovers the game).
- [x] 2026-04-22 priority: center the Home / Settings / About page content instead of left-aligning the constrained layout.
- [x] 2026-04-22 priority: move the `Recent Played` auto-shelf from Library to Play, keep newest-first ordering, and keep the hover remove action.
- [x] 2026-04-22 priority: fix game-card / game-row context menus so they open at the cursor and always render above neighboring cards.
- [x] 2026-04-22 priority: section shelf/group labels should use shared display names (for example `Windows PC`) instead of leaking raw ids like `windows_pc`.
- [x] 2026-04-22 priority: remove the unused notification bell until there is a real notifications surface behind it.
- [x] Focused shelf pages can still feel heavy with large result sets; narrow the initial render cost and load more games as the user scrolls.
- [x] Shelf navigation and focused shelf browsing should use a fast, smooth scroll animation instead of abrupt jumps.
- [x] About page partner/vendor logos should not sit inside card framing; text-only dark logos should render on a light rounded tile so they stay readable.
- [x] Audit badges/icons across cards and hover states: use the correct platform/source marks, replace text-only `Playable` with icon + tooltip behavior, and normalize reuse of the same icons throughout the app.
- [x] Media gallery images should expose `Set as cover image` from a right-click/context action in addition to the existing game-level cover chooser.
- [x] Recheck metadata/platform normalization for known-platform titles that still surface as `unknown`, such as `Plasma Pong`.
- [x] Investigate MAME launch failures that report `ROMSET not recognized` and lock the runner contract down with focused proof.
- [x] RetroAchievements remains vulnerable to Cloudflare blocking in some environments; determine whether MGA can use a more compatible upstream access pattern without misclassifying auth/config failures.
- [x] Package MGA for real distribution (Windows/Linux/macOS and/or Docker) with an installer/start path that does not require source checkout.
  - 2026-04-23 follow-up: Windows-first portable ZIP packaging, bootstrap verification scripts, and a version/tag-based release flow are now in-repo. Cross-platform installers and non-Windows packaging remain future work.
- [x] Refresh `README.md` into a user-facing landing document with quick start, screenshots, packaging/install guidance, and feature overview.
- [ ] Add multi-user / user management for MGA (profile-based, no passwords at this stage).
  - [x] Phase 1: Profile schema, repositories, migration, and stale auth cleanup
  - [ ] Phase 2: Profile context middleware, admin/player API enforcement, and profile-scoped repositories
  - [ ] Phase 3: First-run setup APIs and frontend wizard
  - [x] Phase 4: Profile picker/header/default-profile frontend flow
  - [x] Phase 5: Settings -> Profiles management UI
  - [ ] Phase 6: Settings sync payload v2 with profile restore/bootstrap
  - [ ] Phase 7: Full profile-scoped library verification and regression tests
  - [ ] Web-client frontend should support profiles + profile management in settings
  - [ ] Set roles: Admin Player and Player. Admin Player can access settings; other than that, they are the same.
  - [ ] Each profile has its own integrations, library data, scans, favorites, achievements, and profile settings.
  - [ ] Plugin binaries/capabilities and media assets remain global.
  - [ ] Web-client: surfing to the web client, the user can choose a profile and set it as the browser default.
  - [ ] User image/icon should be shown on the top left, where profile switch/log-out lives.
- [x] Auto-update mechanism
  - [x] Add `mga-update.json` release manifest support with version, release notes URL, asset URL, SHA256, size, OS/arch/type, and minimum updater version.
  - [x] Add server update APIs: `GET /api/update/status`, `POST /api/update/check`, `POST /api/update/download`, and `POST /api/update/apply`.
  - [x] Download update assets into the runtime update cache and verify SHA256 before apply.
  - [x] Installed Windows update flow launches the verified Inno installer with silent update arguments.
  - [x] Portable Windows update checks/downloads/verifies the portable ZIP, launches an external updater helper, preserves mutable data, and restarts MGA.
  - [x] Add Settings -> Update UI with current/latest version, release notes, check, download, and apply actions.
  - [ ] Add signed release verification after code signing is introduced.
- [x] Add "favorites"
  - 2026-04-27 follow-up: canonical games now support server-persisted favorites, game detail/card heart toggles, and computed `Favorites` shelves as the first Library/Play shelf when favorite games exist in that scope.
- [ ] Make MGA available in 0.0.0.0 or localhost. it needs to be configurable. I think using as 0.0.0.0 will require admin rights on start-up.
  - [x] Server `LISTEN_IP` config supports `127.0.0.1`, `localhost`, `0.0.0.0`, concrete LAN IPs, and IPv6 literals.
  - [x] Portable release config ships local-only with `LISTEN_IP: "127.0.0.1"` and `PORT: "8900"`.
  - [ ] This should be configurable from the frontend as well (under "settings/general" page?)
- [ ] Make MGA install (Windows/Linux):
  - [x] Windows Phase 1: runtime layout resolver with portable, per-user, and machine/service data directories.
  - [x] Windows Phase 2: server startup flags/env for `--config`, `--data-dir`, `--app-dir`, `--service`, `--no-tray`, and `--runtime-mode`.
  - [x] Windows Phase 3: keep portable ZIP support with self-contained writable app directory.
  - [x] Windows Phase 4: add Inno Setup installer script and package builder.
  - [x] Windows Phase 5: support per-user user-process install without admin rights.
  - [x] Windows Phase 6: support all-users/service install with native Windows service execution.
  - [x] Windows Phase 7: installer choices for runtime mode, start after install, tray at login, local-only/LAN bind, and optional firewall rule.
  - [x] Windows Phase 8: uninstall removes service/startup/firewall/app files and keeps user data by default with explicit delete-data prompt.
  - [x] Windows Phase 9: installer/update manifest artifacts in GitHub Actions packaging workflow.
  - [x] Windows Phase 10: mutable data moved out of Program Files for installed layouts; portable layout remains self-contained.
  - [x] Packaging attribution: Inno Setup attribution added to NOTICE/README/release docs.
  - [ ] Linux Phase 1: map runtime path resolver to XDG config/data/cache paths.
  - [ ] Linux Phase 2: add tarball packaging, then decide on `.deb`, `.rpm`, or AppImage.
  - [ ] Linux Phase 3: add systemd user unit and system service install options.
- [ ] Achivement page shows only cached data. It needs to be able to show all information of all games, not just cached data.
  - [ ] Does cached data means in the frontend? or the server?
    - [ ] If on the frontend, explain the user what it means, because it is weird
    - [ ] If "cached" on the server - its a bug as the server should "bring" the achivement status from the plugin during scan. If you think this is wrong, you may push back just explain to the user and offer alternatives.
- [ ] In settings, add a tab to find duplicated games acorss sources.
  - [ ] duplications for games (ignoring versioning/platform)
  - [ ] duplications for games including version/platform etc.
- [ ] Home screen should have "library statistics" and "gamer statistics". Still not sure how to display these as different pages and conviniently, but it should support extensive statistics in a cool, colorful, way. Maybe, it should be in different pages, like "achivements" shows also statistics for achivements, so maybe a "library statistics", "gamer statistics" pages (where the latter includes achivements)?
- [ ] Make sure MGA server (and frontend client) both supporting Linux + Windows
- [ ] When playing a game, not all emulators support "retroachivements" achivements recording. check if our EmulatorJS does support it. If it does, check what it means to allow the user to run in that mode (which is restricting on "cheats" and stuff like that).
- [ ] When showing source files of a game (in undetected games page and game page), show all the files in a single multi-line textbox. otherwise it takes too much space. Also no need to show size per file, show total size.
- [x] Support "exclude directories" for files-backed sources. Update Web frontend to support this.
- [x] I am searching for "desert strike". in launchbox db website I find "Desert Strike: Return to the Gulf", which is the right game, but in MGA it finds only "MiniTank: Desert Strike" from IGDB.
- [x] In undetected games, "Inca 2 (MS DOS)" was not found. if I manually search for "Inca 2", also not found. If I search for "Inca II", it is found. Now if I remember correctly the normalization and cadidates to search, if there's no match, we remove the parenthesis (i.e. "Inca 2"), and if that is not found, we replace the numbers to roman numbers (i.e. Inca II) -> meaning, the undetected games search of "Inca 2 (MS DOS)" should have displayed "Inca II". Am I wrong?
- [x] Its time to release v0.0.8.
- [x] The auto update (in the settings->update) should ignore versions that are not in the format of "vX.X.X". Meaning, it shouldn't detect latest versions of the format "v0.0.8-beta" and such. Or, at the least, let the user decide their track (cutting edge vs stable)
- [x] The auto update, how does that work? MGA server should update the code and restart, how do you think to do that exactly? Remember MGA is still running, so it needs to be restarted. What is the plan. I think maybe the update setup executable can shutdown MGA -> update -> restart MGA. What do you think? it just needs to mind the type of installation.
- [ ] What do we do with the integrations redirect URL when the server is listening to 0.0.0.0 and we are NOT accessing from localhost/127.0.0.1 ?
