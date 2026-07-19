# ADR-0027: Profile-bound OAuth drafts and request-scoped provider access

- **Status:** Accepted
- **Date:** 2026-07-19
- **Scope:** Profile credential setup, OAuth connection creation, Xbox/Google Drive provider identity, and provider access

## Context

MGA profiles own their connections, but the Google Drive plugin historically
cached one token process-wide and also read and wrote `tokens.json`. A connection
created or used for one profile could therefore borrow the account most recently
used by another profile. New OAuth connections could also pass validation using
remembered plugin state without showing the provider's account chooser.

The initiating browser is not an ownership boundary. A player may connect their
profile from their own LAN computer or, deliberately, from another computer.
The selected MGA profile and the connection row are the ownership boundaries.

## Decision

1. First-time password or PIN setup is available on the profile sign-in screen
   from any trusted-LAN browser. It remains optional, atomic, and create-only.
   Existing-credential recovery remains a local server administration operation.
2. Every new OAuth connection starts a fresh interactive authorization. MGA does
   not accept a plugin-wide cached credential as proof that a new profile
   connection is authorized.
3. OAuth state is single-purpose and bound to the selected profile and plugin,
   and to the saved connection when reauthorizing one. OAuth result secrets are
   persisted only into that profile's connection row.
4. Provider access is request-scoped. A plugin operation receives the exact
   connection config selected by the server. Google Drive no longer reads,
   writes, or falls back to process-wide user tokens.
5. Draft Google Drive browsing uses the exact profile-bound OAuth draft. Saved
   connection browsing uses the exact profile-owned connection row. Browsing by
   plugin identity alone is not an authorization source.
6. Microsoft and Google authorization URLs request an account chooser so a user
   operating from any computer can deliberately select the account intended for
   the active MGA profile.
7. Epic's legacy server-local login is replaced by an explicit per-connection
   authorization-code field. Plugin startup never opens a provider login and
   Epic scans use only tokens stored in the active profile's connection.
8. Xbox connection creation is fail-closed: a callback-capable plugin cannot
   create a new connection unless it starts interactive authorization, and the
   completed profile-bound draft must contain a refreshable Microsoft account
   plus its Xbox user ID. MGA never reads the Windows Xbox app, Microsoft Store,
   MGA Client, or initiating computer as Xbox connection identity.
9. OAuth tokens remain in the server-side connection row and are redacted from
   integration API responses. A safe provider identity (provider, immutable
   subject, and optional display name) may be returned so the UI can identify
   which account owns the connection.

## Persistence and compatibility

`NO_MIGRATION_NEEDED`: integrations already have profile ownership and store
OAuth tokens in their existing server-side `config_json`. The optional
`provider_identity` object is backward-compatible JSON metadata populated on
authorization or token validation. This change removes an unsafe fallback
rather than requiring a new persisted table or column. Legacy Google Drive
and storefront `tokens.json` files may remain on disk but are ignored. The Epic
authorization-code form writes the existing connection config shape. Existing
connection rows remain readable. A connection previously authorized to the
wrong provider account must be explicitly reconnected; MGA cannot safely infer
the intended account or rewrite it automatically.

## Security and failure behavior

- HTTP on a trusted LAN remains supported; neither localhost nor HTTPS is
  assumed.
- Missing draft state, wrong-profile state, plugin mismatch, and missing saved
  connection identity fail closed.
- OAuth tokens are not returned to frontend JavaScript during draft validation,
  browsing, connection listing, creation, update, or duplicate responses.
- Cancelling authorization leaves the new connection uncreated.
- Concurrent profiles cannot select each other's provider tokens through plugin
  process state.

## Acceptance criteria

- An unconfigured profile can set its optional password or PIN at sign-in from a
  remote LAN browser.
- Adding Xbox or Google Drive always displays interactive provider sign-in with
  account selection.
- Xbox cannot be created from plugin process state, a legacy plugin token file,
  the MGA Client, the Windows Xbox app, or an incomplete OAuth callback.
- Google Drive check, browse, scan, materialization, sync, and save-sync calls
  fail without tokens from the exact connection config or profile-bound draft.
- Steam and Epic scans fail without identity/tokens from the exact profile
  connection; neither plugin loads its legacy process token file.
- An OAuth callback can update only the profile/connection registered in its
  state.
- Focused server, frontend, Xbox, and Google Drive tests cover the isolation
  rules.
