# Next Release Notes (Development)

These notes track upgrade-sensitive work after v0.2.0. They are renamed or
folded into the numbered release notes when the release version is selected.

## Changes

- Allow profile authentication, credential management, device pairing, device
  management, and MGA Client WS connections over HTTP on trusted LANs. HTTPS
  remains supported and recommended for untrusted networks.

## Upgrade and migration notes

- Add a versioned migration or an explicit `NO_MIGRATION_NEEDED` note for every
  persisted SQLite, JSON, or configuration change.
- `NO_MIGRATION_NEEDED`: this changes transport acceptance and documentation
  only. Existing SQLite data, client JSON, and server configuration remain
  compatible and unchanged.
