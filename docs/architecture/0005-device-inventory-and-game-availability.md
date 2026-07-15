# ADR-0005: Bounded device inventory and game availability

- **Status:** Accepted and implemented locally
- **Date:** 2026-07-14
- **Scope:** Device protocol v1, MGA Client, server persistence/API, Devices, and game details

## Context

Installation and local play cannot be planned safely from online/offline state
alone. MGA needs current storage and known-runtime facts for the exact OS-user
client endpoint. A generic installed-software or environment dump would expose
unnecessary private data and would still not be a reliable play model.

## Decision

Protocol v1 adds a bounded inventory schema containing only:

- available storage roots with total and caller-available bytes;
- explicitly recognized game runtimes such as Steam, RetroArch, ScummVM,
  DOSBox, DuckStation, and PCSX2;
- capture time and inventory schema version.

The client uses one `LocalInventoryCollector` for both flows:

1. an authenticated `inventory.report` on connection and every 15 minutes;
2. the allow-listed, audited `inventory.refresh` command requested from the web
   interface.

The collector does not enumerate arbitrary installed applications, registry
software lists, environment values, or user documents. Runtime paths are
reported only for the small allow-list MGA understands.

The server derives per-game device availability from the Edition platform,
endpoint status/access, inventory age, free storage, and recognized runtime.
The initial states are `ready_for_setup`, `needs_runtime`, `not_scanned`,
`offline`, `update_required`, and `unsupported`. “Ready for setup” is not the
same as installed or immediately playable.

Initial runtime suggestions are conservative: native Windows, ScummVM, DOSBox,
DuckStation, PCSX2, and RetroArch-backed classic systems. Platforms without an
implemented local route remain unsupported rather than guessing.

## Persistence and compatibility

Migration 14 adds `device_inventories`, keyed by endpoint ID. The JSON columns
store protocol schema-1 storage/runtime arrays; `schema_version` is persisted
separately and validated on every read. Existing endpoints remain valid and
show “Not scanned yet” until a compatible client reports.

The migration is additive, cascades with endpoint deletion, and does not modify
profiles, games, source records, sync payloads, or client pairing identity.
Normal startup creates the SQLite pre-migration backup. Older clients remain
connected but do not advertise `inventory.refresh` and do not report inventory.

## Security and failure behavior

- Inventory reports are accepted only inside the endpoint's authenticated
  WebSocket connection.
- Manual refresh requires `Manage` access and endpoint capability negotiation.
- Invalid or unsupported schema versions fail fast.
- Successful inventory command result and inventory snapshot are committed in
  one database transaction.
- A collection failure is logged locally and retried at the next interval; it
  does not disconnect an otherwise healthy endpoint.

## Deferred

This decision does not select installer formats, transfer authentication,
installation roots, prerequisite policy, or elevation behavior. Those belong
to the first installation command vertical slice.

