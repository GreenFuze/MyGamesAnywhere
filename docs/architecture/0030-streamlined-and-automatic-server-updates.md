# ADR-0030: Streamlined and automatic server updates

- **Status:** Accepted
- **Date:** 2026-07-23
- **Scope:** MGA Server update checks, update actions, and player notifications

## Player outcome

Updating MGA should normally take one explicit action. MGA also checks for a
new server release in the background and tells signed-in players when one is
available, without downloading or installing anything until a player chooses
to do so.

## Decision

1. When a newer release is available and no verified package has been
   downloaded, the Updates page offers:
   - **Download and apply** as the primary action; and
   - **Download only** as the secondary action.
2. **Download and apply** uses the existing update service's combined path:
   download the selected asset, verify its size and SHA-256, and only then
   launch the platform-specific updater.
3. Once a verified package exists, the actions become:
   - **Apply** as the primary action; and
   - **Redownload** as the secondary action.
   Redownload replaces the verified package only after the replacement has
   downloaded and passed verification. A failed replacement preserves the
   previous verified package.
4. MGA Server starts one process-wide automatic update checker. It makes its
   first check one minute after startup and checks hourly thereafter. Checks
   never overlap with manual checks or update downloads.
5. An automatic check only reads the release manifest. It never downloads or
   applies a release.
6. When an automatic check first discovers a newer version during the current
   server process, MGA publishes the existing server-global
   `update_available` event. Every connected profile may retain the same
   non-sensitive notification, and its action opens **Settings > Updates**.
   The event contains only server version and public release information.
7. The same version is announced at most once per server process. A restart may
   announce an update again so an outstanding update cannot become permanently
   silent. Check failures are logged and retried at the next scheduled check;
   they do not create recurring error notifications.

## Ownership and security

Server update state is server-global, matching ADR-0028's existing event
classification. It carries no profile identity, credentials, device paths, or
provider data. Profile authorization remains required for the Updates API and
SSE stream. Trusted-LAN HTTP support is unchanged.

## Persistence and compatibility

`NO_MIGRATION_NEEDED`: the schedule and per-process notification deduplication
are runtime state only. No SQLite table, persisted setting, JSON configuration,
update manifest, or public API shape changes. Existing verified downloads are
detected using their current path and SHA-256 metadata.

Rollback is safe: an older MGA version retains the existing manual two-step
update flow and ignores no new persisted state.

## Failure behavior

- Manifest, network, and asset-selection failures leave the current server
  running and are retried on the next hourly check.
- A combined update never applies an unverified or partial download.
- A failed redownload does not destroy a previously verified package.
- Update checks and downloads are serialized so automatic and player-started
  work cannot race.

## Acceptance criteria

- [x] A not-yet-downloaded update offers **Download and apply** and
      **Download only**.
- [x] A verified download offers **Apply** and **Redownload**.
- [x] Combined download/apply retains download progress and restart recovery.
- [x] Redownload safely replaces an existing verified package.
- [x] The background checker starts with MGA, checks after one minute and then
      hourly, and never downloads or applies automatically.
- [x] A newly discovered version creates one actionable notification per
      server process and invalidates cached update status in the web UI.
- [x] Automatic check failures are non-fatal and low-noise.
- [ ] Focused update, event-isolation, frontend unit, production build, full Go,
      migration guard, packaging, and real update checks pass.
