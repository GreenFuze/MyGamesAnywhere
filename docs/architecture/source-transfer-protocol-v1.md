# Source transfer protocol v1

Status: code-coupled protocol evidence for MGA-9 and Confluence ADR-0038.

This document describes the plugin protocol implemented by the MGA Server, Google Drive game source, and SMB game source. Confluence remains authoritative for current product and architecture guidance.

## Authority and safety boundary

- The authenticated profile must own both the source and destination connections.
- The server coordinates the move; source plugins materialize/delete and destination plugins stage/commit/abort.
- A destination plugin must advertise every v1 method before MGA offers it as a destination.
- A preview is non-mutating.
- The original is not removed until the destination manifest is verified, committed, and rediscovered by the library scanner.
- `transfer_id` is the idempotency and ownership key. A plugin may mutate only a stage/final object carrying the matching MGA ownership marker.
- Relative paths reject absolute paths, traversal, and the reserved `.mga` control directory.
- Temporary `.mga-transfer-*` content and `.mga` control content are excluded from source scanning.

## Methods

### `source.transfer.begin`

Validates the destination boundary and reserves an owned temporary destination.

Request:

```json
{
  "config": {},
  "transfer_id": "uuid",
  "destination_path": "Games/Title",
  "dry_run": false,
  "files": [
    {"relative_path": "game.bin", "size": 42, "sha256": "..."}
  ]
}
```

`dry_run: true` must not create folders or files. Repeating a non-dry call with the same transfer and manifest returns the existing stage or committed destination.

### `source.transfer.put`

Copies one server-materialized file into the owned stage.

```json
{
  "config": {},
  "transfer_id": "uuid",
  "destination_path": "Games/Title",
  "relative_path": "disc/game.bin",
  "source_path": "server-local-bounded-temporary-path",
  "size": 42,
  "sha256": "..."
}
```

The plugin validates the local source before upload and validates destination content before reporting success. Repeating a matching put is a no-op success.

### `source.transfer.commit`

Revalidates the complete manifest and atomically publishes the stage when the provider supports it.

```json
{
  "config": {},
  "transfer_id": "uuid",
  "destination_path": "Games/Title",
  "files": [
    {"relative_path": "disc/game.bin", "size": 42, "sha256": "..."}
  ]
}
```

Repeating commit returns success only when the final object has matching transfer ownership and manifest evidence. An unrelated collision fails closed.

### `source.transfer.abort`

Removes only an uncommitted stage carrying the matching transfer ownership marker. A committed destination is never removed by abort.

```json
{
  "config": {},
  "transfer_id": "uuid",
  "destination_path": "Games/Title"
}
```

## Persisted server recovery

Migration 33 adds `source_move_jobs` and `source_move_job_files`. Migration 34 adds the source reservation constraint. Jobs are profile-scoped for visibility, reserve a provider-identity/path destination across profiles while unfinished, and reserve one active move per profile/source game. This prevents both backing-destination races and two moves from acting on the same original at once.

The server persists the move phase before each provider boundary. Startup converts in-flight phases to `interrupted` and retains the last `recovery_phase`:

- Before destination commit: Retry first removes the owned stage, then starts the copy again. Clean up removes only the owned stage and leaves the original unchanged.
- During destination discovery: Retry scans the destination again; the original remains unchanged until discovery succeeds.
- During source deletion: Retry repeats the idempotent source deletion. Keep both retains the verified destination and source.

The normal ordering is:

1. materialize and hash source files;
2. stage and verify destination files;
3. commit destination;
4. scan the destination into the library;
5. remove the original through its source plugin;
6. complete the persisted job.

This ordering ensures that a destination scan failure cannot remove the only library-visible copy.
