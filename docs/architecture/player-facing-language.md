# Player-Facing Language and Information Architecture

- **Status:** Product language policy
- **Date:** 2026-07-14

## Goal

MGA's normal interface is for people who want to find, install, organize, and
play games. It should not require them to understand the database, plugin
protocol, resolver, cache implementation, or canonical identity model.

Technical detail is valuable for troubleshooting and library repair, so MGA
keeps it behind Advanced details instead of deleting it.

## Language rules

1. Lead with what the player can do or what happened.
2. Use the game's name, connection name, and device name whenever possible.
3. Explain the next step, not the internal subsystem that failed.
4. Use one familiar term consistently; do not alternate between source record,
   item, canonical game, and resolver entity in the same workflow.
5. Put provider IDs, paths, plugin capabilities, raw errors, and protocol facts
   under **Technical details** or **Advanced**.
6. Destructive actions state exactly what will be removed and whether saves are
   affected.
7. Color never carries status alone. Pair green, amber, gray, purple, and red
   with a short label and reason.
8. Empty states explain whether there is nothing to do, nothing was found, or a
   connection could not be checked.

## Interface density and progressive disclosure

The interface should not try to teach the complete MGA architecture on every
screen. Each surface has three information levels:

1. **Always visible:** name, useful state, primary action, and the few facts
   needed for the next decision.
2. **On demand:** secondary actions, explanations, paths, counts, and recent
   activity in an expandable area, menu, or focused detail view.
3. **Advanced:** provider evidence, raw identifiers, plugin capabilities,
   protocol versions, command IDs, and original errors.

Moving text into a tooltip is not the only solution. Repeated or unnecessary
text should be removed. Tooltips are for concise clarification, not hidden
manual pages.

### Buttons and actions

- Visible button labels use a short verb or verb phrase: **Play**, **Install**,
  **Rescan**, **Connect**, **Edit**, **Retry**, **Stop**.
- A button must not contain its description, current status, and result history.
  Put those beside the action or in details.
- One card or panel has one visually dominant primary action. Less common
  actions move to a compact secondary row or an overflow menu.
- Use icon-only buttons only for familiar actions such as close, expand,
  notifications, or overflow. Every icon-only button needs an accessible name
  and a tooltip on hover and keyboard focus.
- Keep destructive actions visually separate. Their confirmation explains the
  effect; the normal button does not need a sentence-length warning.
- Accessible names may be more descriptive than the visible label. For example,
  the visible label can be **Stop** while the accessible name is **Stop MGA
  Client on TC-PC**.

### Labels, help text, and tooltips

- Labels name the value; they do not explain the implementation.
- Do not place a paragraph under every field. Add help text only when the user
  could make a harmful or confusing choice without it.
- Prefer one short introduction for a section over repeated explanations on
  every card.
- Use a tooltip for an unfamiliar icon, a short definition, or a status reason.
- Never place essential instructions, errors, permissions, or destructive
  consequences only in a tooltip. Tooltips are unavailable or awkward on many
  touch and controller-driven screens.
- Longer explanations belong in an expandable **Learn more** or **Technical
  details** area and should preserve the user's current context.

### Cards and lists

A normal game, connection, or device card should show only:

- recognizable name and artwork/icon;
- one primary state;
- two or three decision-relevant badges at most;
- one primary action;
- an expand control or overflow menu when more is available.

Expanded cards may show history, configuration summaries, permissions, file
locations, and diagnostics. Lists should start collapsed when opening every
card would create a wall of controls.

Do not repeat the same fact in the section header, badge, paragraph, and button.
For example, a green **Connected** state does not also need “Configuration
validated and the integration is ready” on every healthy connection card.

### Status and progress

- Use a short visible state such as **Ready**, **Scanning**, **Installing 42%**,
  **Offline**, or **Needs update**.
- Show the reason inline only when it requires attention. Healthy-state details
  can live in a tooltip or expanded panel.
