# Next Release Notes (Development)

These notes track upgrade-sensitive work after v0.2.2. They are renamed or
folded into the numbered release notes when the release version is selected.

## Changes

- Added stable server-side library sorting before pagination, including title,
  release date, platform, and rating order.
- Added Library and Play grouping by platform, connection, play option,
  achievements, or release year. A game can intentionally appear in more than
  one group when it belongs to multiple connections or play options.
- Added clearer cover badges for play options, achievements, and connections.
- Added dedicated Play shelves for browser and cloud play, while retaining
  favorites, recent games, and custom shelves.
- Simplified Settings terminology and card layouts for players, with advanced
  identifiers and protocol details moved behind expandable details or action
  menus.
- Added reusable compact status, tooltip, and overflow-action UI components.
- Added conservative version-aware game identity. MGA now keeps every source
  copy, distinguishes concrete Editions from shared Titles, and no longer
  combines games from name matching alone.
- Added player-facing Version, Copies, and Match facts to game details, with
  stable IDs and evidence counts available under Technical details.
- Added automatic and manual device scans using one bounded MGA Client
  inventory collector. Devices now show free storage and recognized game apps.
- Added per-game device readiness so game details can distinguish ready for
  setup, missing runtime, not scanned, offline, update required, and unsupported.
- Added transactional ZIP installation and guarded uninstall through MGA Client,
  including live progress and per-device installed state.
- Added separate blue/cyan Download and purple Install percentages backed by
  MGA Client command progress.
- Added safe executable discovery, explicit launch-target selection when
  candidates are ambiguous, and Play on device through the authenticated
  server/client command flow.

## Upgrade and migration notes

- Database migration 13 adds `game_titles`, `game_editions`, and
  `game_title_external_ids`. Existing canonical IDs are preserved as Edition
  IDs, so URLs, favorites, achievements, save references, and manual grouping
  pins remain valid. MGA backs up SQLite before applying pending migrations.
- Database migration 14 adds additive, endpoint-owned inventory snapshots.
  Existing paired clients remain valid and show Not scanned until an updated
  client reports; pairing keys, grants, and sync payloads are unchanged.
- Database migration 15 adds device-command progress and game-installation
  records. Existing commands and paired clients remain valid; clients without
  the new capabilities are not offered archive actions.
- Database migration 16 additively stores staged progress and per-install
  executable candidates/selection. Existing rows remain valid with empty launch
  metadata. New installs use manifest schema 2; schema-1 managed installs remain
  safely uninstallable and can be reinstalled to enable Play.
- Library grouping remains in the existing extensible frontend-preferences
  JSON. Existing profiles default to no grouping, and the removed Timeline
  view maps to Covers grouped by release year. Settings-sync/save-sync payloads
  are unchanged.
