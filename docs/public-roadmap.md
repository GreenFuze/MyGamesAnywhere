# MyGamesAnywhere Public Roadmap

This is the product-facing roadmap. The detailed engineering history remains in [`../roadmap.md`](../roadmap.md).

## Now

- Verify the v0.0.8-beta Windows portable and installer release flow on clean machines.
- Keep improving the poster-first library and game detail experience.
- Expand integration reliability for Steam, Xbox, RetroAchievements, LaunchBox, Google Drive, SMB, browser-play materialization, and local save sync.
- Continue cleanup around metadata detection, source provenance, manual review, local profiles, and profile-owned integration behavior.

## Next

- Harden Windows installer updates, tray/service restart behavior, and troubleshooting documentation.
- Add a Settings surface for server host/port visibility and LAN binding guidance.
- Continue game-page UX work around cards, media, actions, badges, and achievements.
- Continue source-backed version UX for launch options, achievements, and save-sync status.
- Improve public screenshots, release notes, and comparison docs.

## Later

- Cross-platform installers.
- Windows desktop client.
- Mobile companion client.
- Password/PIN-backed user management beyond local convenience profiles.
- Cross-source user file and profile view.
- More source, metadata, runtime, and save-sync integrations.

## Policy

- Keep user data safe during upgrades.
- Prefer local-first behavior by default.
- Keep planned work clearly separated from shipped behavior.
- Avoid hiding detection failures; expose review and repair paths instead.
