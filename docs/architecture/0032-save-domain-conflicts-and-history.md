# ADR-0032: Save Domain conflicts and bounded recovery history

- **Status:** Implementation in progress
- **Date:** 2026-07-23
- **Scope:** Browser/local Save Domain conflicts, recovery history, retention,
  profile privacy, offline races, and clock skew
- **Jira:** MGA-34
- **Canonical record:** [MGA Confluence ADR-0032](https://greenfuzer.atlassian.net/wiki/spaces/MG/pages/3735553/ADR-0032+Save+Domain+Conflicts+and+Bounded+Recovery+History)
- **Depends on:** ADR-0017, ADR-0026, ADR-0031

## Decision

Every protected snapshot operation has an owning profile and exact Save Domain
ID. Divergent means that a caller's base manifest differs from the current
manifest. MGA never chooses from titles, modification times, device clocks, or
"newest wins". Exact slot mutations are serialized so two reconnecting writers
cannot both advance the same base.

Before replacing a current snapshot, MGA retains its manifest and archive in
server-local immutable history. Default retention is 10 versions and 30 days;
supported bounds are 1–50 versions and 1–365 days. Current data is never part
of pruning.

History and policies are profile scoped. Player-visible evidence contains only
safe origin/route labels, server acceptance time, optional untrusted reported
time, manifest hash, file count, and size. It excludes local paths, filenames,
endpoint IDs, credentials, and other profiles.

Recovery is explicit. MGA archives current data before promoting a retained
candidate and queues the normal provider upload. Failure preserves the current
snapshot. Provider-opaque saves remain inaccessible.

## Persistence

Migration 31 creates `save_domain_policies` and `save_domain_versions`.
Existing installations receive empty tables and use defaults lazily. Migration
30 is already applied and must never be edited. Save-sync cache manifests move
from version 1 to additive version 2 metadata; version-1 manifests remain
readable with safe fallback labels.

## Required evidence

- one success and one conflict for concurrent same-base writers;
- retention ordered by server acceptance despite future/past reported clocks;
- active-profile-only history lookup;
- current snapshot retained before replacement and recovery;
- bounded policy validation and pruning;
- migration, repository, service, API, OpenAPI, and frontend checks.
