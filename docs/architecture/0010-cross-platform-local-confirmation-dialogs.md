# ADR-0010: Cross-platform local confirmation dialogs

- **Status:** Accepted
- **Date:** 2026-07-16

## Context

ADR-0007 requires a local, time-bounded confirmation before destructive GOG
uninstall and failed-install cleanup. The first Windows-only implementation
called `TaskDialogIndirect` directly. Packaged E2E exposed multiple abrupt
agent exits and immediate invisible cancellation despite correcting the native
structure, callback, COM apartment, icon resource, and activation manifest.
Maintaining a custom Win32 dialog boundary is not part of MGA's product value.

## Decision

MGA Client uses `github.com/ncruces/zenity` v0.10.14 as its small
cross-platform dialog adapter. Uninstall and failed-cleanup confirmations use
the library's blocking `Question` API with:

- an explicit action label and Cancel label;
- Cancel focused by default;
- a warning icon and foreground behavior;
- the existing command-derived bounded context.

MGA maps library cancellation to `local_confirmation_declined`, context expiry
to `local_confirmation_timeout`, and all other library failures to a failed
command. No failure or timeout authorizes destructive work.

The dialog runs in the existing per-user agent process. MGA does not add a
second installed process, service, credential holder, generic command helper,
or new elevation boundary. The library owns platform UI details; MGA retains
the typed action, authorization, verified installation metadata, and
post-confirmation safety checks.

The dependency is MIT licensed and its notice ships with MGA Client.

## Compatibility and persistence

Windows uses the library's foreground Win32 message dialog. macOS is supported
by the same API; Unix-like desktops use the library's Zenity-compatible path.
Platform availability errors fail closed.

`NO_MIGRATION_NEEDED`: this changes only local transient UI rendering. Client
identity/config, device protocol, server persistence, installation manifests,
and cleanup markers are unchanged.
