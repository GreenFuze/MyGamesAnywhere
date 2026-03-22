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
			Method:        "GET",
			Path:          "/health",
			Summary:       "Health check",
			Description:   "Returns 200 OK if the server is running. No body.",
			ResponseDocs:  map[string]string{"200": "Server is healthy"},
		},
		{
			Method:        "GET",
			Path:          "/api/games",
			Summary:       "List games",
			Description:   "Returns the current inventory of games with title, platform, kind, files, optional parent_game_id for addons, and unified Xbox/xCloud fields when present (is_game_pass, xcloud_available, store_product_id, xcloud_url).",
			ResponseDocs:  map[string]string{"200": "JSON list of games with files", "500": "Internal server error"},
		},
		{
			Method:       "DELETE",
			Path:         "/api/games",
			Summary:      "Delete all games",
			Description:  "Removes all games and their files from the database. Use before a full rescan to start fresh.",
			ResponseDocs: map[string]string{"200": "All games deleted", "500": "Internal server error"},
		},
		{
			Method:        "GET",
			Path:          "/api/games/{id}",
			Summary:       "Get game by ID",
			Description:   "Returns a lightweight game summary (same shape as list items, including Xbox/xCloud fields when present) by canonical game ID.",
			ResponseDocs:  map[string]string{"200": "Game summary", "404": "Game not found", "500": "Internal server error"},
		},
		{
			Method:       "GET",
			Path:         "/api/games/{id}/detail",
			Summary:      "Get game detail",
			Description:  "Full metadata, media (with local_path/hash when known), external IDs, merged files, unified Xbox/xCloud fields (is_game_pass, xcloud_available, store_product_id, xcloud_url when present), and all source games with resolver_matches (including metadata_json).",
			ResponseDocs: map[string]string{"200": "Game detail object", "404": "Game not found", "500": "Internal server error"},
		},
		{
			Method:       "GET",
			Path:         "/api/stats",
			Summary:      "Library statistics",
			Description:  "Single JSON document: canonical and source game counts, breakdowns by platform/kind/integration/plugin, metadata coverage (canonical games with a non-empty resolver title).",
			ResponseDocs: map[string]string{"200": "LibraryStats JSON", "500": "Internal server error"},
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
			Description:    "Runs discovery: calls each source plugin (source.library.list) and, for integrations with root_path in config, runs the local inventory pipeline. Optional body limits which sources to scan.",
			RequestBodyDoc: "Optional JSON: { \"game_sources\": [\"integration-id-1\", ...] }. Omitted or empty = scan all sources.",
			ResponseDocs:  map[string]string{"202": "Scan completed", "400": "Invalid JSON body", "500": "Internal server error"},
		},
		{
			Method:        "GET",
			Path:          "/api/scan",
			Summary:       "Run full scan",
			Description:   "Same as POST /api/scan with no body: runs discovery across all game source integrations (source.library.list) and, for integrations with root_path in config, runs the local inventory pipeline.",
			ResponseDocs:  map[string]string{"202": "Scan completed", "500": "Internal server error"},
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
			Method:         "GET",
			Path:           "/api/config/frontend",
			Summary:        "Get frontend preferences",
			Description:    "Returns stored SPA preferences as a JSON object; {} if unset or invalid.",
			ResponseDocs:   map[string]string{"200": "JSON object", "500": "Internal server error"},
		},
		{
			Method:         "POST",
			Path:           "/api/config/frontend",
			Summary:        "Set frontend preferences",
			Description:    "Upserts SPA preferences as a single JSON object (theme, layout, etc.). Stored under key frontend.",
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
			Description: "Opens a long-lived text/event-stream connection. Emits scan pipeline events (see server/internal/events/scan_events.md) and app notifications (integrations, sync, plugin exit, coarse errors; see server/internal/events/notification_events.md). Each data line is JSON; map payloads include a ts field (RFC3339).",
			ResponseDocs: map[string]string{
				"200": "text/event-stream; connection stays open until client disconnect or server shutdown",
				"500": "Streaming not supported",
			},
		},
	}
}
