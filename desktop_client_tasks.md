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

- [x] **Advanced filter bar** — platform + genre multi-select (Flyout + CheckBox); developer, publisher, integration single-select dropdowns; year from/to; Clear Filters button
- [x] **List view** — tabular rows (title, platform, developer, year) alongside existing grid; Year (newest/oldest) sort options added
- [x] **Timeline view** — games grouped by release year, newest-first
- [x] **Shelf view** — horizontal shelf rows per configured section (mirrors web "Shelf" mode)
- [ ] **Bulk select + reclassify** — checkbox multi-select → push queue to Undetected/reclassify workflow
- [x] **Bulk hard-delete sources** — select multiple source records → confirm → batch DELETE via /api/games/sources/delete-batch
- [ ] **Move to source** — for file-backed games: pick a target source + directory (with create-dir support), apply move for multiple games; move entire directory when all files are within it; update database paths; mirrors webclient_tasks item
- [ ] **Validate files** — Settings > Integrations: "Validate files" for file-backed sources; list games whose files are missing; offer to remove stale records
- [ ] **Hard-delete: missing files OK** — if game files don't exist on disk, remove the DB record anyway (don't error)

---

## Game Detail

- [x] **Source games panel** — list each source_game with integration label, platform, raw title, external ID
- [x] **Source file inventory** — files attached to each source (path, size, status)
- [x] **External links** — IGDB, Steam store page, etc. from external_ids; open in system browser
- [x] **Merge source into another game** — inline search panel → `GET /api/canonical-games/search` + `POST /api/games/{id}/sources/{sourceId}/canonical/merge`
- [x] **Split source** — detach source into its own canonical entry; inline button per source row
- [x] **Hard-delete source** — two-step inline confirm → `DELETE /api/games/{id}/sources/{sourceId}`; navigates back if canonical deleted
- [x] **Clear canonical pin** — Unpin button per source row → `DELETE /api/games/{id}/sources/{sourceId}/canonical-pin`
- [x] **Refresh metadata** — ↻ Metadata button in action bar → `POST /api/games/{id}/refresh-metadata`; handles 409/422
- [x] **Resolver matches** — per-source expandable list of resolver matches with Active/Outvoted/Manual status badges

---

## Achievements

- [x] **Achievement Explorer** — per-game expandable list: individual achievement name, description, icon, unlock date, points, rarity; calls `GET /api/achievements/explorer`
- [x] **Per-source achievement sets** — expandable sets within a game (one per source/system)
- [x] **Achievement search + filter** — search by name/description; filter by unlocked / locked / all
- [x] **Refresh job progress** — SSE-driven progress bar during refresh; per-provider success/fail counts
- [x] **Failed provider summary** — rate-limit waiting message + warning count; error toast on failure
- [x] **Points display** — earned/total points where available (RetroAchievements, Xbox)

---

## Stats

- [x] **Source/integration breakdown bar chart** — "By Source" ranked bars
- [x] **Kinds breakdown bar chart** — game, dlc, demo, etc. (added in Phase 18)
- [x] **Coverage panel** — metadata coverage tiles (cover art %, description %, genres %) from `LibraryStatistics.coverage`
- [x] **Scan history** — recent scan reports list (started_at, duration, games added/removed/updated)
- [x] **Stat tiles row** — total games, favorites, achievements unlocked, with icons (matches web StatTile grid)

---

## Settings

### Integrations tab
- [x] **Add integration wizard** — select plugin → fill config fields dynamically
- [x] **Edit integration** — edit name + config fields
- [x] **Delete integration** — confirm → `DELETE /api/integrations/{id}`
- [x] **OAuth flow** — open browser → poll for callback → show result (`POST /api/integrations/{id}/auth`)
- [x] **Scan trigger** — per-integration + global Scan button; live SSE scan progress bar with cancel
- [x] **Scan job progress** — current/total, per-integration status, events log via SSE
- [x] **Cancel scan** — `POST /api/scan/{jobId}/cancel`
- [x] **Integration config fields** — dynamic form for plugin-defined schema (text, password, folder-picker, bool, enum)
- [x] **Folder browser dialog** — server-side folder tree for `_path` config fields via `POST /api/plugins/{id}/browse`; inline panel with Up navigation + Select
- [x] **Per-integration games list** — expandable panel per row, lazy-loaded via `GET /api/integrations/{id}/games`
- [ ] **Import integrations from sync settings** — pull integration configs from a sync_settings source and apply locally

### Emulators tab (new)
- [x] **List configured emulators** — name, executable path, platforms served, args template
- [x] **Add / Edit / Delete emulator**
- [x] **Test emulator** — File.Exists check + Process.Start + 500ms kill; per-row IsTesting spinner

### Undetected Games tab
- [x] **Manual review candidate list** — filterable by title/platform/integration; scope toggle (active/archive)
- [x] **Candidate detail** — lazy-loaded file inventory + resolver suggestions in right panel
- [x] **Search & match** — metadata provider search → apply result
- [x] **Mark DLC / Not a game / Base game** — quick classification buttons (WrapPanel)
- [x] **Re-detect single / all** — per-row + batch redetect with status feedback
- [x] **Delete candidate files** — two-step via Delete Files button → toast confirmation
- [ ] **Bulk reclassify queue** — accept queue from Library bulk-select (deferred — depends on Library bulk-select)

### Duplicates tab
- [x] **Merge workflow** — expand group → per-source breakdown → "⊕ Merge into this" → inline confirm → MergeSourceGameAsync for each non-preferred source
- [x] **Per-group source breakdown** — integration label, platform, kind, file count, size, cached badge per source; loose/strict mode toggle

### Cache tab
- [x] **Entry list view** — individual entries with canonical title, integration, status, size, file count
- [x] **Prepare cache** — per-entry Prepare button → POST /api/games/{id}/cache/prepare; spinner while in flight

### Plugins tab
- [x] **Capabilities list** — CapabilitiesText shown in accent color below Provides (hidden when empty)

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
- [x] **Sidebar game count badge** — live game count next to Library nav item
- [x] **SSE: integration refresh events** — `integration_refresh_complete` reloads Integrations tab status + started/progress/failed handlers
- [ ] **Deep links** — navigate directly to a game by ID from external sources (file association, URL scheme `mga://`)

