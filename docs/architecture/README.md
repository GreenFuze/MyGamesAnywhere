# MGA Architecture Decisions

> **Current implementation status:** Before continuing the active dirty worktree,
> read [`../handoffs/2026-07-15-codex-return-handoff.md`](../handoffs/2026-07-15-codex-return-handoff.md).
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
- [ADR-0007: Web-authorized GOG Inno Setup installation](0007-locally-confirmed-gog-inno-installation.md)
- [ADR-0008: Device-selected Installed Games Play shelf](0008-device-selected-installed-games-shelf.md)
- [ADR-0009: Player-selected MGA Client elevation](0009-player-selected-client-elevation.md)
- [ADR-0010: Cross-platform local confirmation dialogs](0010-cross-platform-local-confirmation-dialogs.md)
- [ADR-0011: Device installation reconciliation](0011-device-installation-reconciliation.md)
- [ADR-0012: Profile and device install-location preferences](0012-install-location-preferences.md)
- [ADR-0013: Device-side installation preflight and prerequisite policy](0013-device-install-preflight.md)
- [ADR-0014: Multi-route Play control](0014-multi-route-play-control.md)
- [ADR-0015: Device emulator routes](0015-device-emulator-routes.md)
- [ADR-0016: Emulator setup and components](0016-emulator-setup-and-components.md)
- [ADR-0017: Route-level Save Domains and provider boundaries](0017-route-save-domains.md)
- [ADR-0018: Local save adapter discovery](0018-local-save-adapter-discovery.md)

## Protocols

- [MGA device protocol v1](mga-device-protocol-v1.md)

## Working product architecture

- [Decision responsibility and escalation boundary](agent-responsibility-boundary.md)
- [Unified library, play, installation, and save plan](unified-library-and-play-plan.md)
- [Player-facing language and information architecture](player-facing-language.md)

An accepted decision is the implementation source of truth until it is
superseded by another decision record. Protocol documents identify both the
implemented foundation and work that remains intentionally deferred.

Working product architecture documents capture agreed direction and unresolved
decisions before they are split into implementation ADRs. They are not a claim
that the described persistence model or UI has already shipped.
