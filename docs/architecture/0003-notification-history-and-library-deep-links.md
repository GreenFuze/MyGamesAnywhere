# ADR-0003: Browser notification history and filtered Library links

Status: Accepted

## Context

MGA already shows short-lived toast notifications for scans, integration status
changes, sync operations, and errors. Once a toast disappears, the user cannot
review it. Integration game lists also link to the Library without preserving
which source integration the user was viewing.

## Decision

- Keep transient toasts for immediate feedback.
- Also retain the newest 100 notifications in a top-bar notification center.
- Scope notification history to the active profile and the current browser.
- Opening the notification center marks its current notifications as read.
- Allow the user to clear the retained history.
- Source integration links use `/library?integration=<integration-id>`.
- Treat the Library URL filter as the source of truth so navigation, filter chips,
  and automatic all-page loading stay consistent.

The automatic scan interval remains a profile-scoped server setting in
Settings > Integrations. It uses the same scan path as the manual Rescan all
action.

## Persistence and migration

`NO_MIGRATION_NEEDED`: this change adds only a versioned, profile-scoped browser
local-storage key (`mga.notification-history.v1:<profile-id>`). It does not
change SQLite, server configuration, or an existing persisted JSON shape.
Existing installations safely begin with empty notification history. Invalid or
unreadable browser history is discarded without affecting MGA startup.

## Consequences

- Notification history follows a profile on the same browser, but is not a
  server-wide audit log and does not roam between browsers.
- The fixed 100-item limit bounds browser storage usage.
- Source integration deep links show only games currently associated with that
  integration, including canonical games shared with another source.
