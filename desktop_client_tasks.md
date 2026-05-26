# Desktop Client — Feature Parity & Roadmap

Checked off as they land. Grouped by area.

---

## Play & Game Launch

- [ ] **Play tab = Installed / Launchable games** — show only games that have a local file or emulator-backed source; not browser/xCloud games
- [ ] **Auto-detect installed games** — scan sources for installed game files; mark games as launchable in the library
- [ ] **Emulator management** — Settings > Emulators tab: add/edit/remove emulator entries (path, platform(s), args template)
- [ ] **Launch game via emulator** — detect platform → look up emulator config → `Process.Start(emulator, args + gamePath)`
- [ ] **"Install emulator" prompt** — if a game needs an emulator that isn't configured, offer a guided setup or link to download
- [ ] **RetroAchievements-enabled play mode** — pass RetroAchievements credentials to emulator via command-line args (RAIntegration / retroachievements hardcore mode)
- [ ] **Recent Played** — record launched games locally; show horizontal shelf on Play page; allow removing entries
- [ ] **Game Detail: Launch button** — prominent Play button that picks the best launch option; dropdown for multiple sources

---

## Library

- [ ] **Advanced filter bar** — developer, publisher, genres (multi-select), platform (multi-select), source, integration, decade/year-range
- [ ] **List view** — tabular rows (title, platform, developer, year) alongside existing grid
- [ ] **Timeline view** — games grouped by release year, newest-first
- [ ] **Shelf view** — horizontal shelf rows per configured section (mirrors web "Shelf" mode)
- [ ] **Bulk select + reclassify** — checkbox multi-select → push queue to Undetected/reclassify workflow
- [ ] **Bulk hard-delete sources** — select multiple source records → preview files → confirm → DELETE
- [ ] **Move to source** — for file-backed games: pick a target source + directory (with create-dir support), apply move for multiple games; move entire directory when all files are within it; update database paths; mirrors webclient_tasks item
- [ ] **Validate files** — Settings > Integrations: "Validate files" for file-backed sources; list games whose files are missing; offer to remove stale records
- [ ] **Hard-delete: missing files OK** — if game files don't exist on disk, remove the DB record anyway (don't error)

---

## Game Detail

- [ ] **Source games panel** — list each source_game with integration label, platform, raw title, external ID
- [ ] **Source file inventory** — files attached to each source (path, size, status)
- [ ] **External links** — IGDB, Steam store page, etc. from external_ids; open in system browser
- [ ] **Merge source into another game** — "Move to another game" search → `POST /api/games/{id}/source/{sourceId}/merge`
- [ ] **Split source** — detach source into its own canonical entry → `POST /api/games/{id}/source/{sourceId}/split`
- [ ] **Hard-delete source** — preview files → confirm → `DELETE /api/games/{id}/source/{sourceId}`; skip if files already gone
- [ ] **Clear canonical pin** — `DELETE /api/games/{id}/canonical-pin`
- [ ] **Refresh metadata** — `POST /api/games/{id}/metadata/refresh` with SSE progress feedback
- [ ] **Resolver matches** — show metadata resolver candidates with accept/reject actions

---

## Achievements

- [ ] **Achievement Explorer** — per-game expandable list: individual achievement name, description, icon, unlock date, points, rarity; calls `GET /api/achievements/explorer`
- [ ] **Per-source achievement sets** — expandable sets within a game (one per source/system)
- [ ] **Achievement search + filter** — search by name/description; filter by unlocked / locked / all
- [ ] **Refresh job progress** — SSE-driven progress bar during refresh; per-provider success/fail counts
- [ ] **Failed provider summary** — surface rate-limit and auth-failure errors with actionable messages
- [ ] **Points display** — earned/total points where available (RetroAchievements, Xbox)

---

## Stats

