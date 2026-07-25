# ADR-0034: Auto-provisioned zero-setup connections for new profiles

- **Status:** Accepted
- **Date:** 2026-07-25
- **Scope:** What connections a newly created profile starts with; eligibility
  rules for automatic provisioning
- **Jira:** MGA-65
- **Agent:** Claude Dev 1
- **Decided by:** Product owner (GreenFuze)
- **Canonical record:** [MGA Confluence ADR-0034](https://greenfuzer.atlassian.net/wiki/spaces/MG/pages/4685825/ADR-0034+Auto-Provisioned+Zero-Setup+Connections+for+New+Profiles)

## Context

MGA-65 was reported as "Orr's Google Drive 'Shared with me' connection doesn't
detect games." Investigation on TV2 (v0.2.12, Orr profile) showed the shared
Drive scan works correctly: it found **243 games** with correct titles,
platforms, and paths. All 243 were held in review with `no_metadata_matches`
and `no_resolved_title`, because MGA gates a filesystem game into review until
it has a metadata-resolved title.

The profile had **no metadata provider connection at all**. Five zero-setup
metadata plugins ship with MGA (`metadata-gog`, `metadata-hltb`,
`metadata-launchbox`, `metadata-mame-dat`, `metadata-steam`) — `launchbox` and
`mame-dat` are exactly the ROM/arcade matchers that library needed. They were
never added, and nothing prompted the player to add them.

So this was not a Drive bug and not a review-UX bug. The real gap: **a new
profile starts with no metadata capability, so filesystem games cannot be
identified until the player discovers and adds providers manually.**

## Decision

On profile creation, MGA automatically connects every plugin that is genuinely
**zero-setup**. A plugin qualifies only if all three hold:

1. **No required config** — no manifest config field is marked `required`.
2. **No external account sign-in** — the manifest does not provide
   `auth.oauth.callback`.
3. **Capability is enrichment or local-only storage** — every declared
   capability is in `{metadata, save_sync}`.

Eligibility is derived from plugin manifests at runtime, not a hardcoded list,
so a future zero-setup plugin is included automatically.

This currently provisions: `metadata-gog`, `metadata-hltb`,
`metadata-launchbox`, `metadata-mame-dat`, `metadata-steam`,
`save-sync-local-disk`.

Deliberately excluded, because each expresses player intent rather than a safe
default:

- **Game sources** (`game-source-*`) — a source declares *where a player's
  library lives*. Never implicit.
- **Cloud destinations** (`save-sync-google-drive`,
  `sync-settings-google-drive`) — these choose where data is written and need an
  account.
- **Anything needing credentials** (`metadata-igdb`, `metadata-rawg`,
  `retroachievements`, `game-source-smb`).

`game-source-epic` and `game-source-xbox` have no *required* config but are
still excluded by rules 2/3: they would appear connected while being unusable
until sign-in.

**Scope: new profiles only.** Existing profiles are not backfilled; provisioning
runs solely in the profile-creation path. Profiles that predate this decision
(including Orr's) need their providers added manually.

## Behavior and failure semantics

- Provisioning is **idempotent**: a plugin the profile already has is skipped,
  and an existing connection is never modified.
- Connections are written in the **new profile's own scope**, not the creating
  admin's, preserving profile isolation.
- Each connection is created with empty config `{}`, the same label the web
  interface would use, and `integration_type` = the plugin's first capability —
  indistinguishable from a hand-created connection, so it can be edited or
  deleted normally.
- The profile already exists by the time provisioning runs, so a provisioning
  failure is **logged and reported but does not fail profile creation**. A
  server without a plugin host simply provisions nothing.

## Persistence

`NO_MIGRATION_NEEDED`. No schema or persisted-format change: provisioning
inserts ordinary rows into the existing `integrations` table through the
existing repository. Existing installs and existing profiles are untouched;
rollback to an older binary leaves the created connections working, since they
are indistinguishable from manually created ones.

## Acceptance criteria

- A newly created profile starts with the six zero-setup connections above and
  can identify ROM/filesystem games on its first scan without manual setup.
- No game source, cloud-sync destination, or credential-requiring plugin is
  ever auto-connected.
- Eligibility is derived from manifests; a new zero-setup plugin needs no code
  change.
- Re-running provisioning creates nothing new and modifies no existing
  connection.
- Connections are owned by the new profile, never the creating admin.
- Profile creation still succeeds if provisioning fails or no plugin host is
  present.
