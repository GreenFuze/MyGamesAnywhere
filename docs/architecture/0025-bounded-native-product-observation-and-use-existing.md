# ADR-0025: Bounded native-product observation and launch-only existing-installation grants

- **Status:** Accepted; initial Windows/managed-installation implementation complete
- **Date:** 2026-07-18
- **Scope:** MGA Client product evidence, cross-server installation reuse,
  launch authorization, device protocol, and server installation records
- **Depends on:** ADR-0007, ADR-0017, ADR-0020, ADR-0023

## Context

ADR-0023 prevents two MGA Servers from mutating the same managed directory or
native product. It deliberately left two gaps: Add/Remove Programs evidence is
currently reduced to a yes/no path association, and another paired server
cannot use an already installed game without taking management ownership.

Publishing a complete installed-software list to every server would expose
unrelated applications and still would not prove that two similarly named
games are the same product. Title similarity is not an identity boundary.

## Decision

### Bounded native-product evidence

MGA Client may inspect Windows Add/Remove Programs entries, storefront package
registrations, and equivalent OS facts, but reports them only when they are
associated with a game installation already known to MGA or when a locally
confirmed operation asks about a specific candidate.

The first implementation records exact Add/Remove Programs associations for
known managed paths. A native product observation contains a provider, a
stable client-derived product identity, display name, version, publisher, and
capabilities. It never contains an arbitrary uninstall command, registry key,
account identifier, or an unrelated application.

The stable Windows identity is derived from the registry scope, view, and
uninstall-entry key. The raw tuple remains local. Exact install-path
association is required; title similarity alone is never accepted.

Future Steam, Xbox/MSIX, GOG Galaxy, Epic, and other storefront adapters use
their stable product/package IDs. One canonical game may legitimately map to
several native products, and one native product may be linked explicitly to a
specific edition/source.

### Use existing installation

**Use existing installation** is a launch-only grant. It does not release,
adopt, update, repair, uninstall, or otherwise transfer management ownership.

The target server sends a typed request containing its game/source correlation
and the client-local installation ID. MGA Client:

1. resolves the local catalog record and manifest;
2. verifies the recorded path, launch candidates, and native-product evidence;
3. shows a local confirmation naming the game, server, and folder;
4. atomically grants that binding launch access to that local installation;
5. returns only the evidence needed to create a launch-only server record.

The grant is keyed by stable client binding ID and local installation ID. A
server URL, profile ID, title, or server-local game ID is not an authority
credential. Launch authorization is checked again by MGA Client on every
launch. Unbinding removes the active route but does not silently transfer the
grant to a replacement binding.

The server persists shared use separately from managed ownership. Shared
records never offer uninstall, cleanup, repair, or update actions. Revocation
is a later explicit client action; removal of a server-side link does not
delete files or revoke another server's ownership.

### Storefront-owned installations

Storefront installations remain device-global observations. Their install,
update, and uninstall lifecycle belongs to the storefront. MGA may launch an
exactly linked storefront product and describe its availability, but it must
label uninstall as affecting the whole Windows user/device and dispatch it
through the storefront adapter rather than deleting files.

### Save sync

This grant covers launch only. It does not grant writable save-sync ownership.
Save-domain observation may be shown, but enabling a second server's writable
sync requires the separate explicit transfer/reconciliation flow required by
ADR-0017 and ADR-0023.

## Failure and security behavior

- Ambiguous, missing, damaged, or unsupported manifests fail closed.
- A changed registry association invalidates native-product evidence until the
  client observes it again.
- A shared launch never uses the owner server's game/source IDs as authority.
- The client does not disclose the owning server's URL, endpoint, profile, or
  credentials to another server.
- Concurrent uninstall/update and launch operations share the local operation
  coordinator so destructive mutation cannot race an authorized launch.
- HTTP trusted-LAN transport does not remove the local-confirmation requirement.

## Persistence and migration

The client ownership catalog advances from schema 1 to schema 2 to add bounded
native-product evidence and per-binding launch grants. Loading schema 1
validates the complete old document before an additive atomic migration.
Unknown schemas or fields continue to fail closed.

Server migration 27 adds the client-local installation ID and authority mode
to device installation records. Migration 26 is immutable. Existing rows
migrate as `managed`; no existing install becomes shared automatically.

The device inventory advances to schema 5. Inventory remains stored in the
existing versioned JSON column, so no additional SQLite column is needed for
the product observation itself.

## Acceptance criteria

- Registry evidence uses exact path association and a stable opaque identity.
- Inventory never becomes a general installed-software dump.
- A second server can request use of a specific observed installation only
  after local confirmation.
- The resulting record can launch but cannot update, repair, clean up, or
  uninstall the owner's installation.
- Every launch rechecks the binding grant, local ID, manifest, path, and launch
  candidate.
- Catalog schema 1 migrates atomically to schema 2; migration 27 preserves all
  current server installations as managed.
- Tests cover privacy boundaries, stable identity, migration, declined
  confirmation, grant isolation, shared launch, and forbidden mutation.
