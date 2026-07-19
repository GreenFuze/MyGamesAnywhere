# ADR-0029: Google Drive "Shared with me" source folders

- **Status:** Accepted
- **Date:** 2026-07-19
- **Scope:** Google Drive game-source browsing, persisted source scope, and scans

## Player outcome

A player connecting Google Drive can choose a game folder from either My Drive
or the account's **Shared with me** collection. The picker makes the two
locations visibly distinct. Existing My Drive connections continue to behave
exactly as before.

This decision does not add Google Workspace Shared Drives. Those are a separate
provider collection with different membership and API semantics.

## Decision

1. The Google Drive folder picker exposes **Shared with me** as a virtual browse
   location alongside the folders in My Drive. The virtual collection itself is
   not selectable; the player selects a concrete folder inside it.
2. A selected shared folder is persisted with both:
   - a friendly logical `path`, used in the UI and MGA library paths; and
   - its stable Google Drive file ID in `include_paths[].object_id`.
3. Scans use `object_id` as the authoritative root when it is present. Folder
   names and later renames do not change which provider object MGA scans.
   Connections without `object_id` retain the existing My Drive path resolver.
4. Browse navigation may use an opaque, non-persisted token containing the
   folder ID and friendly path. The token is validated and grants no access by
   itself: every API call still uses the exact profile-owned connection token,
   and Google enforces that account's access.
5. Listing the Shared with me collection uses the Drive API `sharedWithMe`
   query term and follows every result page. Child folders are then browsed by
   their parent file ID.
6. Settings Sync and Save Sync remain rooted in My Drive because they create or
   update files. Their folder pickers do not offer Shared with me. This ADR only
   broadens the read-oriented Google Drive game source.
7. Invalid, inaccessible, trashed, or non-folder persisted object IDs fail fast
   with a concrete scan error. MGA never silently falls back to a same-named My
   Drive folder.

Google documents `sharedWithMe` as the query term for files in the authorized
user's Shared with me collection and documents pagination on `files.list`:

- <https://developers.google.com/workspace/drive/api/guides/search-files>
- <https://developers.google.com/workspace/drive/api/reference/rest/v3/files/list>

## Persistence and compatibility

`NO_MIGRATION_NEEDED`: `object_id` is an additive member of the existing JSON
objects stored in an integration's `config_json`. SQLite tables and meanings do
not change. Existing configurations omit the member and keep the My Drive path
behavior. New code preserves it only for `game-source-google-drive`; SMB scopes
cannot acquire Google object IDs through normalization.

Rollback is data-safe. An older MGA version cannot scan a Shared with me scope
by stable ID and may strip the unknown member if that connection is edited, but
it does not corrupt the database or affect old My Drive scopes. Re-selecting the
shared folder after returning to a supporting version restores the ID.

## Acceptance criteria

- [x] My Drive browsing and existing path-only scans are unchanged.
- [x] The game-source picker visibly offers Shared with me after Google sign-in.
- [x] Shared folder browsing is paginated and concrete selections persist a
      friendly path plus stable object ID.
- [x] A shared-folder scan begins at the stored object ID and returns logical
      paths beneath the friendly selected path.
- [x] Renaming or duplicating a shared folder name cannot redirect a configured
      connection to a different provider object.
- [x] Settings Sync and Save Sync do not offer the Shared with me location.
- [x] Profile-owned OAuth isolation remains enforced by the existing browse
      controller and is covered by regression tests.
- [x] Focused plugin, scope, controller, frontend unit, and production build
      checks pass.

## Verification evidence

The packaged portable server was rebuilt and run from `server/bin`, not from a
Vite or `go run` development process. Using the existing TCs profile-owned
Google Drive connection, the real folder picker showed My Drive's folders plus
a distinct **Shared with me** location. Opening it returned the authorized
account's real shared folders; a concrete folder was selectable and produced
the friendly path `Shared with me/876081`. The dialog was cancelled, leaving
the existing `Games` source connection unchanged. The Settings Sync picker was
also opened at My Drive root and contained no Shared with me entry.

Automated evidence:

```text
server:          go test ./...                         PASS
Drive + scope:   go test ./plugins/drive ./internal/sourcescope PASS
client:          go test ./...                         PASS
protocol:        go test ./...                         PASS
frontend:        test:unit                             PASS (16 tests)
frontend:        build                                 PASS
migration guard: check-migration-guard.ps1             PASS
quality:         gofmt + git diff --check              PASS
server package:  build.ps1                             PASS
```

The production build retains the known large-main-chunk warning. No connection
configuration or provider data was changed during the real UI verification.
