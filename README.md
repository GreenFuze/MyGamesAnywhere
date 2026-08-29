# MyGamesAnywhere (MGA)

![MyGamesAnywhere](docs/branding/title-text.png)

## Your game library, served anywhere

MyGamesAnywhere is a self-hosted, headless-first game-library control plane. The Go server connects to game and storage sources, reconciles titles and copies into a canonical library, tracks provider availability and versions, and serves authorized game content, metadata, media, and compliant runtime artifacts to established frontend applications.

MGA's WebUI is an administration console. It manages profiles, sources, the library, catalog offers, artifacts, achievements, API clients, storage, and server operations. It is not a first-party game player, installer, or launcher.

## Product boundary

MGA owns:

- profiles, permissions, source connections, and integration credentials;
- canonical game identity, copies, versions, metadata, and media;
- current and historical provider offers, including subscription availability;
- authorized content delivery and materialization APIs;
- emulator and runtime artifact metadata and delivery when licensing, provenance, and integrity requirements are satisfied;
- achievements, jobs, storage, audit/recovery data, and the management WebUI;
- scoped APIs used by frontend integrations.

Frontend applications such as Playnite, LaunchBox, Pegasus, and future mobile integrations own device-local placement, installation or extraction, emulator configuration, storefront authentication, and execution. MGA appears to them as a library/store source.

MGA never supplies protected storefront packages, ROMs, firmware, licenses, or DRM bypasses. A source or runtime that cannot be delivered with adequate authorization and compliance evidence fails closed.

## Current pivot status

The headless-first product shift is under active development on `codex/mga-87-headless-first`. The branch currently contains:

- normalized catalog offers with version and availability history;
- versioned content-delivery and materialization APIs;
- a compliance-gated runtime artifact registry;
- scoped frontend API clients and capability discovery;
- explicit retirement responses for MGA-owned local install and launch workflows;
- the new profile-aware management console shell.

Deeper management workflows, old client/player code removal, packaging simplification, frontend SDKs, and frontend adapters are tracked in Jira. The latest public release and website may still describe the pre-pivot product.

## Run the development server

Requirements: Go, Node.js, and npm.

```powershell
cd server/frontend
npm ci
npm run build

cd ..
go run ./cmd/server --runtime-mode user --app-dir "$PWD" --data-dir "$PWD/.dev-data" --no-tray
```

Open `http://127.0.0.1:8900`. On a fresh data directory, complete the first-profile setup shown by the server. Keep development data separate from an existing MGA installation.

Useful verification commands:

```powershell
cd server/frontend
npm run test:unit
npm run build

cd ..
go test ./...
go vet ./...
```

## Contributing or continuing an agent session

Start with [AGENTS.md](AGENTS.md), [CLAUDE.md](CLAUDE.md), and [docs/agent-bootstrap.md](docs/agent-bootstrap.md). Current product, UX, architecture, security, and operating guidance lives in the [MGA Confluence space](https://greenfuzer.atlassian.net/wiki/spaces/MG/overview). Jira MGA is the only source of truth for open work, priority, assignment, acceptance criteria, and progress.

The repository contains historical client/player code during the staged retirement. Its presence is not evidence that those workflows remain part of the product.

MGA is pre-1.0 software under active development. See [VERSION](VERSION) and [LICENSE.md](LICENSE.md).