- Progress replaces static explanatory text while work is active and exposes a
  focused details view for stages and logs.
- Recent history belongs in the notification center or activity details, not as
  permanent prose beside every action.

### Responsive and controller-friendly behavior

MGA is a game interface and may be used on a television, handheld, touch screen,
or controller-driven browser. Hover cannot be required. Every tooltip-backed
fact must also be available through focus, tap, expansion, or the detail page.
Primary actions and status remain readable without precision pointing.

### Density modes

Different workflows need different density rather than one globally verbose
layout:

- **Covers** prioritizes artwork and immediate play state.
- **Details** exposes more badges and one-line facts.
- **Table** supports dense comparison, sorting, and bulk management.
- **Advanced details** exposes provenance and diagnostics regardless of the
  selected library view.

The application should remember view and density preferences per profile once a
versioned preference model is introduced.

## Preferred product vocabulary

| Internal or current wording | Normal interface | Advanced detail or notes |
|---|---|---|
| Canonical game | Game | Use “combined game page” only when explaining combine/separate actions. |
| Source game / source record | Copy or version; “Found in Steam” | “Source record” may appear in technical details. |
| Library source | Game connection or game location | Connection types include storefront, cloud drive, and network folder. |
| Integration | Connection | “Integration” is acceptable in developer/plugin documentation. |
| Metadata provider | Game info service | Provider name can remain visible. |
| Resolver / resolver match | Match suggestion; “MGA thinks this is…” | Scores and provider evidence belong in Advanced. |
| Unresolved / undetected game | Game MGA couldn't identify | The navigation destination should be Library Review. |
| Duplicate | Possible copy or version | Do not imply deletion is the only resolution. |
| Merge canonical games | Combine under one game | Preserve the concrete copies/versions. |
| Split source record | Show as a separate game | Explain what will remain attached. |
| Materialize | Prepare or download for play | The storage page may call it “prepared game files.” |
| Source cache | Prepared game files | Show size, device/server location, and safe clear behavior. |
| Media cache | Artwork and videos | “Cache” may appear in Advanced. |
| Install artifact/cache | Installation files | Distinguish it from the installed game. |
| Endpoint | Device | Display a per-user endpoint as `TC-PC (cs)` or equivalent. |
| Profile access | Who can use this device | Then show Play, Manage, or Owner permissions with descriptions. |
| Capability | Supported feature | Raw capability identifiers are plugin diagnostics. |
| `source.games.list` | Finds games | Advanced plugin page only. |
| `source.file.materialize` | Prepares games for play | Advanced plugin page only. |
| `plugin.check_config` | Check connection | Advanced plugin page only. |
| Authoritative reconciliation | Library scan | Explain added, removed, and unchanged results. |
| `not_found` | No longer found | Keep history without exposing status codes. |
| Save Sync transport/provider | Save sync service | Google Drive and Local Disk are service choices. |
| Save Domain | Compatible saves | Usually expressed as “These versions share saves.” |
| Play Route | Play option | Examples: Play in browser, Play with RetroArch, Launch with Steam. |
| Execution target | Play on | Examples: This browser or TC-PC (cs). |
| Configuration schema | Setup options | Schema information is plugin diagnostics. |

## Rewrite examples from the current interface

### Unidentified content

Current:

> Review unresolved source records inline, apply metadata matches, archive
> not-a-game items, and reopen archived decisions.

Player-facing:

> Review games MGA couldn't identify. Match them to the right game, mark
> add-ons, or hide files that aren't games.

Current:

> 0 resolver matches · No metadata matches · No resolved title

Player-facing:

> MGA couldn't find a confident match.

Advanced details can still show provider counts and raw title evidence.

### Duplicate and version review

Current:

> Groups same-title source records inside the same canonical game.

Player-facing:

> Find copies or versions that may belong under the same game.

The available decisions should be understandable actions:

