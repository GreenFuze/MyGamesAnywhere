# ADR-0033: Steam family-shared library visibility

- **Status:** Accepted (credential model revised 2026-07-25)
- **Date:** 2026-07-24; revised 2026-07-25
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
**account access token**, **not** the Web API publisher key the plugin used
before. Such access tokens are short-lived, so a durable solution needs a
long-lived credential MGA can renew from. The plugin's original credentials (a
manually entered Web API key plus a SteamID resolved via OpenID) cannot read the
shared library at all.

## Decision

1. **Credential model — app-approved QR sign-in, never a password.** MGA must not
   handle a Steam password or second factor. The player starts a sign-in in MGA
   and approves it in the **Steam mobile app**
   (`IAuthenticationService/BeginAuthSessionViaQR` → `PollAuthSessionStatus`),
   so password and Steam Guard stay on their own device. Steam returns a
   **long-lived refresh token**, which MGA stores on the profile's connection
   (`refresh_token`, `x-secret`, alongside `api_key`) and exchanges for
   short-lived access tokens on demand via `GenerateAccessTokenForApp`. The
   refresh token lasts months and renews without player action.

   *Superseded approach (v0.2.12):* the first implementation used a manually
   pasted `webapi_token` from
   `store.steampowered.com/pointssummary/ajaxgetasyncconfig`. Steam expires that
   token roughly daily, which made re-pasting a recurring chore, so it was
   rejected as the ongoing model and removed. Only the refresh token is honoured
   now; a previously pasted `access_token` value is ignored.
2. **Discovery.** With a minted access token, the plugin resolves the family group
   via `GetFamilyGroupForUser` and enumerates shared apps via
   `GetSharedLibraryApps`. Apps whose `owner_steamids` exclude the account's own
   SteamID are marked **shared** with an owner attribution.
3. **Labeling and semantics.** Shared titles are shown, labeled **"Shared"**,
   and are play-capable in principle, but availability depends on the lender. A
   shared title does **not** grant the borrower install-ownership or Save Domain
   authority; MGA treats shared titles as a visibility-and-play surface only.
4. **Graceful degradation — fail-fast but non-catastrophic.** Owned-game listing
   never depends on the shared-library credential. Not signed in → owned games
   only. Refresh token rejected (revoked or expired) → clear attributable error
   for the shared portion only, **without** failing the owned-games scan; the
   player signs in again to recover. Minting an access token happens per scan,
   so a rejected credential is detected before any family API call.
5. **Persistence (`NO_MIGRATION_NEEDED`).** The shared marker and owner
   attribution flow from the plugin through `ResolverMatch`/`core.Game`
   following the `IsGamePass`/`XcloudAvailable` precedent. Like those fields they
   persist inside the existing per-resolver-match `metadata_json` blob, so **no
   schema migration is required**. The change is backward- and forward-
   compatible: existing rows lack the key and decode to not-shared, and older
   binaries ignore the extra key. The marker only becomes set after a rescan.

## Security and privacy

- The refresh token is a per-profile secret stored like `api_key` (`x-secret`),
  never logged, never returned in API responses. Minted access tokens are held
  only in memory for the duration of a scan.
- Only the profile's own Steam credential is used; no cross-profile reuse. The
  sign-in endpoints load the profile's own connection first and reject a
  connection owned by another profile. Shared data is fetched under the profile
  that owns the credential.
- Owner attribution exposes only the lender's Steam identity as Steam returns it
  for a family group the user already belongs to; no additional profile data is
  joined. Consistent with the profile isolation ledger.
- The refresh token is exchanged for a short-lived access token per scan and is
  never sent to the Steam Families endpoints, so the long-lived credential is not
  exposed to those calls.
- Approval happens in the Steam mobile app, so MGA never receives, stores, or
  transmits a Steam password or Steam Guard code.

## Acceptance criteria

- After an app-approved sign-in, family-shared titles appear labeled "Shared"
  with an owner attribution, and keep working for months without player action.
- Not signed in, or with a rejected refresh token, owned games still list; the
  shared portion degrades with a clear, non-fatal signal.
- No Steam password or Steam Guard code is ever entered into or handled by MGA.
- The refresh token is never sent to the Steam Families endpoints; only minted
  short-lived access tokens are.
- Shared titles do not acquire install-ownership or Save Domain authority.
- The shared marker persists via the existing `metadata_json` blob with no
  schema migration (`NO_MIGRATION_NEEDED`); existing installs default to
  not-shared until rescanned.
- Plugin and server tests cover: owned-only (not signed in), owned+shared merge,
  rejected-refresh-token degradation, QR begin/poll pending/approved/expired
  states, credential persistence, cross-profile rejection, and shared labeling
  with owner attribution.

## Follow-ups (tracked in Jira, not here)

- Enforcing "no install-ownership / no Save Domain authority for shared titles"
  across the install and save-sync controllers, if not fully covered by the
  visibility change, is a separate Jira item.
