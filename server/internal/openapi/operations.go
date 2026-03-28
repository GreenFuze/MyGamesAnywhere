package openapi

// OperationDoc holds documentation for one API operation (method + path).
// Used by the generator to fill summary, description, and response codes in the OpenAPI spec.
type OperationDoc struct {
	Method      string // GET, POST, etc.
	Path        string // e.g. /api/games (path params as {id})
	Summary     string // Short one-line summary for the operation.
	Description string // Longer description; optional.
	// RequestBodyDoc describes the request body for POST/PUT/PATCH. Empty means no body or optional.
	RequestBodyDoc string
	// ResponseDocs keyed by status code ("200", "202", "400", "404", "500"). Description for that response.
	ResponseDocs map[string]string
}

// Operations returns documentation for all public API operations.
// Paths must match the routes registered in http.BuildRouter (use {id} for path params).
func Operations() []OperationDoc {
	return []OperationDoc{
		{
			Method:       "GET",
			Path:         "/health",
			Summary:      "Health check",
			Description:  "Returns 200 OK if the server is running. No body.",
			ResponseDocs: map[string]string{"200": "Server is healthy"},
		},
		{
			Method:      "GET",
			Path:        "/api/games",
			Summary:     "List games (paginated)",
			Description: "Returns total, page, page_size and games[] as full detail objects (same shape as GET /api/games/{id}/detail) for library grids/tables. Query: page (0-based, default 0), page_size (default 100, max 2000). page_size=0 requests all games in one response (rejected if library exceeds 20000 games; use pagination). Totals also available via GET /api/stats (canonical_game_count).",
			ResponseDocs: map[string]string{
				"200": "ListGamesResponse: total, page, page_size, games (GameDetailResponse[])",
				"400": "Invalid query or library too large for page_size=0",
				"500": "Internal server error",
			},
		},
		{
			Method:       "DELETE",
			Path:         "/api/games",
			Summary:      "Delete all games",
			Description:  "Removes all games and their files from the database. Use before a full rescan to start fresh.",
			ResponseDocs: map[string]string{"200": "All games deleted", "500": "Internal server error"},
		},
		{
			Method:       "GET",
			Path:         "/api/games/{id}",
			Summary:      "Get game by ID",
			Description:  "Returns full game detail (same JSON as GET /api/games/{id}/detail and each element of GET /api/games).",
			ResponseDocs: map[string]string{"200": "GameDetailResponse", "404": "Game not found", "500": "Internal server error"},
		},
		{
			Method:       "GET",
			Path:         "/api/games/{id}/detail",
			Summary:      "Get game detail",
			Description:  "Full metadata, media (with local_path/hash when known), external IDs, merged files, generic play metadata (play availability, launch sources, launch candidates, file ids), unified Xbox/xCloud fields (is_game_pass, xcloud_available, store_product_id, xcloud_url when present), and all source games with resolver_matches (including metadata_json).",
			ResponseDocs: map[string]string{"200": "Game detail object", "404": "Game not found", "500": "Internal server error"},
		},
		{
			Method:       "GET",
			Path:         "/api/games/{id}/play",
			Summary:      "Stream one game file by file_id",
			Description:  "Streams a single file that belongs to the requested canonical game. Requires query param file_id (stable id from game detail/list file metadata). Local-disk Phase 6 implementation only; rejects invalid ids, path traversal, and files outside the game's found source roots. Supports Range requests via http.ServeContent.",
			ResponseDocs: map[string]string{"200": "Binary stream", "400": "Missing or invalid file_id", "404": "Game, file, or source path not found", "500": "Internal server error"},
		},
		{
			Method:       "GET",
			Path:         "/api/games/{id}/save-sync/slots",
			Summary:      "List save-sync slots for one browser runtime source",
			Description:  "Returns summary metadata for `autosave` and `slot-1` through `slot-5` for the requested canonical game, source game, runtime, and save-sync integration. Query params: integration_id, source_game_id, runtime.",
			ResponseDocs: map[string]string{"200": "JSON object with slots[] of SaveSyncSlotSummary", "400": "Missing or invalid integration_id, source_game_id, or runtime", "404": "Game not found", "500": "Internal server error"},
		},
		{
			Method:       "GET",
			Path:         "/api/games/{id}/save-sync/slots/{slot_id}",
			Summary:      "Get one remote save-sync slot snapshot",
			Description:  "Loads the manifest metadata and opaque zip snapshot for one browser-runtime save slot. Query params: integration_id, source_game_id, runtime.",
			ResponseDocs: map[string]string{"200": "SaveSyncSnapshot JSON", "400": "Missing or invalid parameters", "404": "Slot or game not found", "500": "Internal server error"},
		},
		{
			Method:         "PUT",
			Path:           "/api/games/{id}/save-sync/slots/{slot_id}",
			Summary:        "Store one remote save-sync slot snapshot",
			Description:    "Uploads a whole runtime save snapshot for a single slot. The server validates ownership, runtime compatibility, file manifest integrity, and optional conflict state via `base_manifest_hash`. Use `force=true` to overwrite after a conflict warning.",
			RequestBodyDoc: "JSON: { integration_id, source_game_id, runtime, base_manifest_hash?, force?, snapshot: { canonical_game_id?, source_game_id?, runtime?, slot_id?, files[], archive_base64 } }",
			ResponseDocs:   map[string]string{"200": "SaveSyncPutResult JSON", "400": "Invalid parameters or snapshot payload", "404": "Game or slot target not found", "409": "SaveSyncPutResult with conflict metadata", "500": "Internal server error"},
		},
		{
			Method:       "GET",
			Path:         "/api/media/{assetID}",
			Summary:      "Stream cached media file",
			Description:  "Serves a file from MEDIA_ROOT using media_assets.id and the row's local_path (must be relative, no '..'). Set MEDIA_ROOT in config (default ./media). Use media[].asset_id from game detail; supports Range requests via http.ServeContent.",
			ResponseDocs: map[string]string{"200": "Binary stream", "400": "Invalid id", "404": "Unknown asset or missing file", "500": "Internal server error"},
		},
		{
			Method:       "GET",
			Path:         "/api/stats",
			Summary:      "Library statistics",
			Description:  "Single JSON document: canonical and source game counts, breakdowns by platform/kind/integration/plugin, metadata coverage, decade and genre summaries, and canonical-game coverage for media and achievements.",
			ResponseDocs: map[string]string{"200": "LibraryStats JSON", "500": "Internal server error"},
		},
		{
			Method:       "GET",
			Path:         "/api/about",
			Summary:      "Application build metadata",
			Description:  "Returns the server's authoritative app version information, including version, commit, build date, and author credits.",
			ResponseDocs: map[string]string{"200": "AboutInfo JSON", "500": "Internal server error"},
		},
		{
			Method:       "GET",
			Path:         "/api/about/license",
			Summary:      "Open-source license text",
			Description:  "Streams the repository license file used by the About page.",
			ResponseDocs: map[string]string{"200": "License text", "500": "License file not found"},
		},
		{
			Method:       "GET",
			Path:         "/api/games/{id}/achievements",
			Summary:      "Get achievements for a game",
			Description:  "Fetches achievements on-demand from all achievement-capable plugins that have an external ID match for this game. Returns an array of achievement sets, one per source (steam, xbox, retroachievements).",
			ResponseDocs: map[string]string{"200": "Array of achievement sets", "400": "id required", "404": "Game not found", "500": "Internal server error"},
		},
		{
			Method:         "POST",
			Path:           "/api/scan",
			Summary:        "Run scan",
			Description:    "Starts an asynchronous scan job. Runs discovery: calls each source plugin (source.library.list) and, for integrations with root_path in config, runs the local inventory pipeline. Optional body limits which sources to scan.",
			RequestBodyDoc: "Optional JSON: { \"game_sources\": [\"integration-id-1\", ...] }. Omitted or empty = scan all sources.",
			ResponseDocs: map[string]string{
				"202": "ScanJobStatus JSON",
				"400": "Invalid JSON body",
				"409": "Another scan job is already active; returns the active ScanJobStatus",
				"500": "Internal server error",
			},
		},
		{
			Method:      "GET",
			Path:        "/api/scan",
			Summary:     "Run full scan",
			Description: "Same as POST /api/scan with no body: starts an asynchronous full-library scan job across all game source integrations.",
			ResponseDocs: map[string]string{
				"202": "ScanJobStatus JSON",
				"409": "Another scan job is already active; returns the active ScanJobStatus",
				"500": "Internal server error",
			},
		},
		{
			Method:       "GET",
			Path:         "/api/scan/jobs/{job_id}",
			Summary:      "Get scan job status",
			Description:  "Returns the current in-memory status for an asynchronous scan job, including state, progress counters, phase, current integration, report id, and any terminal error.",
			ResponseDocs: map[string]string{"200": "ScanJobStatus JSON", "400": "job_id is required", "404": "Unknown job id", "500": "Internal server error"},
		},
		{
			Method:       "GET",
			Path:         "/api/plugins",
			Summary:      "List plugins",
			Description:  "Returns the list of discovered plugins with ID, version, and capabilities.",
			ResponseDocs: map[string]string{"200": "Array of plugin descriptors", "500": "Internal server error"},
		},
		{
			Method:       "GET",
			Path:         "/api/plugins/{plugin_id}",
			Summary:      "Get plugin by ID",
			Description:  "Returns a single plugin's descriptor including config schema.",
			ResponseDocs: map[string]string{"200": "Plugin info", "400": "plugin_id required", "404": "Plugin not found", "500": "Internal server error"},
		},
		{
			Method:       "GET",
			Path:         "/api/config/frontend",
			Summary:      "Get frontend preferences",
			Description:  "Returns stored SPA preferences as a JSON object; {} if unset or invalid.",
			ResponseDocs: map[string]string{"200": "JSON object", "500": "Internal server error"},
		},
		{
			Method:         "POST",
			Path:           "/api/config/frontend",
			Summary:        "Set frontend preferences",
			Description:    "Upserts SPA preferences as a single JSON object (theme, layout, active save-sync integration, etc.). Stored under key frontend.",
			RequestBodyDoc: "JSON object (any keys). Max size enforced by server.",
			ResponseDocs:   map[string]string{"200": "Saved", "400": "Not a JSON object or too large", "500": "Internal server error"},
		},
		{
			Method:         "POST",
			Path:           "/api/config/{key}",
			Summary:        "Set config value",
			Description:    "Upserts a configuration value by key (e.g. google_drive_creds).",
			RequestBodyDoc: "JSON: { \"value\": \"string\" }",
			ResponseDocs:   map[string]string{"200": "Value set", "400": "Invalid body", "500": "Internal server error"},
		},
		{
			Method:       "GET",
			Path:         "/api/integrations",
			Summary:      "List integrations",
			Description:  "Returns all configured integrations (plugin instances) with id, plugin_id, label, integration_type, config.",
			ResponseDocs: map[string]string{"200": "Array of integrations", "500": "Internal server error"},
		},
		{
			Method:       "GET",
			Path:         "/api/integrations/status",
			Summary:      "Integration status",
			Description:  "Returns status for each integration (ok, error, unavailable) by calling plugin.check_config.",
			ResponseDocs: map[string]string{"200": "Array of { integration_id, plugin_id, label, status, message }", "500": "Internal server error"},
		},
		{
			Method:         "POST",
			Path:           "/api/integrations",
			Summary:        "Create integration",
			Description:    "Creates a new integration (plugin instance). Validates config against plugin schema and plugin.check_config before persisting.",
			RequestBodyDoc: "JSON: { \"plugin_id\": \"string\", \"label\": \"string\", \"integration_type\": \"source\"|\"metadata\"|..., \"config\": {} }",
			ResponseDocs: map[string]string{
				"201": "Integration created (body is the integration)",
				"400": "Validation error (missing/invalid plugin_id, label, integration_type, or config)",
				"409": "duplicate_integration: same plugin_id and equivalent config JSON (body includes integration_id and integration)",
				"500": "Internal server error",
			},
		},
		{
			Method:      "GET",
			Path:        "/api/events",
			Summary:     "Server-Sent Events stream",
			Description: "Opens a long-lived text/event-stream connection. Emits scan pipeline events (see server/internal/events/scan_events.md), app notifications (integrations, sync, plugin exit, coarse errors; see server/internal/events/notification_events.md), and save-sync migration lifecycle events (`save_sync_migration_started`, `save_sync_migration_progress`, `save_sync_migration_completed`, `save_sync_migration_failed`). Scan payloads include `job_id` so clients can recover in-progress jobs after reload. Each data line is JSON; map payloads include a ts field (RFC3339) when the publisher includes one.",
			ResponseDocs: map[string]string{
				"200": "text/event-stream; connection stays open until client disconnect or server shutdown",
				"500": "Streaming not supported",
			},
		},
		{
			Method:         "POST",
			Path:           "/api/save-sync/migrations",
			Summary:        "Start a save-sync migration job",
			Description:    "Starts a background save migration from one save-sync integration to another. Supports `scope=all` or `scope=game`; `game` requires `canonical_game_id`. Progress and completion arrive over SSE and can also be polled by job id.",
			RequestBodyDoc: "JSON: { source_integration_id, target_integration_id, scope: \"all\"|\"game\", canonical_game_id?, delete_source_after_success? }",
			ResponseDocs:   map[string]string{"202": "SaveSyncMigrationStatus JSON", "400": "Invalid request body or unsupported scope", "500": "Internal server error"},
		},
		{
			Method:       "GET",
			Path:         "/api/save-sync/migrations/{job_id}",
			Summary:      "Get save-sync migration job status",
			Description:  "Returns the latest in-memory status for a save-sync migration job, including counts for total, completed, migrated, skipped, and any terminal error.",
			ResponseDocs: map[string]string{"200": "SaveSyncMigrationStatus JSON", "404": "Unknown job id", "500": "Internal server error"},
		},
	}
}
