# MyGamesAnywhere Public Roadmap

This is the product-facing roadmap. Cross-component technical decisions live in
the [`architecture`](architecture/README.md) documentation.

## Now

- Monitor the v0.1.2 Windows portable, installer, migration, and auto-update release flow on clean machines.
- Keep improving the Play-first library, statistics, achievements, and game detail experience.
- Expand integration reliability for Steam, Xbox, RetroAchievements, LaunchBox, Google Drive, SMB, browser-play materialization, and local save sync.
- Continue cleanup around metadata detection, source provenance, duplicate review, canonical split/merge, local profiles, and profile-owned integration behavior.
- Harden the implemented MGA Client v1 foundation: optional password/PIN sessions, explicit profile-to-endpoint grants, per-user pairing, authenticated outbound presence, diagnostics, and the Devices settings surface.

## Next

- Package and sign the standalone per-user MGA Client installer for published releases, then validate clean install/update/uninstall flows.
- Define minimum-client-version policy and implement restricted purple update-required recovery mode.
- Continue hardening Windows update recovery, tray/service restart behavior, and troubleshooting documentation.
- Add a Settings surface for server host/port visibility and LAN binding guidance.
- Continue game-page UX work around cards, media, actions, badges, and achievements.
- Continue source-backed version UX for launch options, achievements, and save-sync status.
- Improve public screenshots, release notes, and comparison docs.

## Later

- Expand MGA Client command coverage for game installation, launch/stop, emulator management, and machine-local repair workflows.
- Cross-platform installers.
- Mobile companion client.
- Remote credential provisioning, recovery, and broader account-management policy beyond local optional profile credentials.
- Cross-source user file and profile view.
- More source, metadata, runtime, and save-sync integrations.

## Policy

- Keep user data safe during upgrades.
- Prefer local-first behavior by default.
- Keep planned work clearly separated from shipped behavior.
- Avoid hiding detection failures; expose review and repair paths instead.
