# ADR-0014: Multi-route Play control

- **Status:** Implemented and locally verified
- **Date:** 2026-07-17
- **Scope:** Selecting how and where a game is played from a game card

## Context

A game does not have one universal Open action. The same edition or library
item may be playable through several routes at the same time, including:

- more than one compatible local emulator;
- an in-browser emulator;
- a native or storefront-managed local installation;
- Xbox Cloud Gaming or another cloud/remote route.

Those routes may differ in execution target, save-domain compatibility,
achievements or RetroAchievements support, controller behavior, and required
preparation. Installing a locally playable copy must not erase an available
cloud route, and discovering an in-browser route must not hide compatible local
emulators.

## Decision

### One Play control, several explicit routes

Game cards use a split **Play** control when more than one route or preparation
choice is available:

- the main segment executes the current default available action;
- the down-arrow segment lists every other available action;
- every menu entry names the route or target, such as **Play in browser**,
  **Play in xCloud**, **Play with RetroArch**, or **Play on Living Room PC**;
- **Details** remains a separate secondary action and is not a play route.

The generic card action is never **Open**. A card with one route uses that
route-specific Play label. A card with no playable route may show **Install**,
**Set up**, or **Details**, according to its real state; MGA does not label a
non-playable action as Play.

The route list is intentionally not deduplicated by source, platform, or title.
Several emulator choices may coexist, as may local installation, browser play,
and cloud play. Each action retains its edition/library-item identity, route
type, execution target, and relevant capability facts. Selecting one action
must not silently fall back to a different route.

An emulator default is only the primary choice within an ordered compatible
emulator list. It does not remove the other emulator routes. Route labels and
details expose meaningful differences such as RetroAchievements support,
required firmware/core, save compatibility, and device readiness.

### Initial default selection

The initial default is deterministic and contextual:

1. A route-specific shelf makes that route primary: **Play in browser** chooses
   the browser route, **Cloud play** chooses the cloud route, and an installed
   device shelf chooses the action for that selected device/OS-user endpoint.
2. Outside a route-specific context, an explicitly supplied ready action wins.
3. Otherwise the first currently available browser route precedes cloud play.

Disabled or attention-requiring contextual actions may remain visible so the
player understands that shelf's device state; another playable route remains
selectable from the arrow menu.

Remembering a player's preferred route per game is useful but is not part of
this slice. It requires a later decision about profile ownership, route identity
stability, unavailable-route fallback, and synchronization between browsers.

### Installation and preparation

Install is a preparation action, not proof that the game has only one route.
For an xCloud-capable game, the UI may therefore offer **Install on ...**,
**Play locally** when ready, and **Play in xCloud** together. Installation
preflight and mutation continue to follow ADR-0012 and ADR-0013.

## Persistence and compatibility

`NO_MIGRATION_NEEDED` for the initial UI foundation. It derives actions from
the existing game, cloud, browser-play, and installed-device facts and stores no
new preference. Migration 21 is already applied and remains unchanged. A later
remembered default or persisted emulator selection requires migration 22 or
later.

Existing callers that supply one primary card action remain compatible. The
card action contract grows to accept multiple typed actions and a contextual
route preference without changing server or device protocol payloads.

## Initial acceptance criteria

1. A browser-playable and xCloud-playable game exposes both routes.
2. Browser and Cloud shelves make their named route the main action.
3. An installed-device action can coexist with browser and cloud alternatives.
4. The arrow remains usable when the contextual primary action is disabled.
5. Card controls contain no generic visible **Open** action.
6. The control has separate accessible main-action and menu buttons, supports
   Escape/outside-click dismissal, and labels the menu for the game.

## Verification — 17 July 2026

The packaged production frontend ran from `server\bin\mga_server.exe`, not a
Vite development server. The live Play page showed **Play in browser** for an
Altered Beast browser-emulator card and **Play in xCloud** for an A Game About
Digging a Hole cloud card, with **Details** separate and no generic Open action.
The current library contains no item in both of those shelves, so focused
resolver tests cover cloud-context defaulting, simultaneous browser/cloud
routes, a disabled installed-device primary with playable alternatives, route
deduplication, and the no-route Details fallback.

Frontend unit tests (8 total) and the production build pass. The packaged app
reported no browser console warnings or errors. The known approximately 838 KB
Vite chunk warning remains. Schema stays 21; no persisted setting or protocol
payload changed.
