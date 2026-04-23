# MyGamesAnywhere (MGA)

![MyGamesAnywhere — title banner](docs/branding/title-text.png)

**MyGamesAnywhere** is a **local-first game library** for people whose collection lives across **PC installs, emulators, cloud streaming, removable drives, network shares, and multiple storefronts**.

It runs a small Go server on your machine, scans the sources you choose, merges them into a canonical library, enriches the results with metadata and media, and gives you one shelf for what you own, what you can play, and where it actually lives.

**Current release line:** `v0.0.5`  
**Status:** pre-1.0, actively moving, local-first by design

## Why MGA exists

Most launchers are store-first. MGA is **library-first**.

That means:

- your collection is not reduced to one storefront
- ROMs, ripped media, cloud catalogs, and installed PC games can live in the same library
- metadata, achievements, media, and source provenance are visible together
- your data stays local unless *you* choose to sync it

## What makes MGA special

- **Canonical game merge across sources**  
  MGA does not just list source entries. It tries to reconcile them into one game with source-level provenance.

- **Local-first architecture**  
  Your database, media, and config live with your MGA runtime. This is not a hosted account-first app pretending to be local.

- **Cross-source library view**  
  Steam, Xbox, Epic, Google Drive, SMB, emulation folders, and metadata providers can all contribute to one shelf.

- **Playable awareness, not just catalog awareness**  
  MGA tracks whether a game is playable, streamable, browser-runnable, or just known.

- **Manual review and re-detect flow**  
  When automatic matching fails, MGA does not bury the problem. It exposes undetected titles, metadata search, and re-detect actions.

- **Built for collectors, not just storefront users**  
  Physical dumps, ROM libraries, mixed-platform collections, and metadata cleanup are first-class concerns.

- **Plugin-driven backend**  
  Sources, metadata providers, achievements, sync targets, and browser-play runtimes are modular instead of being welded into one monolith.

## Feature Status

| Area | Status | What it means |
|---|---|---|
| Unified cross-source library | Available | MGA merges source entries into canonical games instead of leaving you with duplicate launcher rows. |
| Local-first runtime | Available | SQLite, media, config, and plugins run locally. |
| Metadata enrichment | Available | LaunchBox, IGDB, RAWG, HLTB, Wikipedia, PCGamingWiki, YouTube, and similar provider paths feed the library. |
| Manual review + re-detect | Available | Undetected games can be reviewed, searched, and re-run through the detection flow. |
| Browser play runtimes | Available | Supported runtimes such as EmulatorJS, js-dos, and ScummVM can launch directly from the web UI where configured. |
| Achievements dashboard | Available | MGA exposes cached achievements across supported integrations and per-game detail views. |
| Save-sync migration flows | Available | Save sync jobs and migration status are exposed through the app and API. |
| Poster-first library and game pages | Available | The current UI focuses on cover art, grouped metadata, focused shelves, and smoother browsing. |
| REST API + web UI | Available | MGA exposes a first-party API and a React frontend on top of the same local server. |
| Portable Windows release packages | Available | MGA now has a Windows-first portable packaging and tagged-release flow built around `vX.Y.Z` releases. |
| Cross-platform installers | Planned | Proper install paths, prerequisites, and upgrade flow will follow after portable packages. |
| Windows desktop client | Planned | A dedicated desktop shell/client is intended beyond the current tray-first server experience. |
| Mobile client | Planned | Mobile browsing and companion flows are planned after packaging and desktop hardening. |
| Multi-user / user management | Planned | Separate users, user-aware libraries, and server-side identity are still ahead. |
| Cross-source user file/profile view | Planned | MGA should eventually understand user identity, ownership, and progression across multiple source accounts. |
| Upgrade-safe packaged releases | Planned | Tagged releases, portable packages, migration notes, and versioned upgrade guidance are part of the release track. |

## Current Highlights

- **Source plugins:** Steam, Xbox, Epic, SMB, Google Drive, local filesystem-style paths, and more
- **Metadata plugins:** LaunchBox, IGDB, RAWG, HLTB, MobyGames, PCGamingWiki, Wikipedia, YouTube
- **Runtime surfaces:** local server, web UI, Windows tray app today
- **Library workflows:** scanning, manual review, re-detect, context actions, recent-played shelfing, achievements browsing
- **Collector workflows:** metadata cleanup, cover overrides, merged source visibility, save-sync support

