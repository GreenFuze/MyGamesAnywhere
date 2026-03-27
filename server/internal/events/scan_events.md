# Scan SSE event catalog

Non-scan notifications (integrations, sync, plugin exit): see [`notification_events.md`](notification_events.md).

Events are delivered on `GET /api/events` as Server-Sent Events: each message has an `event:` name and a `data:` line with a JSON object.

## Common fields

- **`ts`** — RFC3339 nanosecond timestamp (UTC), added by the orchestrator for map payloads.
- **`integration_id`** — Present on all integration-scoped events.

## Events

| Event | Description |
|-------|-------------|
| `scan_started` | Scan run began. `integration_count` — integrations considered after filter. |
| `scan_integration_started` | Work starting for one integration. `plugin_id`, `label`. |
| `scan_integration_skipped` | Integration not processed. `reason`: `plugin_not_found`, `invalid_config`, `no_source_capability`, `no_games`. Optional `error`. |
| `scan_source_list_started` | About to list files or games from source plugin. `plugin_id`. |
| `scan_source_list_complete` | List done. `file_count` (filesystem) or `game_count` (storefront). `plugin_id`. |
| `scan_scanner_started` | File scanner pipeline starting. `file_count`. |
| `scan_scanner_complete` | Grouping done. `group_count`. |
| `scan_metadata_started` | Metadata enrichment starting. `game_count`, `resolver_count`. |
| `scan_metadata_phase` | Phase boundary. `phase`: `identify`, `consensus`, or `fill`. |
| `scan_metadata_plugin_started` | Resolver IPC call starting. `phase`, `plugin_id`, `batch_size`. |
| `scan_metadata_game_progress` | Resolver batch progress. `phase`, `plugin_id`, `game_index`, `game_count`, `game_title`. |
| `scan_metadata_plugin_complete` | Resolver IPC succeeded. `phase`, `plugin_id`; identify: `matched`, `total`; fill: `filled`, `candidates`. |
| `scan_metadata_plugin_error` | Resolver IPC failed (non-fatal). `phase`, `plugin_id`, `error`. |
| `scan_metadata_consensus_complete` | Consensus phase done. `identified`, `unidentified`. |
| `scan_metadata_finished` | Enrichment finished. `identified`, `unidentified`, `external_id_count`. |
| `scan_persist_started` | About to write DB. `source_game_count`. |
| `scan_integration_complete` | Integration persisted. `games_found`. |
| `scan_complete` | Full scan done. `canonical_games`, `duration_ms`. |
| `scan_error` | Fatal scan failure. `error`; optional `integration_id`. |

Per-game discovery events are not emitted in v1 (high volume); clients rely on counts above.
