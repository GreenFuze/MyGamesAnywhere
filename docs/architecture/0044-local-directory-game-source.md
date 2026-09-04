# ADR-0044: Local Directory Game Source

Status: Accepted for implementation
Date: 2026-09-04
Jira: MGA-113
Canonical decision: Confluence page `ADR-0044 — Local Directory Game Source`

This local mirror records the config contract and containment rules next to the code. Confluence remains authoritative.

## Context

MGA had no way to connect a folder on the server's own disk as a game source. Sources were Google Drive, SMB, Epic, Steam and Xbox; `save-sync-local-disk` is saves-only and has no configurable path. An operator whose library sits on the server, an attached drive, or a synced folder could not get it into the catalog at all.

## Decision

- A new in-module plugin `game-source-local` (directory `server/plugins/local`, binary `local.exe`) provides `source.filesystem.list`, `source.filesystem.delete`, `source.browse` and `plugin.check_config`.
- The connection is one **absolute** `base_path` plus `include_paths` that are **relative** to it. Two drives means two connections.
- `game-source-local` joins `sourcescope.IsFilesystemBackedPlugin`, which is what enables include-path normalization, scan-scope reconciliation, duplicate detection, file validation and destructive delete.
- Content delivery serves local files directly, resolving each file against the connection's configured base — the same approach `openSMBFile` already uses.
- Move-destination support (`source.transfer.*`) is deliberately **not** declared. Follow-on work.

## Why the config key is `base_path` and not `root_path`

`sourcescope.NormalizeConfig` deletes `path`, `root_path` and `exclude_paths` for every filesystem-backed plugin, and `controllers.go` normalizes *before* validating against the manifest schema. A required `root_path` would therefore fail every create and update with `field "root_path" is required`. The frontend strips the same keys.

It would also break duplicate detection: `findDuplicateIntegration` compares normalized config JSON, so with `root_path` stripped two connections rooted at different drives would compare equal and the second would be rejected as a duplicate.

`base_path` is untouched by any normalizer and still matches the console's `endsWith('_path')` rule that gives a field its folder picker.

## Why one absolute base per connection

`sourcescope.NormalizeLogicalPath` strips a leading `/`. Storing `/mnt/games` as an include path would silently yield the relative `mnt/games` and match the wrong subtree, while `C:/Games` would survive — Windows passes, Linux corrupts. Keeping include paths relative to one absolute base mirrors SMB's share exactly and leaves every `sourcescope` function correct on both platforms.

For the same reason `legacyPathKey` returns `""` for this plugin. Wiring `base_path` in there would reintroduce the bug.

## Why delivery resolves against the base, not `RootPath`

The generic direct branch in `contentdelivery` joins `sourceGame.RootPath` with `file.Path`. But the scanner sets `RootPath` to `GameGroup.RootDir`, which is *relative*, while `GameFile.Path` is relative to the source root and already contains `RootDir` as a prefix — joining them counts the group directory twice. The only fixture exercising that branch hand-wrote a convention the scanner never produces, so the path was dormant and had never run end to end.

`openLocalFile` therefore ignores `RootPath` for delivery and resolves `file.Path` against `base_path`, exactly as `openSMBFile` resolves against the share. Four gates must all agree before a byte is read: the integration must belong to the requesting profile, the connection must declare an absolute base, the file must sit inside `include_paths`, and it must still resolve inside the base after every symlink on both sides is followed.

`supportsDirectSourceGame` in `contentdelivery` names the plugin explicitly, because the scanner's relative `RootPath` means the absolute-path rule never fires for a local source. Note that the similarly named functions in `sourcecache` and `http/play_support` answer a *different* question — "is there already a real local file a runtime can open by path", for which an SMB share does not qualify — and are deliberately left separate.

## Links are never followed

Symlinks, junctions and other reparse points are skipped during listing (neither emitted nor descended), refused during deletion, and cannot be resolved through at delivery time. Emitting a link would create a row that delivery resolves elsewhere and deletion refuses to touch — permanently stuck. Windows reparse points also cover cloud placeholders such as OneDrive "online-only" files, where reading would trigger a network hydrate.

The walker additionally carries a visited set keyed on the canonicalized directory, a depth cap of 64 and an entry cap of 2,000,000. Exceeding either returns `SCAN_FAILED` telling the operator to narrow `include_paths`. A refusal beats a hang.

Containment errors name only the logical path the caller supplied, never the resolved target — a link out of the base must not become a way to read the server's directory layout out of an error message.

## Source identity

`plugin.check_config` returns `"localdir:" + sha256(EvalSymlinks(base_path))`, lowercased only on Windows: NTFS treats `Games` and `games` as one folder while a POSIX filesystem treats them as two. Canonicalizing first collapses junction and symlink aliases. Hashing keeps the server's filesystem layout out of API responses.

If the base cannot be resolved, `check_config` returns an error status and **no** identity. The server skips duplicate detection on an empty identity, so guessing one would let an unverified folder past that check.

Known limits, stated rather than hidden: this does not see through two drive letters mapped to the same network share, bind mounts, or `subst` drives.

## Destructive delete

Ported from SMB with every invariant intact: `root_path` required and inside include scope; each file inside `root_path` matched on a full segment so `SNES` never authorizes deleting under `SNES Extras`; a full `Lstat` preflight over the whole plan before any mutation, so one refused item deletes nothing; symlink and reparse refusal; files first, then directories deepest-first; directories pruned only when they read back empty, otherwise kept with a warning naming unauthorized files.

Containment is re-resolved inside each filesystem method rather than only when the plan was built, so a path is checked at the moment it is acted on.

## Migration

`NO_MIGRATION_NEEDED`. No schema change; `integrations`, `source_games` and `game_files` keep their existing shapes. `base_path` is a new key on `integrations.config_json` for a new `plugin_id` only, and `NormalizeConfig`'s delete-list is unchanged, so no existing SMB or Drive row is reinterpreted. The `IsFilesystemBackedPlugin` addition is plugin-scoped and no `game-source-local` row exists in any real deployment.

`cmd/publicdemo` seeds a demo integration with this plugin id and an empty config. It becomes filesystem-backed, which is safe: it writes its scan batch directly and never calls `source.filesystem.list`, its file inventory is empty so hard delete still refuses, and its `RootPath` is already absolute so its delivery mode is unchanged.
