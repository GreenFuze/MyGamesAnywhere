
# Sources & Launching

## Steam source plugin (MVP)
### Scan
- Detect locally installed Steam library folders.
- Enumerate installed app IDs + names.
- Emit `Game` entries with `sourceRef = { kind: "steam", appId }`.

### Launch
- Launch via `steam://rungameid/{appId}`

### Monitor / playtime
- MVP can record session start/end best-effort:
  - if we can map to a process, monitor process lifetime
  - otherwise, record a minimal “attempted launch” and rely on user feedback

## Google Drive installers source plugin (MVP)

### Folder selection
- User can add multiple Drive folders (IDs) to scan.
- Scan recursively within those folders only.

### Setup file types
- `.exe`, `.msi`, `.zip`, `.7z`, `.rar`, `.iso`

### Scan output
For each detected file:
- `fileId`, `name`, `mimeType`, `size`, `modifiedTime`, `parentFolderId`
- `hash` (optional if downloaded; not required in MVP)

### Matching (installer → game)
MVP pipeline:
1. Normalize filename into candidate titles.
2. Query metadata plugins (LaunchBox export if available; else IGDB) to get candidates.
3. Produce top N candidates with:
   - `confidence` (0..1)
   - `explanation` list
4. User confirms or overrides.
5. Persist mapping:
   - `gdrive fileId` ↔ `gameId`
   - optional provider identity ids (IGDB/LaunchBox)

### Launch for installer-backed games
MVP launching from installer-backed entry:
- If the installer has been installed locally, we should have either:
  - a shortcut (lnk), or
  - an executable path recorded in local SQLite
- Launch order:
  1) protocol (if any)
  2) shortcut
  3) exe + args + working dir

### Optional future
- Download installer to local cache
- Detect installed state by inspecting local shortcuts, registry entries, or user-provided exe path
