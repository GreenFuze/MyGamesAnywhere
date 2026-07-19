# ADR-0026: Client-authoritative local save domains and explicit writer transfer

- **Status:** Implemented
- **Date:** 2026-07-18
- **Scope:** Local save adapters, multi-server write authority, release/adoption,
  provider-owned synchronization, protocol, persistence, and player actions
- **Depends on:** ADR-0017, ADR-0020, ADR-0023, ADR-0025

## Context

ADR-0017 classifies saves per play route. ADR-0023 requires at most one MGA
Server binding to hold writable authority for the same resolved local save
domain, but installation release/adoption deliberately does not transfer that
authority. MGA Client already reports whether bounded RetroArch and ScummVM
adapter probes succeeded; it does not yet reveal save paths or read/write saves.

An installation, title, edition, storefront account, emulator, and save domain
are not interchangeable identities. Several games may share an emulator save
directory, one game may have several incompatible save formats, and a provider
or emulator may already synchronize the files itself.

## Decision

### Exact local domain identity

Writable authority belongs to an exact client-resolved **local save domain**,
not to a title, canonical game ID, installation owner, server URL, or profile.
The adapter resolves a bounded set of files/directories from fixed
application-owned configuration and the exact launch route. It returns:

- a random stable `local_save_domain_id` persisted by MGA Client;
- adapter, route, format, and compatibility evidence;
- a local path set that never leaves MGA Client;
- an opaque fingerprint of the resolved path/configuration evidence;
- capabilities such as observe, snapshot, restore, and safe quiesce; and
- whether another synchronization writer is detected.

Title similarity and a directory name are never sufficient. An ambiguous,
overridden, unsupported, or changed configuration fails closed to observation.

### Separate save-authority catalog

MGA Client owns a versioned `save-domain-authority.json` catalog in its
per-OS-user data directory. Save authority is deliberately separate from the
installation-ownership catalog because their lifecycles differ. Each resolved
domain records one optional writer binding, adapter evidence, state, last
successful snapshot fingerprint, and audit timestamps.

States are:

```text
observed -> owned -> disconnected -> released -> owned
                    \-> reconciliation_required
```

Only the writer binding may request a restore or any MGA operation that changes
local save files. Read-only observation may be shared with all paired bindings
using sanitized facts. A launch-only installation grant grants no save access.

### Explicit release and transfer

Installation release/adoption and save release/transfer are separate choices.
The UI may offer them together for convenience, but neither checkbox implies
the other.

A save writer transfer requires a local MGA Client confirmation naming the
game/route, old server state, new server, and affected save kind. The client:

1. acquires the per-domain operation lease and prevents game launch or another
   save mutation from racing the transfer;
2. re-resolves the adapter and verifies the path/configuration fingerprint;
3. asks the current writer for a final snapshot when it is connected and the
   adapter supports it;
4. compares the local files, the last known local fingerprint, and the incoming
   writer's snapshot metadata;
5. completes only a no-conflict transfer automatically;
6. otherwise enters `reconciliation_required` without changing local files or
   writer authority.

If the old server is disconnected or lost, the player may locally release its
writer claim. Files remain untouched. The new server must then reconcile its
snapshot with the local state before restore becomes available. “Newest wins”
is not a safe default; the first implementation offers keep local, keep server
snapshot, or preserve both where the adapter can do so atomically.

Unbinding preserves save authority by default. The tray may offer **Release
save access and unbind** as an explicit additional action.

### Provider and emulator-owned synchronization

Steam, Xbox, Epic, xCloud, and other `provider_opaque` routes do not receive a
client-local writer owner. MGA shows that the provider manages the boundary and
does not offer transfer, restore, or an implication that a provider cloud save
exists.

When a local emulator has its own synchronization enabled, MGA treats that
domain as externally managed unless a future adapter proves a cooperative
protocol. MGA must not write beside an active provider manifest or compete with
another cloud writer. This applies to RetroArch's native cloud-sync feature as
well as storefront clients.

### First vertical implementation

Implementation begins with the adapter/authority framework and one bounded
ScummVM route whose launch identity and save path can be resolved exactly.
ScummVM supports an explicit target and `--savepath`; MGA must not continue
using `--auto-detect` as save identity. Existing configured targets and
per-target overrides remain observation-only until linked explicitly.

RetroArch follows only after its global, core, content-directory, and game
override hierarchy plus native cloud-sync state are all resolved. Save RAM and
save states remain separate compatibility kinds. MGA does not infer paths from
the ROM basename alone.

The vertical slice must include protocol capabilities and typed commands,
client resolver/catalog/leases/local confirmation, server authorization and
state, game-detail actions, snapshot conflict handling, tests, packaging, and
real E2E. A catalog-only or UI-only partial feature must not be enabled.

The implemented ScummVM adapter first materializes and verifies the exact
route content, then requires ScummVM `--detect` to produce exactly one
engine-qualified target. Once save access is granted, launches use that full
target plus the explicit per-domain `--savepath`; auto-detection is no longer
used for a writable domain. Launch, backup, restore, release, and reconciliation
share the client operation lease.

Snapshot transfer uses a 64 MiB / 4096-file bounded deterministic ZIP and a
ten-minute capability token sent in the HTTP Authorization header. Upload
tokens are single use; download tokens may be retried until expiry. The first
slot is `autosave` in the selected existing Save Sync connection. Restore uses
staging, preserves a changed local copy when requested, and rolls back on
activation failure.

## Persistence and migrations

This ADR-only change has `NO_MIGRATION_NEEDED`: it changes no runtime data.
Implementation requires:

- client `save-domain-authority.json` schema 1, atomically written and rejecting
  unknown schemas/fields;
- device inventory schema 6 for sanitized save-domain observations;
- server migration 28 for binding/domain authority links and reconciliation
  state; and
- additive snapshot metadata versioning if the existing save-sync manifest
  cannot represent adapter/local-domain fingerprints.

Migration 27 is already applied and must never be edited.
Migration 28 is now also applied to the real local database and must never be
edited. Any later SQLite change starts at migration 29.

## Security and failure behavior

- Local paths, filenames, provider credentials, and another server's identity
  never appear in cross-server inventory.
- HTTP on a trusted LAN does not remove local confirmation.
- Unknown adapters, path changes, active external sync, stale fingerprints,
  interrupted transfers, and catalog/snapshot disagreement fail closed.
- Restore uses an adapter-owned staging/backup strategy and never deletes the
  only known copy. Rollback is part of each adapter contract.
- The client operation coordinator serializes launch, snapshot, restore,
  release, transfer, install mutation, and uninstall whenever their local
  domains overlap.

## Acceptance criteria

- Two server bindings cannot both hold writable MGA authority for one exact
  local save domain.
- Installation ownership and launch-only grants never broaden save authority.
- Transfer/release requires local confirmation and preserves files on failure.
- A disconnected/lost writer can be released locally, after which restore is
  blocked until reconciliation finishes.
- Provider-opaque and externally synchronized routes never expose MGA restore
  or transfer actions.
- The first ScummVM route uses stable target/path evidence; ambiguous
  auto-detection or per-target overrides remain read-only.
- Tests cover catalog schema, privacy, collision, lost-writer release,
  conflict/reconciliation, rollback, external-sync refusal, and concurrent
  launch/restore exclusion.
