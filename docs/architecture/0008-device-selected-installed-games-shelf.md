# ADR-0008: Device-selected Installed Games Play shelf

- **Status:** Accepted implementation contract; not implemented
- **Date:** 2026-07-15
- **Scope:** Play-page device association and installed-game shelf

## Context

MGA persists installations per `(endpoint, game, source copy)`, but the Play page
currently leads with browser/profile shelves and does not answer “what can I
play on this device?” The browser already has a profile-scoped endpoint
association created by the signed `mga://start` flow:

```text
mga.clientEndpoint.<profile-id>
```

That association represents the current browser/profile's selected per-user MGA
Client endpoint. It is a browser preference, not a physical-host identity or
authorization grant.

## Decision

### Selected/current device

The Installed Games shelf uses one explicit endpoint:

1. Use the existing profile-scoped browser association when it still identifies
   an authorized endpoint.
2. If the profile has exactly one authorized endpoint, use it as a non-persisted
   fallback.
3. With multiple endpoints and no valid association, show a device selector and
   do not aggregate installations across devices.
4. Selecting a device writes the existing local-storage key and updates both
   top-bar client status and Play shelf immediately through one shared
   association module/hook and a same-tab change event.
5. Never infer “this device” from host name, browser IP, hardware fingerprint,
   most recently seen endpoint, or another profile's association.

`ClientEndpointAssociation` moves out of `ClientStatusControl.tsx` into a shared
frontend module. Invalid/revoked IDs are ignored; the server remains the
authorization authority.

### Shelf order and visibility

On the root `/play` page, when no focused custom section is open:

1. **Installed Games** for the selected device is the first shelf.
2. **Continue Playing** follows.
3. Existing Favorites, Play in browser, Cloud play, and custom shelves follow
   in their existing relative order.

The Installed Games section remains first in root Play grid/group/shelf
presentations; it is omitted only on a focused `/play/section/...` route. It is
rendered even when empty so device selection and state remain understandable.

Header:

```text
Installed Games · <device display name>
```

With several authorized endpoints, a compact device selector is available in
the header. This is player-facing **Device**, not Endpoint.

### Installed item eligibility

The shelf is based on persisted installations for the selected endpoint and
active profile.

Included:

- `install_state=installed`;
- both `managed_archive` and `gog_inno`;
- connected or offline endpoints;
- installations with or without a selected launch target.

Excluded from Installed Games:

- `attention_required`
- `cleanup_required`
- `cleanup_running`
- `cleanup_failed`
- `ignored_failure`
- future `missing` or `needs_repair`

Excluded states contribute to `attention_count`. The shelf shows a compact
“N games need attention” link to the selected device's management/details
surface at `/settings?tab=devices&device=<endpoint-id>`. Settings must select
the Devices tab and expand that device from these query parameters. The shelf
does not mislabel failed/missing content as installed.

One canonical game appears once per device. If several source copies are
installed, choose the shelf action source deterministically:

1. state Installed with a non-empty recorded launch target;
2. newest installation `updated_at`;
3. lexical `source_game_id` tie-break.

Other installed copies remain visible on game detail. This deduplication is a
shelf presentation rule only and never merges/deletes source identities.

Sort shelf games by normalized player-visible title, then canonical game ID for
stable ties.

### Server API

Add:

```text
GET /api/play/devices/{id}/installed-games
```

It uses authenticated profile context and requires at least View access to the
endpoint. It does not accept profile, source, state, or sort overrides.

Response:

```json
{
  "device": {
    "id": "...",
    "display_name": "...",
    "status": "ready",
    "connected": true,
    "access_level": "play"
  },
  "games": [
    {
      "game": {},
      "source_game_id": "...",
      "install_kind": "managed_archive",
      "install_state": "installed",
      "launch_target": "Game/Game.exe",
      "launch_supported": true,
      "can_play": true,
      "installed_at": "...",
      "updated_at": "..."
    }
  ],
  "attention_count": 0
}
```

`game` uses the existing `GameDetailResponse` card shape. The endpoint query is
profile-scoped, deduplicated, and server-stably sorted before response. View
access can list; `can_play` additionally requires Play access, live compatible
connection, `game.launch` capability, Installed state, and a recorded launch
target. The launch endpoint still re-enforces authorization/capability and the
client revalidates the manifest.

### Card actions and states

- `can_play=true`: primary **Play** dispatches the existing typed device launch,
  tracks the command, and reports the same player-facing success/failure used on
  game detail.
- Offline: card remains in the shelf with **Offline**; Play is disabled and the
  action opens client/device controls.
