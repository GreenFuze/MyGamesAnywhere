# ADR-0020: Multi-server bindings for one device/OS-user client

- **Status:** Accepted; supersedes ADR-0019 decision 4
- **Date:** 2026-07-18
- **Scope:** MGA Client configuration, endpoint identities, process lifecycle, tray controls

## Context

A device/OS-user identity may legitimately use several MGA Servers, such as a
local development server and a household TV server. A single global pairing
forces destructive unbind/re-pair cycles and makes server migration look like
device migration. Each server already issues its own endpoint identity, so the
client must preserve those identities independently.

## Decision

1. One installed MGA Client maintains a bounded list of server bindings. Each
   binding owns its normalized server URL, WebSocket URL, endpoint ID, client
   instance ID, display name, and separately protected Ed25519 private key.
2. Pairing with a new server appends a binding. Pairing with an already-bound
   equivalent origin fails clearly and never replaces it.
3. One tray process starts an agent for every binding. Each agent reconnects to
   its server independently. A browser `mga://start` challenge is redeemed only
   by the matching binding; an unknown server produces an actionable local
   error directing the player to **Pair this Windows user**.
   Pairing a new server asks an already-running tray process to exit, then the
   pairing process takes over and starts all bindings immediately.
4. Tray and CLI unbind actions target one server. Clearing all bindings requires
   an explicit all-bindings CLI choice. Unbinding one server removes only that
   binding and its key; other bindings remain paired.
5. The existing server-side endpoint and access model is unchanged. The same
   physical device/OS-user therefore remains a distinct endpoint on every MGA
   Server, as intended by ADR-0001.
6. Until a later per-binding lifecycle protocol exists, `endpoint.stop` stops
   the shared tray process and therefore all live binding connections. It does
   not remove any binding; starting the client again reconnects all of them.

## Persisted client migration

Client config schema 2 stores `bindings[]`. On first load of schema 1, the
client verifies the existing protected key, represents the legacy row as one
schema-2 binding with `legacy_identity=true`, then atomically writes schema 2.
The original DPAPI key file remains in place and is referenced only by that
binding. New bindings use deterministic, non-secret hashed key filenames in a
dedicated identity directory.

Migration fails closed before writing if the legacy key cannot be loaded. The
schema-1 file remains recoverable if the schema-2 atomic write fails. Unknown
schemas and fields remain rejected. No server SQLite migration is involved.

## Security and failure behavior

- Keys, launch tokens, and commands are never shared across bindings.
- Server-origin matching retains exact scheme, effective port, path, and host
  rules; equivalent loopback host spellings remain accepted.
- A broken or offline server does not prevent other agents from retrying their
  own connections.
- Removing the final binding returns the client to Not paired and deletes the
  config document.
- The binding list is bounded to 16 entries to prevent accidental config or
  process growth.

## Acceptance criteria

- Schema-1 migration preserves the existing endpoint/key and writes schema 2.
- Two server bindings can coexist with different endpoint IDs and keys.
- `mga://start` selects only the requested origin and never asks to discard a
  different binding.
- the tray lists and can unbind each server separately;
- client tests cover migration, duplicate origin rejection, targeted lookup,
  targeted unbind, and bounded validation.