- Same version;
- Different version;
- Different game;
- DLC or add-on;
- Not a game.

### Storage

Current:

> Reusable materialized source files for remote integrations such as Google
> Drive.

Player-facing:

> Game files MGA downloaded temporarily so they can start faster or play in
> your browser.

### Scanning

Current:

> Uses the same source scan and removal reconciliation as Rescan all.

Player-facing:

> MGA checks your connected libraries for added or removed games. You can run
> the same check now with Rescan all.

### Devices

Current:

> Profile access

Player-facing:

> Who can use this device

Permission descriptions should say what a person can do, not merely display an
authorization enum.

### Connection cards

Current healthy card:

> Connected  
> Configuration validated and the integration is ready.  
> Check Connection · Scan Library · Refresh Achievements · Edit · Delete

Player-facing collapsed card:

> **Steam** · 31 games · **Connected**  
> **Rescan** · More

The expanded card can contain connection checking, achievement refresh, setup,
removal, last scan, and technical details. These actions do not all need equal
weight on every card.

### Client status

A top-bar control should visibly say **Client ready**, **Connect client**, or
**Client needs update**. The full sentence explaining what clicking it opens is
an accessible name or tooltip, not the visible button label.

## Navigation recommendations

The current Settings surface mixes configuration, diagnostics, and daily
library work. The target organization is:

| Destination | Player purpose |
|---|---|
| Connections | Add storefronts, folders, game-info services, and sync services. |
| Devices | See where MGA Client is connected and manage who can use each device. |
| Profiles | Manage players, sign-in credentials, roles, and preferences. |
| Scanning | Choose automatic scan timing and review recent scans. |
| Storage | Manage artwork/videos, prepared game files, installation files, and save backups. |
| Appearance | Choose theme, date/time, card density, and library display defaults. |
| Updates | Update MGA and see client compatibility. |
| Advanced | Inspect plugins, capabilities, provider evidence, and protocol diagnostics. |

Move duplicate/version decisions and unidentified content to **Library Review**.
These are collection-management tasks, not application configuration. Settings
can show a link such as “79 games need review.”

## Library controls

Use separate controls for:

- **View:** Covers, Details, or Table;
- **Group:** Platform, Connection, Play option, Device, Achievements, Year, or
  None;
- **Sort:** Title, Recently added, Recently played, Release date, and other
  stable orderings;
- **Filter:** Installed, Browser play, Cloud play, Achievements, Favorites,
  Connection, Platform, and Device.

Do not call a chronological grouping a separate “Timeline view” unless its
interaction is genuinely different from grouping by year.

When grouping by connection, the same game can appear in several groups. The UI
should say this with a short hint instead of deduplicating away a valid source.

## Card and badge behavior

Cover cards are for recognition first. Default badges should answer “Can I play
this now, and how?” without covering the artwork.

Recommended priority:

1. playable now / action required;
2. installed or browser/cloud play;
3. update or save-sync warning;
4. achievements and connection identity when space permits.

Details and Table views can expose the full badge set. Badges use text labels or
accessible tooltips; an unfamiliar logo alone is insufficient.

## Progress and errors

Prefer a three-layer message:

1. **Outcome:** “Couldn't install Sonic Mania.”
2. **Reason:** “TC-PC needs 4.2 GB more free space.”
3. **Action:** “Free space and try again.”

Technical details may add the command ID, plugin, path, server error, or client
log entry. They must not replace the useful message.

Background work should be visible in the top bar notification history and on the
relevant page. A player should be able to answer what is running, on which
device, how far it has progressed, and whether attention is required.

## Persistence and migration

`NO_MIGRATION_NEEDED` for this documentation change: it changes no SQLite data,
persisted JSON/configuration, browser storage, or sync payload.

Implementing saved view, group, sort, badge, or route preferences will require a
versioned persisted preference shape and migration/default behavior for existing
profiles.
