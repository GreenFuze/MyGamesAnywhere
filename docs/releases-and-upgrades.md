# Releases and Upgrades

## Versioning policy

MGA uses a repository-level version file at [`../VERSION`](../VERSION).

- stable release tags are formatted as `vX.Y.Z`
- beta/release-candidate tags use SemVer prerelease suffixes such as `vX.Y.Z-beta` or `vX.Y.Z-rc.1`
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

1. a repository version bump in `VERSION`, or an explicit `MGA_VERSION` / packaging `-Version` override for beta validation artifacts
2. a GitHub release tag in SemVer form such as `vX.Y.Z` or `vX.Y.Z-beta`
3. release notes with user-visible changes
4. upgrade notes if runtime layout, schema, or sync behavior changed
5. packaged artifacts:
   - `mga-vX.Y.Z[-prerelease]-windows-amd64-portable.zip`
   - `mga-vX.Y.Z[-prerelease]-windows-amd64-installer.exe`
   - `mga-update.json`
   - `SHA256SUMS.txt`

The current public release flow is:

1. choose the release version, for example `0.0.8-beta`
2. build the portable package locally with `./server/package-portable.ps1 -Version <version>`
3. build the Windows installer and update manifest with `./server/package-installer.ps1 -Version <version> -SkipBuild -ReleaseBaseUrl https://github.com/GreenFuze/MyGamesAnywhere/releases/download/v<version>`
4. publish the GitHub Release manually with `gh release create v<version>` and upload the generated artifacts
5. mark beta builds as prereleases and stable builds as latest

GitHub Actions remains available as an opt-in packaging helper via manual workflow dispatch, but it no longer publishes releases automatically from pushed tags.

## Upgrade policy

MGA should treat local user data conservatively.

Principles:

- never silently delete user data during upgrade
- prefer additive, idempotent schema changes
- make one-way migrations explicit in release notes
- document any runtime directory change before shipping it

## Portable upgrade flow

Portable builds remain supported and self-contained. Auto-update v1 can check
the release manifest and download/verify the portable ZIP, but portable
self-replacement is intentionally manual.

Recommended user flow:

1. stop MGA
2. back up `config.json`
3. back up `data/`
4. back up `media/` if local media or overrides matter
5. replace binaries and shipped assets with the new release
6. start MGA and verify the About page version/build metadata

## Packaging policy

The supported Windows packaging modes are:

1. portable ZIP, with app files and mutable data beside `mga_server.exe`
2. per-user installer, with app files under `%LOCALAPPDATA%\Programs\MyGamesAnywhere` and mutable data under `%LOCALAPPDATA%\MyGamesAnywhere`
3. all-users/service installer, with app files under `%ProgramFiles%\MyGamesAnywhere` and mutable data under `%ProgramData%\MyGamesAnywhere`

Linux packaging is deferred, but the runtime path abstraction should map to:

- config: `$XDG_CONFIG_HOME/mga`
- data: `$XDG_DATA_HOME/mga`
- cache: `$XDG_CACHE_HOME/mga`

Windows installer packaging uses Inno Setup. Inno Setup is open source under
its own license terms; keep this attribution in package docs and NOTICE, and
decide later whether to buy a commercial license for project compliance comfort.

## Auto-update policy

Auto-update checks use SemVer precedence. A stable release is newer than a
prerelease with the same numeric version, so an installed `v0.0.8-beta` build
will detect `v0.0.8` as an available update once the stable manifest is
published. Build metadata such as `+build.1` is ignored for precedence.

## Migration notes expectation

Any release that changes one of the following must carry explicit migration notes:

- database schema behavior
- config keys or runtime directory layout
- sync payload structure or compatibility expectations
- plugin discovery/runtime location assumptions
- network binding or localhost/LAN behavior
