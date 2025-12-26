
# MyGamesAnywhere — Spec Pack (Windows MVP, cross-platform codebase)

Generated: 2025-12-26

This zip contains a repo-ready specification for a **desktop Game Manager/Launcher** called **MyGamesAnywhere**.

## What this is
A commit-ready set of product + architecture + schema docs that a coding agent (or team) can implement with minimal follow-up.

## MVP scope (high level)
- Windows desktop app (Electron via Capacitor), codebase kept cross-platform.
- No backend. Sync via **Google Drive**.
- Sources: **Steam** (installed scan + protocol launch), **Google Drive installers** (scan user-selected folders).
- Metadata/media via plugins: **LaunchBox export**, **IGDB**, **SteamGridDB**.
- Plugin system supports **user-installed plugins** in MVP.

## Where to start implementing
1. Read: `docs/spec/01-overview.md` and `docs/spec/02-architecture.md`
2. Implement schemas in `schemas/`
3. Implement plugin host contracts in `docs/spec/05-plugins.md`
4. Execute milestones in `docs/spec/10-milestones.md`

## Reference docs (as provided)
- `references/DESIGN_DECISIONS.md`
- `references/STYLE_GUIDE.md`
