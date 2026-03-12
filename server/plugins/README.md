# Plugins

Plugins are discovered from this directory: one subdirectory per “plugin package”. A package can expose **one or more plugin IDs** via manifest files.

## Manifest files

- **`*.plugin.json`** — Each file is one plugin manifest (one `plugin_id`, `exec`, `provides`, etc.).
- **`plugin.json`** — Optional; if present, it is also loaded as a single manifest (backward compatibility).

Same directory can host multiple manifests that share the same `exec` binary (e.g. Drive: game source + sync settings).

## Plugin ID convention

Use **lowercase, hyphenated IDs**; no reverse-DNS (no dots).

- Good: `game-source-smb`, `game-source-google-drive`, `sync-settings-google-drive`, `game-source-mock`
- Bad: `com.mga.drive`, `com.example.plugin`

Pattern: `^[a-z][a-z0-9-]*$`. IDs that do not match are rejected at discovery (logged and skipped).

## Layout example

```
plugins/
  drive/
    game-source-google-drive.plugin.json   # plugin_id: game-source-google-drive
    sync-settings-google-drive.plugin.json # plugin_id: sync-settings-google-drive
    bin/drive.exe                         # shared exec
  smb/
    game-source-smb.plugin.json
    bin/smb.exe
```
