# Releases and Upgrades

## Versioning policy

MGA uses a repository-level version file at [`../VERSION`](../VERSION).

- release tags are formatted as `vX.Y.Z`
- the `VERSION` file is the default source of truth for release packaging and build metadata
- `MGA_VERSION` can still override builds in CI or ad hoc packaging flows

### Pre-1.0 interpretation

MGA is still pre-1.0.

That means:

- feature velocity is still more important than strict compatibility
- breaking changes are still possible
- every tagged release should still document them clearly

The working rule is:

- **patch**: bug fixes, polish, narrow behavior changes
- **minor**: feature additions, packaging changes, UX shifts, larger behavior changes
- **major**: reserved for the first stable compatibility contract

## Release policy

Every tagged release should include:

1. a repository version bump in `VERSION`
2. a Git tag in the form `vX.Y.Z`
3. release notes with user-visible changes
4. upgrade notes if runtime layout, schema, or sync behavior changed
5. packaged artifacts once packaging lands

The current public release flow is a tag-triggered GitHub Release for Windows portable artifacts.

## Upgrade policy

MGA should treat local user data conservatively.

Principles:

- never silently delete user data during upgrade
- prefer additive, idempotent schema changes
- make one-way migrations explicit in release notes
- document any runtime directory change before shipping it

## Portable upgrade flow

Until a real installer exists, MGA upgrades should assume a portable runtime.

Recommended user flow:

1. stop MGA
2. back up `config.json`
3. back up `data/`
4. back up `media/` if local media or overrides matter
5. replace binaries and shipped assets with the new release
6. start MGA and verify the About page version/build metadata

## Packaging policy

The intended order is:

1. portable builds
2. installer-ready runtime layout
3. platform installers

Installer work should not happen before MGA has a clean answer for writable runtime locations on:

- Windows (`%LOCALAPPDATA%` / `%APPDATA%`)
- Linux (XDG-style paths)
- macOS (`~/Library/Application Support/...`)

## Migration notes expectation

Any release that changes one of the following must carry explicit migration notes:

- database schema behavior
- config keys or runtime directory layout
- sync payload structure or compatibility expectations
- plugin discovery/runtime location assumptions
- network binding or localhost/LAN behavior
