# Next Release Notes (Development)

These notes track upgrade-sensitive work after v0.2.1. They are renamed or
folded into the numbered release notes when the release version is selected.

## Changes

- Simplify the trusted-LAN profile credential policy: passwords accept any four
  or more characters, and PINs accept four or more digits with no maximum.

## Upgrade and migration notes

- Add a versioned migration or an explicit `NO_MIGRATION_NEEDED` note for every
  persisted SQLite, JSON, or configuration change.
- `NO_MIGRATION_NEEDED`: only validation of newly initialized or changed
  credentials is relaxed. Existing Argon2 hashes, sessions, SQLite data, client
  JSON, and server configuration remain compatible and unchanged.