- [ ] **Source/integration breakdown bar chart** — "By Source" ranked bars
- [ ] **Kinds breakdown bar chart** — game, dlc, demo, etc.
- [ ] **Coverage panel** — metadata coverage tiles (cover art %, description %, genres %) from `LibraryStatistics.coverage`
- [ ] **Scan history** — recent scan reports list (started_at, duration, games added/removed/updated)
- [ ] **Stat tiles row** — total games, favorites, achievements unlocked, with icons (matches web StatTile grid)

---

## Settings

### Integrations tab
- [ ] **Add integration wizard** — select plugin → fill config fields dynamically
- [ ] **Edit integration** — edit name + config fields
- [ ] **Delete integration** — confirm → `DELETE /api/integrations/{id}`
- [ ] **OAuth flow** — open browser → poll for callback → show result (`POST /api/integrations/{id}/auth`)
- [ ] **Scan trigger** — per-integration + global Scan button; live SSE scan progress bar with cancel
- [ ] **Scan job progress** — current/total, per-integration status, events log via SSE
- [ ] **Cancel scan** — `POST /api/scan/{jobId}/cancel`
- [ ] **Integration config fields** — dynamic form for plugin-defined schema (text, password, folder-picker, bool, enum)
- [ ] **Folder browser dialog** — server-side folder tree for picking local paths
- [ ] **Per-integration games list** — expandable list of games per integration
- [ ] **Import integrations from sync settings** — pull integration configs from a sync_settings source and apply locally

### Emulators tab (new)
- [ ] **List configured emulators** — name, executable path, platforms served, args template
- [ ] **Add / Edit / Delete emulator**
- [ ] **Test emulator** — verify the executable exists and runs

### Undetected Games tab
- [ ] **Manual review candidate list** — paginated, filterable (`GET /api/manual-review`)
- [ ] **Candidate detail** — file inventory, current classification, resolver suggestions
- [ ] **Search & match** — search canonical games → `POST /api/manual-review/{id}/match`
- [ ] **Mark DLC / Not a game / Base game** — quick classification buttons
- [ ] **Re-detect single / all** — `POST /api/manual-review/{id}/redetect`
- [ ] **Delete candidate files** — preview → confirm hard-delete
- [ ] **Bulk reclassify queue** — accept queue from Library bulk-select

### Duplicates tab
- [ ] **Merge workflow** — select preferred canonical → merge others into it
- [ ] **Per-group source breakdown** — which integrations/files make up each duplicate

### Cache tab
- [ ] **Entry list view** — individual entries with canonical title, integration, status, size, file count
- [ ] **Prepare cache** — trigger cache preparation for selected entries

### Plugins tab
- [ ] **Capabilities list** — human-readable capability labels per plugin

---

## Media Manager (new page)

- [ ] **Game Media page** — grid of all media assets; accessible from Game Detail
- [ ] **Set as cover / hero / background** — PUT overrides via API
- [ ] **Upload media** — `POST /api/games/{id}/media`
- [ ] **Delete media asset** — `DELETE /api/games/{id}/media/{assetId}`
- [ ] **Image/video preview** — inline image preview; open video in system player
- [ ] **YouTube thumbnail + open** — show thumbnail; open link in browser

---

## First-Run Wizard (Onboarding rewrite)

- [ ] **Multi-step wizard** — replace single-URL onboarding screen with a proper wizard:
  1. Welcome + app intro
  2. Connect to server (URL input + test connection)
  3. Select active profile
  4. Import integrations (optional, from sync)
  5. Done — open Library
- [ ] **Skip wizard** — allow jumping straight to main UI if user already has config

---

## Cross-Cutting

- [ ] **Global search** — TitleBar search box functional; filters current page or navigates to Library with query
- [ ] **Sidebar game count badge** — live game count next to Library nav item
- [ ] **SSE: integration refresh events** — `integration_refresh_complete` reloads Integrations tab status
- [ ] **Deep links** — navigate directly to a game by ID from external sources (file association, URL scheme `mga://`)

