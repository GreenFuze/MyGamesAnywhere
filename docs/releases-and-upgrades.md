# Releases and Upgrades

## Versioning policy

MGA uses a repository-level version file at [`../VERSION`](../VERSION).

- stable release tags are formatted as `vX.Y.Z`
- beta/release-candidate tags use SemVer prerelease suffixes such as `vX.Y.Z-beta` or `vX.Y.Z-rc.1`
- the `VERSION` file is the default source of truth for release packaging and build metadata
- `MGA_VERSION` can still override builds in ad hoc packaging flows

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

1. choose the release version, for example `0.0.12`
2. build the portable package locally with `./server/package-portable.ps1 -Version <version>`
3. build the Windows installer and update manifest with `./server/package-installer.ps1 -Version <version> -SkipBuild -ReleaseBaseUrl https://github.com/GreenFuze/MyGamesAnywhere/releases/download/v<version>`
4. publish the GitHub Release manually with `gh release create v<version>` and upload the generated artifacts
5. mark beta builds as prereleases and stable builds as latest

GitHub Actions packaging workflows have been removed. Releases are built locally and published manually with `gh`.

## Upgrade policy

MGA should treat local user data conservatively.

Principles:

- never silently delete user data during upgrade
- prefer explicit, versioned SQLite migrations
- make one-way migrations explicit in release notes
- document any runtime directory change before shipping it
- persisted schema/data/config changes must include a migration or a `NO_MIGRATION_NEEDED` note

## SQLite migration policy

The server owns database migrations. Migrations are ordered, versioned, and
recorded in `schema_migrations` with name, checksum, applied time, duration,
and success state.

Startup still runs pending migrations for dev and manual portable runs, but it
creates a SQLite backup first. Packaged update flows run:

```text
mga_server --migrate-only --config <config> --data-dir <data> --app-dir <app>
```

before declaring the update successful. `--migrate-only` validates config,
connects the database, applies migrations, and exits without starting HTTP,
plugins, tray UI, scans, or background workers.

Fail-fast cases:

- the DB contains a migration version newer than this binary supports
- an applied migration has a checksum mismatch
- a previous migration is marked failed
- a migration cannot be applied cleanly

Future DB schema changes, persisted SQLite data transforms, and persisted JSON
config changes require either a migration or an explicit `NO_MIGRATION_NEEDED`
note explaining why existing installs remain safe. Run
`./server/scripts/check-migration-guard.ps1` locally before release to catch
persistence-adjacent changes without either signal.

## Portable upgrade flow

Portable builds remain supported and self-contained. Auto-update v1 can check
the release manifest, download/verify the portable ZIP, and launch an external
Windows updater script that restarts MGA while replacing immutable app files.

Recommended user flow:

1. open Settings -> Update
2. check for updates
3. download and verify the portable ZIP
4. apply the update and wait for MGA to restart
5. verify the Settings/About version metadata

The portable updater preserves `config.json`, `data/`, `media/`,
`source_cache/`, `updates/`, and `logs/`.

During update, the portable updater backs up immutable app files and the SQLite
DB triplet (`db.sqlite`, `db.sqlite-wal`, `db.sqlite-shm`) before copying the
new package. If `--migrate-only` fails, it restores the previous app files and
DB triplet and restarts the old MGA.

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

Installed Windows updates launch the verified installer in silent update mode.
Per-user installs stop and restart the user-mode server process. All-users
installs stop and restart the Windows service. Portable Windows updates use the
packaged updater script instead of the installer.

Installed update mode backs up the previous app directory and SQLite DB triplet
before replacing files. After file copy it runs `mga_server --migrate-only`. On
migration failure, the helper restores the previous binaries and DB files, then
restarts the old process/service and logs the detailed failure under the update
log directory.

## Migration notes expectation

Any release that changes one of the following must carry explicit migration notes:

- database schema behavior
- config keys or runtime directory layout
- sync payload structure or compatibility expectations
- plugin discovery/runtime location assumptions
- network binding or localhost/LAN behavior
