# SSE notification events (non-scan)

These events are published on `GET /api/events` alongside scan events (see `scan_events.md`).
Every JSON payload includes an RFC3339Nano `ts` field added by `events.PublishJSON`.

**No secrets** (tokens, passwords, raw config) are included. Friendly labels (e.g. integration label, plugin id) are OK.

## Progress bars (coarse)

| Flow | Determinate fields | Notes |
|------|-------------------|--------|
| `GET /api/integrations/status` | `integration_status_run_started.total`, then each `integration_status_checked` has `index` / `total` | One row per integration checked; use for a bar during status refresh. |
| Scan pipeline | `scan_*` events in `scan_events.md` | Finer-grained phases. |
| Sync | `sync_operation_started` then `sync_operation_finished` with `ok` | Spinner or two-step UI; finished payload includes summary counts (no secrets). |
| Achievements fetch | `operation_error` with `scope: achievements` includes `index` / `total` when a plugin call fails | `total` = plugins that will be queried for this game; `index` = Nth query attempted. |

## Event catalog

### Integrations

| `event` | When | Payload (excerpt) |
|---------|------|-------------------|
| `integration_created` | After `POST /api/integrations` persists | `integration_id`, `plugin_id`, `label`, `integration_type` |
| `integration_status_run_started` | Start of `GET /api/integrations/status` | `total` (integration count) |
| `integration_status_checked` | Each integration after `check_config` | `index`, `total`, `integration_id`, `plugin_id`, `label`, `status`, `message` |
| `integration_status_run_complete` | End of status run | `total` |

### Plugin process

| `event` | When | Payload (excerpt) |
|---------|------|-------------------|
| `plugin_process_exited` | Plugin stdout closed unexpectedly (crash / kill), not a normal `Close()` | `plugin_id`, `reason` (e.g. `unexpected_disconnect`), `detail` |

### Sync

| `event` | When | Payload (excerpt) |
|---------|------|-------------------|
| `sync_operation_started` | Start of push or pull | `operation`: `push` \| `pull` |
| `sync_operation_finished` | Push/pull success or failure | `operation`, `ok`, `error` (if failed); on success includes non-secret summary fields (counts, timestamps) |
| `sync_key_stored` | `POST /api/sync/key` succeeded | `status` |
| `sync_key_cleared` | `DELETE /api/sync/key` succeeded | `status` |

### Errors (generic)

| `event` | When | Payload (excerpt) |
|---------|------|-------------------|
| `operation_error` | Operation failed (achievements plugin call, sync key store/clear, etc.) | `scope`, `error`, plus scope-specific fields (`plugin_id`, `game_id`, `operation`, …) |
