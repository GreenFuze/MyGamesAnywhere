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
  local/
    game-source-local.plugin.json          # plugin_id: game-source-local
    bin/local.exe                          # exec matches the directory name
```

## Reporting progress during a long call

A plugin may report while a call is still running by writing an extra frame
with the same request id and a `progress` object instead of a `result`:

```json
{"id": "<request id>", "progress": {"current": 250, "total": 1000, "unit": "items", "item": "Reading Games…"}}
```

`total` is optional — a filesystem walk knows how many entries it has seen but
not how many remain, and a provider fetch may only be able to name the step it
is on. A report with no `total` still distinguishes working from stuck, which is
the point.

The host correlates these by request id, forwards them to whoever asked for
progress on that call, and ignores ids it does not recognise. A progress frame
never completes a call. Reporting is optional in both directions: a plugin that
reports nothing behaves exactly as before, and a plugin that reports to a caller
that is not listening is unaffected.

Keep the reports coarse — every few hundred items — and remember they share
stdout with responses, so guard the write if the plugin has more than one
goroutine.
