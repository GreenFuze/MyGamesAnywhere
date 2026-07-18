# ADR-0019: Profile-owned accounts and recoverable client pairing

- **Status:** Accepted
- **Date:** 2026-07-18
- **Scope:** Profile selection, trusted-LAN credential setup, storefront OAuth, MGA Client pairing UX

## Context

MGA is intentionally usable over HTTP on a trusted LAN. Profiles, storefront
accounts, and device/OS-user client identities are separate scopes. Current UX
breaks those boundaries in several ways: the profile picker cannot add a player,
initial credentials are restricted to the server host, the Xbox plugin can fall
back to process-wide cached tokens, and a client paired to another server exits
without a player-visible recovery path.

## Decision

1. The profile picker offers **Add player**. Creating a player from the picker is
   administrator-mediated: MGA signs into an existing administrator, creates a
   player profile through the normal authorized API, then returns to the new
   player's sign-in/setup flow. Administrator creation remains in Settings.
2. Initial password or PIN setup is allowed from remote trusted-LAN browsers.
   Initialization remains one-time, profile-scoped, hashed by the server, and
   rejects an already configured credential. Login rate limiting and strict,
   HTTP-only, same-site session cookies remain unchanged.
3. Storefront authorization is owned by the profile integration row. A new Xbox
   connection always starts an explicit Microsoft account chooser. Xbox scanning
   and achievements use only tokens supplied in that integration's request;
   process-global or `tokens.json` fallback is forbidden.
4. MGA Client pairing remains one server per device/OS-user client identity. If
   a browser asks a client paired to another server to start, the client shows a
   local explanation instead of silently closing and offers a deliberate
   **Unbind** action. The tray also exposes **Unbind from server**. Unbinding is
   locally confirmed, stops the running instance when applicable, and clears
   only that OS user's client configuration and private key. It does not delete
   the server's historical device record.
5. Settings navigation wraps or switches to a compact control instead of
   exposing native horizontal scrollbars.

## Persistence and compatibility

`NO_MIGRATION_NEEDED`: no SQLite schema or persisted JSON shape changes. Remote
credential initialization writes the existing credential record; Xbox tokens
continue to use the existing encrypted per-integration configuration; client
unbinding deletes the existing per-user pairing config and protected private
key only after explicit local confirmation. Existing pairings and integrations
remain readable. Legacy Xbox `tokens.json` may remain on disk but is ignored.

## Security and failure behavior

- Trusted-LAN HTTP remains supported; HTTPS and localhost are not assumed.
- A picker-created player requires a signed-in administrator and cannot create
  another administrator.
- Credential initialization cannot replace an existing credential.
- OAuth cancellation leaves the new Xbox integration unconnected and never
  borrows another profile's account.
- Missing or invalid per-integration Xbox tokens fail closed with
  `AUTH_REQUIRED`.
- Client server mismatch is explained locally; no automatic unbind or re-pair
  occurs.

## Acceptance criteria

- The profile selection screen can add a player after administrator sign-in.
- A remote LAN browser can initialize a four-character password or four-digit
  PIN for an unconfigured profile.
- Two profile integrations cannot reuse one another's Xbox tokens through
  plugin process state or disk fallback, and creating Xbox always prompts for
  an account.
- A mismatched client launch shows the paired and requested server and offers a
  locally confirmed unbind path; the tray offers the same recovery action.
- Settings navigation has no native scrollbar at the reported desktop width.
- Focused server, frontend, Xbox plugin, and client tests pass.
