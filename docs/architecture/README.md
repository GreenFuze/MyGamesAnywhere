# MGA Architecture Decisions

> **Current guidance:** Start with
> [`../agent-bootstrap.md`](../agent-bootstrap.md). The
> [MGA Confluence space](https://greenfuzer.atlassian.net/wiki/spaces/MG/overview)
> is authoritative for current architecture, product, UX, security, and
> operating guidance. The
> [MGA Jira backlog](https://greenfuzer.atlassian.net/jira/software/c/projects/MGA/boards/69/backlog)
> is authoritative for open work and progress.

This directory records architectural decisions and protocol contracts that
cross the server, web interface, and device agent boundaries. These local files
remain code-adjacent decision evidence and implementation contracts. Dated
handoffs and deferred-work sections are historical context, not current status.

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
- [ADR-0019: Profile-owned accounts and recoverable client pairing](0019-profile-account-and-client-recovery.md)
- [ADR-0020: Multi-server bindings for one device/OS-user client](0020-multi-server-client-bindings.md)
- [ADR-0021: Actionable installation attention and local storage reporting](0021-actionable-attention-and-local-storage.md)
- [ADR-0022: Actionable, contextual notifications](0022-actionable-notifications.md)
- [ADR-0023: Client-authoritative cross-server installation ownership](0023-cross-server-installation-ownership.md)
- [ADR-0024: Installable PWA and optional managed HTTPS](0024-installable-pwa-and-managed-https.md)
- [ADR-0025: Bounded native-product observation and launch-only existing-installation grants](0025-bounded-native-product-observation-and-use-existing.md)
- [ADR-0026: Client-authoritative local save domains and explicit writer transfer](0026-local-save-domain-authority-and-transfer.md)
- [ADR-0027: Profile-bound OAuth drafts and request-scoped provider access](0027-profile-bound-oauth-drafts.md)
- [ADR-0028: Strong profile isolation hardening](0028-strong-profile-isolation-hardening.md)
- [ADR-0029: Google Drive "Shared with me" source folders](0029-google-drive-shared-with-me.md)
- [ADR-0030: Streamlined and automatic server updates](0030-streamlined-and-automatic-server-updates.md)
- [ADR-0031: Persisted save compatibility and converter registry](0031-save-compatibility-and-converter-registry.md)

## Protocols

- [MGA device protocol v1](mga-device-protocol-v1.md)

## Working product architecture

- [Decision responsibility and escalation boundary](agent-responsibility-boundary.md)
- [Unified library, play, installation, and save plan](unified-library-and-play-plan.md)
- [Player-facing language and information architecture](player-facing-language.md)
- [Profile isolation scope ledger](profile-isolation-scope-ledger.md)

An accepted local decision remains a code-coupled implementation contract until
it is superseded. Confluence carries the current consolidated decision log.
Any remaining implementation work must be represented in Jira rather than
tracked in an ADR checklist or handoff.

Working product architecture documents capture agreed direction and unresolved
decisions before they are split into implementation ADRs. They are not a claim
that the described persistence model or UI has already shipped.