## Portable Release

The first packaged MGA line is a **Windows portable ZIP**:

- artifact name: `mga-v0.0.5-windows-amd64-portable.zip`
- runtime model: **portable**, writable folder required
- network model: **local only** on [http://127.0.0.1:8080](http://127.0.0.1:8080)

Portable release flow:

1. unzip MGA into a writable folder
2. run `Start MGA.cmd`
3. open [http://127.0.0.1:8080](http://127.0.0.1:8080)

The launcher verifies the runtime shape before starting and warns if the package was extracted somewhere unsuitable such as `Program Files`.

## Quick Start From Source

If you want to build MGA yourself instead of using the portable package:

### Windows

```powershell
cd server
.\build.ps1
.\start.ps1
```

Then open [http://127.0.0.1:8080](http://127.0.0.1:8080).

### Notes

- `build.ps1` builds the Go server, bundled plugins, frontend dist, and `openapi.yaml`
- Node.js is required for the frontend build unless you deliberately use `-SkipFrontend`
- MGA currently runs as a **portable working-directory-based runtime**, which is why portable packaging is the first packaging target

More technical detail lives in:

- [server/README.md](server/README.md)
- [server/frontend/README.md](server/frontend/README.md)
- [roadmap.md](roadmap.md)

## Packaging Direction

The current packaging direction is deliberate:

1. **portable Windows build first**
2. **installer-ready runtime layout after that**
3. **cross-platform installers later**

That order matches MGA's current architecture. Today the runtime still assumes writable local paths for:

- `config.json`
- `./data`
- `./media`
- `./plugins`
- `./frontend/dist`

That is fine for portable releases and a bad fit for pretending we already have a polished installed-app story.

The repo now includes:

- a dedicated Windows portable packaging script
- bootstrap and runtime verification scripts for packaged launches
- a tag-driven GitHub Release workflow for `vX.Y.Z`

## Versioning, Releases, and Upgrades

MGA now carries a repository version source at [`VERSION`](VERSION). The current line is **`0.0.5`**.

Release policy:

- releases are tagged as **`vX.Y.Z`**
- the root `VERSION` file is the default source of truth for packaging and build metadata
- pre-1.0 releases can still evolve quickly, but every tagged release should carry explicit upgrade notes

Upgrade policy:

- upgrades must **not silently discard user data**
- schema changes should be **additive and idempotent** where possible
- any release that changes runtime layout, schema behavior, or sync payload expectations must ship with migration notes
- until installers exist, upgrades should assume a **portable replacement flow** with backup guidance for `config.json`, `data/`, and `media/`

Detailed notes live in [docs/releases-and-upgrades.md](docs/releases-and-upgrades.md).

## Repo Layout

| Path | Purpose |
|---|---|
| [`server/`](server/) | Go server, plugins, database layer, API, build scripts |
| [`server/frontend/`](server/frontend/) | React/Vite frontend |
| [`docs/`](docs/) | branding and project docs |
| [`roadmap.md`](roadmap.md) | active roadmap and future work |

## Brand Assets

| Asset | Path |
|---|---|
| README / marketing banner | [`docs/branding/title-text.png`](docs/branding/title-text.png) |
| Web title strip | [`server/frontend/public/title.png`](server/frontend/public/title.png) |
| Optional title bar | [`server/frontend/public/title-bar.png`](server/frontend/public/title-bar.png) |
| UI logo | [`server/frontend/public/logo.png`](server/frontend/public/logo.png) |
| Favicon | [`server/frontend/public/favicon.ico`](server/frontend/public/favicon.ico) |
| Windows tray icon | [`server/cmd/server/mga.ico`](server/cmd/server/mga.ico) |

## Contributing

MGA is still tightening architecture, packaging, and product shape. If you contribute, prefer work that is:

- conservative with user data
- explicit about migrations and blast radius
- aligned with the local-first model
- honest about current vs planned behavior

## License

License and contribution policy can be finalized once packaging and release flow are locked in.
