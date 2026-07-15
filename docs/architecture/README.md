# MGA Architecture Decisions

> **Current implementation status:** Before continuing the active dirty worktree,
> read [`../handoffs/2026-07-14-cursor-handoff.md`](../handoffs/2026-07-14-cursor-handoff.md).
> That dated handoff is authoritative for live status, verification gaps, and next
> actions. ADRs here remain authoritative for accepted design; older roadmaps and
> release pages are not current implementation status.

This directory records architectural decisions and protocol contracts that
cross the server, web interface, and device agent boundaries.

## Decisions

- [ADR-0001: Web control plane and per-user MGA Client](0001-mga-client-architecture.md)
- [ADR-0002: Authoritative library reconciliation and background scans](0002-library-reconciliation-and-background-scans.md)
- [ADR-0003: Browser notification history and filtered Library links](0003-notification-history-and-library-deep-links.md)
- [ADR-0004: Conservative version-aware game identity](0004-version-aware-game-identity.md)
- [ADR-0005: Bounded device inventory and game availability](0005-device-inventory-and-game-availability.md)
- [ADR-0006: Managed archive installation](0006-managed-archive-installation.md)

## Protocols

- [MGA device protocol v1](mga-device-protocol-v1.md)

## Working product architecture

- [Unified library, play, installation, and save plan](unified-library-and-play-plan.md)
- [Player-facing language and information architecture](player-facing-language.md)

An accepted decision is the implementation source of truth until it is
superseded by another decision record. Protocol documents identify both the
implemented foundation and work that remains intentionally deferred.

Working product architecture documents capture agreed direction and unresolved
decisions before they are split into implementation ADRs. They are not a claim
that the described persistence model or UI has already shipped.
