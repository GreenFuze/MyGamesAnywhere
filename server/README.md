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

## Building

Build the binary into `bin/`:

```bash
make build
```

Or directly:

```bash
go build -o bin/server ./cmd/server
```

On Windows the binary is typically `bin/server.exe`. The server changes its working directory to the executable's directory at startup, so place `config.json` next to the binary (e.g. in `bin/config.json`). A sample config with port 8900 is in `bin/config.json`.

## Running

Run from the directory containing the binary and config (e.g. `bin/`), or from anywhere—the server will chdir to the executable directory first:

```bash
./bin/server
```

## Configuration

- `config.json`: Must contain `PORT` and `DB_PATH`. Optional: `PLUGINS_DIR` (defaults to `plugins` if omitted from config).
- Google Drive (and other sources like SMB): Configure via integrations. Create an integration with `POST /api/integrations`; use `plugin_id: "game-source-google-drive"` for Drive as a game source, or `plugin_id: "sync-settings-google-drive"` for settings sync (body includes `config` with `credentials_json` and `folder_id`). Use `plugin_id: "game-source-smb"` for SMB. Credentials are stored in the SQLite database (integrations table). The internal settings sync uses the first integration with `sync-settings-google-drive`.
