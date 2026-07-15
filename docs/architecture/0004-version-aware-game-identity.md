# ADR-0004: Conservative version-aware game identity

- **Status:** Accepted and implemented locally
- **Date:** 2026-07-14
- **Scope:** MGA Server database, reconciliation, API, and web game details

## Context

The legacy canonical game model can combine source records when they share a
provider ID, and historically also used a cleaned-title fallback. A single
name is not a safe compatibility boundary: the same library can contain
platform releases, regional variants, remasters, DLC, storefront-specific
entries, and several legitimate records with very similar names.

MGA needs to relate those records for browsing without erasing the concrete
copy or version that controls play, achievements, installations, and saves.

## Decision

MGA uses three identity layers:

1. **Title** is the recognizable game family used to relate editions.
2. **Edition** is a concrete release/compatibility boundary. Its ID is the
   existing canonical game ID.
3. **Library Item** is every source record contributed by an integration. It
   remains independently stored as a `source_game` and linked to an Edition.

Automatic resolution is deliberately conservative:

- A cleaned or equal title never merges records by itself.
- Library/storefront membership neither merges nor separates records.
- Library Items become one Edition only when they share accepted provider
  identity and have compatible, known platform and content-kind evidence.
- The same accepted provider identity on different platforms creates separate
  Editions under one Title.
- Unknown or conflicting evidence remains separate.
- A temporary provider outage may retain a previously established Title only
  while the remaining accepted evidence still points to one identity.
- Newly conflicting provider identities split automatically established
  Titles rather than carrying a stale merge forward.
- Existing manual combine/separate pins remain authoritative.

This policy prefers under-merging. MGA may ask the player to review related
items later; it must not silently destroy release identity now.

Region and edition labels are persisted but remain unknown until reliable
evidence or an explicit player decision exists. They are not parsed from loose
title text in this stage. Content relationships such as base game, DLC, and
expansion remain a later vertical slice.

## Persistence

Migration 13, `version_aware_game_identity`, adds:

- `game_titles`, owned by a player profile;
- `game_editions`, keyed by the existing canonical game ID;
- `game_title_external_ids`, containing accepted provider evidence.

The backfill preserves every existing canonical ID as its Edition ID. Valid
single-profile rows are reconciled immediately. Malformed legacy rows with no
valid owner, or a canonical row spanning profiles, are left on the legacy
canonical path and logged for later review instead of preventing startup or
inventing ownership.

Normal application startup creates a SQLite backup before pending migrations.
Migration errors fail fast and do not record migration 13 as successful.

## Compatibility

- Game URLs, favorites, achievement joins, save-sync references, cover
  overrides, and manual grouping pins continue to use the stable canonical /
  Edition ID.
- Source records and files are not rewritten or deleted.
- Existing clients can ignore the optional `identity` field in game-detail
  responses.
- Settings-sync and save-sync payload formats are unchanged.

Rolling the executable back after migration leaves additive tables in place;
older code ignores them. Restoring the automatic pre-migration backup is the
full database rollback when required.

## UI

The normal game page uses player language: Version, Copies, and Match. Stable
IDs and evidence counts live under Technical details. Provider evidence and
manual review controls remain inspectable without dominating ordinary play.

## Consequences

- Some items that previously appeared as one game may separate on a later
  authoritative rescan when their only relationship was a title match or their
  provider evidence now conflicts.
- Related cross-platform releases can share a Title while retaining separate
  Edition IDs.
- Title-level browsing and content relationships can be added without another
  destructive reinterpretation of source records.

