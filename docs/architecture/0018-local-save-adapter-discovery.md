# ADR-0018: Local save adapter discovery

- **Status:** Accepted
- **Date:** 2026-07-17
- **Scope:** MGA Client inventory, device protocol, server persistence, emulator settings

## Context

ADR-0017 classifies emulator saves as local files but deliberately does not
guess their location. MGA now needs to distinguish an emulator it merely sees
from one whose save configuration it can resolve safely. Discovery must work
per device and OS user, survive a disconnected client, and avoid sending local
paths to the browser.

RetroArch can use separate save-RAM and save-state directories, and its
core/content/game override hierarchy may change settings for one route.
ScummVM has a global save path plus optional per-game `savepath` overrides.
Neither model permits a title-only guess.

## Decision

### Inventory schema 3 and migration 25

Device inventory schema 3 adds a bounded `save_adapters` collection. Each fact
contains only a stable adapter/runtime ID, player-facing name, probe state,
supported save kinds, and whether route-specific overrides were observed. It
contains no executable, config, save, content, or user-profile path.

Migration 25 adds `device_inventories.save_adapters_json`. This is separate
from runtime discovery so old schema-1/2 snapshots remain valid and so the
server can retain adapter readiness while a client is offline. Migrations
22-24 remain immutable.

Probe state means:

- `complete`: the client resolved the known configuration boundary;
- `partial`: the runtime was found but configuration could not be read or
  validated completely;
- `unsupported`: MGA has no bounded adapter for that detected runtime; and
- `unknown`: an older client or incomplete snapshot supplied no evidence.

`complete` does not authorize copying. It only means a later route-specific
operation can attempt fresh resolution.

### Initial adapters

1. **RetroArch** reports `save_ram` and `save_state`. The client reads only the
   selected bounded RetroArch configuration and known directory keys. It
   reports whether the configured override tree contains route overrides, but
   not their names or paths.
2. **ScummVM** reports `save_file`. The client checks the documented portable
   configuration beside the executable and the OS-user configuration location.
   It recognizes global and per-game `savepath` entries without returning their
   values.
3. A missing configuration file is not itself an error when the emulator has a
   documented default. An unreadable, oversized, malformed, or unresolved
   configured location produces `partial` and fails closed.

All discovery is read-only, bounded to fixed emulator-owned locations, and
performed by MGA Client under the paired OS user. The server cannot provide a
path or broaden the probe.

### Player experience

Settings > Emulators shows **Save support detected**, **Save setup needs
attention**, or **Save support unknown** for the selected device/user. Game
details remain conservative: discovery alone does not enable backup/restore or
claim two routes are compatible.

Actual snapshot, restore, conflict handling, route binding, and conversion are
a later command-family ADR. Those operations must re-resolve paths on the
client, identify the exact route, use staged writes and rollback, and require
explicit user confirmation before replacing local data.

## Compatibility and security

- Schema 1 and 2 remain accepted. They cannot contain save-adapter facts.
- Schema 3 facts are additive and ignored by older servers.
- Paths and filenames are never persisted or returned to the browser.
- Adapter and capability IDs are fixed allow-lists with bounded counts.
- Trusted-LAN HTTP and remote MGA servers remain supported; discovery happens
  on the paired endpoint and assumes no shared filesystem.

## Acceptance criteria

- Protocol validation covers schemas 1-3, duplicate adapters, invalid states,
  and unknown capabilities.
- Client tests cover documented defaults, portable/user configuration,
  overrides, unreadable/oversized files, and absence of paths in the payload.
- Migration 25 upgrades existing inventory rows with an empty adapter list and
  round-trips schema-3 facts.
- Emulator settings display adapter readiness without enabling save writes.
- Full protocol, client, server, plugin, frontend, packaging, and real-device
  checks pass before the next checkpoint.

## Upstream evidence

- RetroArch directory configuration:
  <https://docs.libretro.com/guides/change-directories/>
- RetroArch override hierarchy:
  <https://docs.libretro.com/guides/overrides/>
- ScummVM configuration locations and per-game settings:
  <https://docs.scummvm.org/en/v2.9.1/advanced_topics/configuration_file.html>
- ScummVM save-path setting:
  <https://docs.scummvm.org/en/v2.8.0/settings/paths.html>

