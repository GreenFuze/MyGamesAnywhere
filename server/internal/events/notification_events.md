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
| Achievements fetch | `operation_error` with `scope: achievements` identifies the affected connection and game when a plugin call fails | The browser can link to the exact connection without interpreting the raw error. |

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
| `operation_error` | Operation failed (achievements plugin call, sync key store/clear, etc.) | `scope`, `error`, plus scope-specific fields. Achievement failures include `profile_id`, `integration_id`, `integration_label`, `plugin_id`, `game_id`, and `game_title`. |

### Installed game checks

These profile-scoped events are emitted by the shared manual/background
installation-validation coordinator. Unchanged healthy background checks stay
silent in notification history.

| `event` | When | Payload (excerpt) |
|---------|------|-------------------|
| `installation_validation_started` | A manual or scheduled check dispatches to a connected endpoint | `profile_id`, `endpoint_id`, `command_id`, `trigger`, `total` |
| `installation_validation_finished` | The typed command reaches a terminal state | `profile_id`, `endpoint_id`, `command_id`, `trigger`, `status`, `total`, `changed_missing`, `changed_needs_repair`, `restored`, `error` |
| `installation_validation_schedule_updated` | A profile changes its automatic-check interval or paused state | `profile_id`, `enabled`, `interval_minutes` |

### Server updates

Server update lifecycle events are explicitly server-global. They contain only
public release metadata and update progress, never profile or provider data.

| `event` | When | Payload (excerpt) |
|---------|------|-------------------|
| `update_available` | The hourly checker first discovers a newer version during this server process | `current_version`, `latest_version`, `release_notes_url`, `message` |
| `update_download_started` / `update_download_progress` / `update_download_complete` / `update_download_error` | A player-started update download changes state | version, byte progress, verified local update path, coarse error |
| `update_apply_started` / `update_apply_error` | The verified updater is launched or cannot be launched | version, state, coarse error |
