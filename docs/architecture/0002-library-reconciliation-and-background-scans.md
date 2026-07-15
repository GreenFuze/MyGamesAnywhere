# ADR-0002: Authoritative library reconciliation and background scans

- **Status:** Accepted
- **Date:** 2026-07-14

## Context

MGA source integrations represent changing inventories. Files can be added to or
removed from a local or SMB library, storefront ownership can change, and
availability properties such as Xbox Game Pass or cloud streaming can change
without any action inside MGA.

Historically, `Rescan all` treated a successful source result containing zero
games as a skipped integration. That prevented the persistence layer from
reconciling previously discovered games, leaving stale library entries. MGA also
had no server-owned periodic source scan.

## Decision

### One scan coordinator

Manual and background scans are triggers into the same asynchronous scan job
coordinator. They share source discovery, progress events, cancellation and
serialization, report generation, persistence, and missing-game reconciliation.
There is no separate background reconciliation implementation.

Background runs use the coordinator's source-only mode. They refresh the source
plugin's own title, availability, media, files, and inventory while preserving
enrichment already produced by metadata providers. They do not invoke every
metadata provider every 15 minutes. Manual `Rescan all` remains the full
discovery-plus-metadata operation.

Only one source scan runs at a time. A scheduled scan defers when another scan
is active. Manual scans can join and display an already-active scan.

### Successful empty results are authoritative

A source call that completes successfully is a complete snapshot, including
when it contains zero games. MGA persists that empty batch and marks previously
found, in-scope source games as `not_found`. This removes them from the active
library while preserving history and allowing them to reappear cleanly.

A source failure is not a snapshot. Authentication, network, plugin, and remote
filesystem errors skip reconciliation for that source so a temporary outage
cannot empty the library. Source plugins must return an explicit error when they
cannot produce an authoritative snapshot; they must not represent failure as an
empty successful result.

### Profile-scoped background schedule

Automatic source scans are enabled by default for every profile, run every 15
minutes, and make their first attempt one minute after server startup. The user
can pause them or select an interval from 5 minutes through 24 hours.

The scheduler always enters a scan with the owning profile in context. This is
required because integrations, source games, reports, and settings are
profile-owned. A background scan must never enumerate or write another
profile's library.

Automatic source scans do not start an achievements refresh after every run.
Achievement refresh remains a separately coordinated operation.

### Visibility

Settings > Integrations shows whether automatic scans are scheduled, running,
paused, waiting, or failed. It exposes the next run, last result/error, current
job and current integration. Automatic jobs use the existing detailed scan
progress UI and are labeled `Automatic` in persisted scan history.

### Xbox availability

Xbox title history and current availability are separate concepts. A title is
not deleted merely because its Game Pass or cloud-streaming flag changes. MGA
keeps the title-history record and refreshes its availability properties. A
future entitlement-specific source may provide a stricter "currently playable"
view without overloading source identity.

## Persistence compatibility

`NO_MIGRATION_NEEDED`: no SQLite schema changes are required. Scan reports are
already persisted as JSON and tolerate the optional `trigger` field. The
automatic schedule is stored as a new optional profile setting key; profiles
without it receive safe defaults. Existing source-game status and reconciliation
columns already represent missing and restored games.
