# Next Release Notes (Development)

These notes track upgrade-sensitive work after v0.1.2. They are renamed or
folded into the numbered release notes when the release version is selected.

## MGA Client foundation

- Adds optional per-profile password/PIN credentials and HttpOnly sessions.
  Profiles without credentials remain passwordless; protected profiles verify
  their credential at profile selection before entering MGA.
- Gives the first profile with the administrator role the temporary password
  `changeme`, regardless of its display name. It must be replaced during the
  first profile sign-in before MGA opens.
- Adds Show/Hide controls to password, PIN, recovery, and encryption-secret
  fields.
- Adds OS-backed credential recovery through the server executable's
  `--reset-profile-credential` command. The web sign-in screen provides the
  exact profile ID and install-mode commands but cannot perform an
  unauthenticated reset itself.
- Adds explicit per-profile endpoint grants, single-use pairing, authenticated
  client presence, command audit records, and the Settings Devices tab.
- Keeps password/PIN setup, changes, and disabling exclusively under Settings →
  Profiles. The Devices tab contains endpoint and client concerns only; profiles
  without credentials receive a link back to their profile settings.
- Collapses device cards by default, labels endpoint metadata refresh explicitly,
  and explains that profile permissions map MGA web profiles to one device / OS
  user endpoint rather than granting Windows account access.
- Adds the separately versioned, per-user MGA Client under `client/`.
- Adds an always-visible top-bar MGA Client control. A short-lived signed
  `mga://start` challenge identifies the responding device / OS-user endpoint,
  live server presence drives the color, and Manage users can confirm a typed
  `endpoint.stop` action. The installer also starts the per-user agent at login.
- The Devices download action serves a packaged installer from the server app
  directory (or `MGA_CLIENT_INSTALLER_PATH`) and falls back to the published
  release artifact when no local package is present.

## Upgrade and migration notes

- Database migration 11 creates `profile_credentials` and `auth_sessions`.
- Database migration 12 creates `device_endpoints`, `device_grants`,
  `device_pairing_challenges`, and `device_commands` plus their indexes.
- Both migrations are additive. Existing profile, library, integration,
  settings, and sync rows are unchanged.
- The bootstrap credential is created only for the first administrator profile
  when that profile has no existing credential. Existing credentials are never
  replaced. The persisted `admin_player` role value is retained for compatibility.
- The new MGA Client starts at JSON config schema version 1. There is no legacy
  client state to migrate; a future schema change must add a versioned client
  migration.
- `NO_MIGRATION_NEEDED` for installer discovery: it uses an immutable app
  artifact path or process environment override and does not alter persisted
  server configuration.
- `NO_MIGRATION_NEEDED` for profile-level sign-in: moving credential verification
  to profile selection and changing the displayed role label do not alter stored
  credentials or role values.
- `NO_MIGRATION_NEEDED` for credential recovery and visibility controls: recovery
  reuses the existing credential/session tables, and visibility is transient UI
  state only.
- `NO_MIGRATION_NEEDED` for the Profiles/Devices UI separation: it moves existing
  controls without changing stored profile credentials or device grants.
- `NO_MIGRATION_NEEDED` for collapsible device cards and permission help text:
  expansion is transient UI state and existing grants remain unchanged.
- `NO_MIGRATION_NEEDED` for top-bar client control: launch challenges are
  process-local and short-lived, `endpoint.stop` uses existing command records,
  and the additive browser endpoint-association key safely defaults to absent.
  Client config schema 1 and all existing SQLite rows remain compatible.
