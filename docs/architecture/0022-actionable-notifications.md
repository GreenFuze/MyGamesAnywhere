# ADR-0022: Actionable, contextual notifications

Status: Accepted

## Context

MGA's browser notification history currently stores a title and free-form
description. Scan notifications expose only counts, so a player cannot tell
which games changed. Integration failures can expose a raw plugin error without
naming the affected connection or offering a path to repair it.

## Decision

- Every retained notification should answer: what happened, where it happened,
  and what the player can do next.
- Library scan completion events include a bounded structured list of added and
  removed source games. The notification center renders that list in a collapsed
  disclosure so routine scans remain compact.
- Integration-related failures include the connection id and friendly label.
  Authentication failures use player-facing language and link directly to the
  affected connection's repair and sign-in controls.
- Notification actions are internal MGA routes constructed by the frontend from
  typed event identifiers. Raw event data is never treated as an arbitrary URL.
- Existing generic notifications gain a relevant action when their event already
  identifies a connection, game, or device.
- Transient toasts remain concise; structured details are retained in notification
  history.

The list in one scan event is capped at 100 changes. Totals remain authoritative
when a larger change set is truncated, and the notification states how many
additional changes were omitted.

## Persistence and migration

`NO_MIGRATION_NEEDED`: scan report JSON and browser notification-history JSON
gain optional fields only. Existing SQLite rows and `mga.notification-history.v1`
records remain readable because all new fields are omitted when absent and the
readers accept the old shape. No table, column, key, or required field changes.

## Security and failure behavior

- Event payloads contain only ids, friendly labels, and game titles; they contain
  no credentials, tokens, or integration configuration.
- Profile-owned events include `profile_id` and continue through the existing SSE
  profile filter.
- If exact scan changes cannot be read, the scan fails explicitly instead of
  publishing a misleading successful diff.
- A malformed retained notification record is discarded without affecting MGA
  startup or other valid history entries.

## Consequences

- Scan reports can explain additions and removals rather than only their net
  count.
- Clicking a connection notification opens Settings > Connections, expands the
  correct group, and scrolls the affected connection into view.
- Browser history remains local to the active profile and browser as established
  by ADR-0003.
