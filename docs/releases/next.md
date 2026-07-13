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
- Adds explicit per-profile endpoint grants, single-use pairing, authenticated
  client presence, command audit records, and the Settings Devices tab.
- Adds the separately versioned, per-user MGA Client under `client/`.
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
