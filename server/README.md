# MyGamesAnywhere Server

Go server for game discovery, integrations, and settings sync.

## OpenAPI (Swagger)

The server’s public API is documented in OpenAPI 3.0. To regenerate `openapi.yaml` from the code (reflection over routes + operation docs in `internal/openapi/operations.go`):

```bash
go run ./cmd/openapi-gen
```

Or run the test (writes to a temp dir):

```bash
go test -run OpenAPI ./internal/openapi/...
```

Keep operation summaries and descriptions in `internal/openapi/operations.go` in sync when adding or changing endpoints.

## API notes (library)

- **`GET /api/games`** — Paginated list; each item is a full **game detail** object (same shape as `GET /api/games/{id}/detail`) so the UI can expose every metadata field via column pickers. Query: `page` (default `0`), `page_size` (default `100`, max `2000`). **`page_size=0`** returns all games in one response (allowed only if the library has ≤ 20000 games). **`GET /api/stats`** includes `canonical_game_count` (same notion of “visible” canonical game).
- **`GET /api/games/{id}`** — Same full detail JSON as **`GET /api/games/{id}/detail`** (and as one element of **`GET /api/games`**).
- **`GET /api/media/{assetID}`** — Streams a cached file from **`MEDIA_ROOT`** (config key, default `./media`) using `media_assets.id` and the row’s **`local_path`** (must be relative; `..` rejected). Use `media[].asset_id` from game detail; remote URLs in `media[].url` are unchanged.

## Building

Build the binary into `bin/`:

```bash
make build
```

Or directly:

```bash
go build -o bin/server ./cmd/server
```

Build the Windows portable release artifact:

```powershell
.\package-portable.ps1
```

This produces a versioned ZIP plus `SHA256SUMS.txt` under `server/release/`.

On **Windows**, plain `go build` does **not** embed the **File Explorer** application icon (only the **system tray** uses `mga.ico` via `go:embed`). For the `.exe` icon in Explorer, either:

- run **`build.ps1`** (generates `cmd/server/rsrc_windows_${GOARCH}.syso` from `mga.ico` before `go build`), or  
- from `server/`: `go generate ./cmd/server` (amd64 `.syso`), then `go build`.

The generated `rsrc_windows_*.syso` files are gitignored; regenerate after changing `cmd/server/mga.ico`.

On Windows the binary is typically `bin/server.exe`. The server changes its working directory to the executable's directory at startup, so place `config.json` next to the binary (e.g. in `bin/config.json`). A sample config with port 8900 is in `bin/config.json`.

## Running

Run from the directory containing the binary and config (e.g. `bin/`), or from anywhere—the server will chdir to the executable directory first:

```bash
./bin/server
```

## Configuration

- `config.json`: Must contain `PORT` and `DB_PATH`. Optional: `PLUGINS_DIR` (defaults to `plugins` if omitted from config).
- Google Drive (and other sources like SMB): Configure via integrations. Create an integration with `POST /api/integrations`; use `plugin_id: "game-source-google-drive"` for Drive as a game source, or `plugin_id: "sync-settings-google-drive"` for settings sync (body includes `config` with `credentials_json` and `folder_id`). Use `plugin_id: "game-source-smb"` for SMB. Credentials are stored in the SQLite database (integrations table). The internal settings sync uses the first integration with `sync-settings-google-drive`.
