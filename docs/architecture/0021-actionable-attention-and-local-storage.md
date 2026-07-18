# ADR-0021: Actionable installation attention and local storage reporting

- **Status:** Accepted
- **Date:** 2026-07-18
- **Scope:** Play/Devices recovery UX and Windows client storage inventory

## Context

An attention badge without cause and a safe recovery action is not useful.
Legacy failed-install rows may lack the cryptographic cleanup marker required
for destructive cleanup, but can still be explained and dismissed without
touching files. Separately, Google Drive Desktop volumes may report themselves
as fixed disks, so drive type alone can expose unsuitable install destinations
and inflate local free-space totals.

## Decision

1. Every non-healthy installation row in Devices shows a player-facing cause
   and a **Review and resolve** action leading to the exact game/device section.
2. A legacy failed install without a cleanup marker explains that MGA cannot
   safely delete its files. The player may inspect the shown folder and dismiss
   the warning. Dismissal changes only server state; it never deletes files.
3. Cleanup and Retry remain available only when existing ADR-0007 ownership
   evidence permits them. UI wording must not imply repair or deletion where
   MGA lacks evidence.
4. Windows storage inventory includes only fixed drive letters backed by a real
   Windows volume identity. Network, removable, optical, RAM, SUBST-like, and
   virtual cloud-drive letters are excluded from install capacity and UI totals.
5. The client revalidates the resolved install root before preflight or file
   creation and rejects roots on those excluded devices. This is enforcement,
   not merely a UI filter.

## Persistence and compatibility

`NO_MIGRATION_NEEDED`: attention actions use the existing failure-ignore state
transition and fields. Storage filtering changes only newly reported inventory
contents; the protocol schema and SQLite/JSON shapes are unchanged. Existing
cached inventory is replaced by the next normal client inventory refresh.

## Acceptance criteria

- Duke's legacy row displays the installer failure explanation, folder, and a
  safe dismiss path without exposing cleanup;
- the Play attention link reaches a page with visible recovery actions;
- Google Drive Desktop letters G:, H:, and I: are absent after client refresh
  while the real local C: volume remains;
- client, server, frontend tests and production build pass.
