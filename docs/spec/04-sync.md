
# Sync — Google Drive (MVP)

## OAuth
- The app uses a **shared OAuth client**.
- Tokens are stored locally (OS keychain if available).
- No syncing of secrets by default.

## Folder layout
- Ensure `.mygamesanywhere/` exists at Drive root (or app-managed location).
- Store canonical JSON files there.

## Sync algorithm
On startup (and on manual sync):
1. Pull remote JSON files (or initialize them if absent).
2. Validate schemas; fail-fast on invalid JSON with clear diagnostics.
3. Merge into local canonical state using **record-level last-write-wins**:
   - For each `Game` record: pick the version with higher `updatedAt`.
   - For playtime totals: by default, merge by **max** (or additive). MVP decision:
     - **MVP uses additive for session-based updates** OR **max for single-writer totals**.
     - Choose one and document it in `sync-meta.json`.
4. Persist merged canonical state to local SQLite cache and push updated JSON to Drive if dirty.

## Conflict policy
- LWW with timestamps for library entities.
- If both sides update same `gameId` near-simultaneously, keep newer `updatedAt`.
- UI may show “last sync merge applied” toast; no complex manual conflict UI in MVP.

## Performance
- Sync should minimize Drive API calls:
  - batch reads where feasible
  - ETag/modifiedTime checks
- Drive scanning is separate from sync (see source plugin spec).

## Required schemas
See `schemas/*.schema.json`.
