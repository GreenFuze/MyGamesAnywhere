# ADR-0033: Steam family-shared library visibility

- **Status:** Accepted
- **Date:** 2026-07-24
- **Scope:** Steam game-source plugin discovery of Steam Families shared-library
  titles, the credential model required to read them, shared-title labeling and
  semantics, persistence of the shared marker, and scan degradation behavior
- **Jira:** MGA-64
- **Agent:** Claude Dev 1
- **Canonical record:** [MGA Confluence ADR-0033](https://greenfuzer.atlassian.net/wiki/spaces/MG/pages/4456449/ADR-0033+Steam+Family-Shared+Library+Visibility)
- **Depends on:** ADR-0001; relates to the Profile Isolation scope ledger

## Context

The Steam source lists a library only via `IPlayerService/GetOwnedGames`, which
returns games the account **owns**. Steam Families (the 2024 family-library
system) shares titles the borrowing account does **not** own, so those titles
never appear. A user who borrows a large shared library sees an incomplete
Steam library in MGA.

Reading a shared library requires Steam's `IFamilyGroupsService` endpoints
(`GetFamilyGroupForUser` then `GetSharedLibraryApps`). These require a Steam
account **access token** (`webapi_token`), **not** the Web API publisher key the
plugin uses today. The token is short-lived (~24h) and bound to a logged-in
Steam session. The plugin's current credentials (a manually entered Web API key
plus a SteamID resolved via OpenID) cannot read the shared library.

## Decision

1. **Credential model — user-supplied token, never a password.** MGA must not
   handle a Steam password or perform an interactive Steam credential/2FA login.
   The plugin gains an optional, secret `access_token` config field. The user
   obtains the token from their own logged-in Steam session
   (`store.steampowered.com/pointssummary/ajaxgetasyncconfig` → `webapi_token`)
   and pastes it, exactly like the existing API key.
2. **Discovery.** With an `access_token`, the plugin resolves the family group
   via `GetFamilyGroupForUser` and enumerates shared apps via
   `GetSharedLibraryApps`. Apps whose `owner_steamids` exclude the account's own
   SteamID are marked **shared** with an owner attribution.
3. **Labeling and semantics.** Shared titles are shown, labeled **"Shared"**,
   and are play-capable in principle, but availability depends on the lender. A
   shared title does **not** grant the borrower install-ownership or Save Domain
   authority; MGA treats shared titles as a visibility-and-play surface only.
4. **Graceful degradation — fail-fast but non-catastrophic.** Owned-game listing
   never depends on the shared-library token. No token → owned games only. Token
   present but rejected/expired → clear attributable error for the shared
   portion and a needs-token flag on the integration, **without** failing the
   owned-games scan. Token expiry is expected (~daily); re-pasting is refresh.
5. **Persistence (`NO_MIGRATION_NEEDED`).** The shared marker and owner
   attribution flow from the plugin through `ResolverMatch`/`core.Game`
   following the `IsGamePass`/`XcloudAvailable` precedent. Like those fields they
   persist inside the existing per-resolver-match `metadata_json` blob, so **no
   schema migration is required**. The change is backward- and forward-
   compatible: existing rows lack the key and decode to not-shared, and older
   binaries ignore the extra key. The marker only becomes set after a rescan.

## Security and privacy

- The access token is a per-profile secret stored like `api_key` (`x-secret`),
  never logged, never returned in API responses.
- Only the profile's own Steam integration token is used; no cross-profile
  reuse. Shared data is fetched under the profile that owns the token.
- Owner attribution exposes only the lender's Steam identity as Steam returns it
  for a family group the user already belongs to; no additional profile data is
  joined. Consistent with the profile isolation ledger.
- The short token lifetime bounds the blast radius of a leaked token.

## Acceptance criteria

- With a valid access token, family-shared titles appear labeled "Shared" with
  an owner attribution.
- Without a token, or with an expired/invalid token, owned games still list; the
  shared portion degrades with a clear, non-fatal signal and a needs-token flag.
- No Steam password is ever entered into or handled by MGA.
- Shared titles do not acquire install-ownership or Save Domain authority.
- The shared marker persists via the existing `metadata_json` blob with no
  schema migration (`NO_MIGRATION_NEEDED`); existing installs default to
  not-shared until rescanned.
- Plugin and server tests cover: owned-only (no token), owned+shared merge
  (valid token), expired-token degradation, and shared labeling with owner
  attribution.

## Follow-ups (tracked in Jira, not here)

- Enforcing "no install-ownership / no Save Domain authority for shared titles"
  across the install and save-sync controllers, if not fully covered by the
  visibility change, is a separate Jira item.