- Update required: **Needs update**; Play disabled.
- Installed without launch target: **Choose executable** opens game detail at
  the device section; it is not treated as a failure.
- View-only access: installed state is visible; Play is disabled.
- Card/title navigation always opens the existing game detail page with normal
  route state.

The UI must not launch a different endpoint or source as fallback when the
selected one cannot play.

### Empty and error states

- Multiple devices, none selected: “Choose a device to see installed games.”
- Selected device, zero Installed rows: “No games installed on this device.”
- Offline selected device: retain known installed games and explain that the
  device is offline.
- Inventory/API failure: “MGA couldn't check installed games on this device.”
  with Retry; do not show a false empty shelf.
- Revoked/deleted selection: ignore stale association and return to device
  selection/single-device fallback.

## Persistence and migration

`NO_MIGRATION_NEEDED`:

- installation state/device ownership already exists in migration 17/18;
- the browser association uses the existing profile-scoped local-storage key;
- no SQLite, client config, sync payload, or manifest shape changes;
- manual selection adds no new storage key/version.

If the association shape later stores more than one endpoint ID string, it will
require a versioned browser-preference migration.

## Bounded implementation packet

### In scope

- shared `ClientEndpointAssociation` module/hook and same-tab change event;
- manual device selector using existing authorized device list;
- profile-scoped installed-games server query/DTO/route;
- deterministic canonical/source selection and stable sorting;
- Installed Games shelf first on root Play;
- direct existing `game.launch` action and command tracking;
- offline/update/no-target/view-only/attention/empty/error behavior;
- attention deep link selects Devices and expands the selected endpoint;
- OpenAPI/contracts, accessibility, tests, production build, packaged browser
  E2E, and documentation.

Likely surfaces:

- `server/frontend/src/components/devices/ClientStatusControl.tsx`
- new shared association hook/module;
- `server/frontend/src/pages/LibraryPage.tsx`
- `server/frontend/src/components/library/PlayRouteShelves.tsx` or a dedicated
  Installed Games shelf component;
- `server/frontend/src/api/client.ts` and generated contracts;
- server game/device query service, controller, router, tests, and OpenAPI.

### Out of scope

- cross-device aggregate shelf;
- physical-host inference;
- changing endpoint grants or signed `mga://start`;
- installing, repairing, cleaning, forgetting, or reconciling from the shelf;
- showing failed/missing/cleanup rows as installed;
- remembering per-game preferred device/source;
- changing library filters/grouping semantics;
- redesigning other Play shelves beyond placing Installed first;
- changing GOG install/uninstall/cleanup consent policy (ADR-0007 governs it).

### Required tests

Server:

- profile/device access isolation and not-found/forbidden mapping;
- Installed-only state filtering and attention count;
- archive + GOG inclusion;
- canonical dedupe/source precedence/tie-break;
- stable title/ID sorting;
- `can_play` for connected/capability/access/target combinations;
- no cross-profile/cross-device leakage.

Frontend:

- existing association, one-device fallback, multi-device chooser, stale ID;
- top bar and shelf update from one shared association event;
- Installed shelf renders before Continue Playing in root Play;
- focused custom section omits it;
- direct Play dispatches selected endpoint/source only;
- offline/update/view-only/no-target states;
- attention link/count and empty/error/retry states;
- Settings device deep-link parsing/expansion;
- accessible selector/buttons/status text.

Build:

- full server tests;
- frontend production build;
- OpenAPI generation/test;
- `git diff --check`.

### Packaged E2E

- Use packaged server and installed client; no npm dev server.
- Preserve credentials, Drive Desktop configuration, and existing installations.
- Associate/select the real current endpoint and verify Installed Games is the
  first Play shelf.
- Verify Plasma Pong appears once for that endpoint.
- Click its shelf Play action, verify successful `game.launch` command and exact
  executable PID/path, then terminate only that exact game process if running.
- Verify switching to another authorized endpoint (if available) changes the
  shelf without leaking the first endpoint's installations; otherwise verify
  the single-device fallback and chooser unit tests.
- Verify an attention row is excluded and increments the attention link without
  mutating it.
- Leave packaged server/client running and Plasma Pong installed.

## Stop and escalation conditions

Stop before:

- inventing a second association key/shape or server-persisted device preference;
- aggregating multiple devices;
- changing grants, endpoint identity, launch authorization, or source identity;
- adding repair/cleanup/reconciliation actions to the shelf;
- treating attention/missing/no-target as playable;
- changing other shelf policy beyond the recorded order;
- needing a database/config migration;
- auto-selecting a device using host/network/hardware inference.
