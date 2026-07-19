# Profile isolation scope ledger

- **Status:** Authoritative for ADR-0028
- **Date:** 2026-07-19
- **Allowed scopes:** `server_global`, `profile_owned`, `device_os_user`,
  `opaque_capability`

Missing ownership is never interpreted as global. `profile_owned` repositories
derive the owner from `core.ProfileFromContext` and validate object IDs again at
the SQL boundary. `device_os_user` state is addressed by a per-OS-user endpoint
and, where applicable, a binding/server or granted profile. `opaque_capability`
records can be used only with an expiring, single-purpose, non-enumerable token.

## SQLite inventory

| Tables | Scope | Boundary |
|---|---|---|
| `schema_migrations`, `settings` | `server_global` | Schema history and server configuration. Profile UI settings use `profile_settings`, not `settings`. |
| `profiles`, `profile_settings`, `profile_credentials`, `auth_sessions`, `integrations` | `profile_owned` | Exact `profile_id`; credentials are Argon2id hashes and sessions store token hashes, never raw passwords, PINs, or cookies. Safe profile-picker fields are the only public projection. |
| `source_games`, `game_files`, `metadata_resolver_matches`, `source_game_media`, `scan_reports` | `profile_owned` | Explicit owner on the source/report or ownership derived through `source_games`; repository queries require the selected profile. |
| `achievement_sets`, `achievements`, `achievement_refresh_states` | `profile_owned` | Ownership derives through the exact source game and provider integration; repository methods validate selected-profile visibility. |
| `source_cache_entries`, `source_cache_entry_files`, `source_cache_jobs` | `profile_owned` | Explicit owner on entry/job and derived ownership on files. Cross-profile IDs return no object. Cache payload directories are owner-prefixed. |
| `canonical_games`, `game_titles`, `game_editions`, `game_title_external_ids` | `server_global` | Deduplicated canonical identity/metadata. A profile sees a canonical row only through one of its own source rows; the shared title itself is not an account secret. |
| `canonical_source_games_link`, `canonical_source_pins` | `profile_owned` | Ownership derives from the linked source row. Pins additionally carry profile ownership. |
| `canonical_game_cover_overrides`, `canonical_game_cover_override_clears`, `canonical_game_hover_overrides`, `canonical_game_background_overrides` | `server_global` | Canonical artwork curation is shared metadata, like canonical title data. Access is profile-authorized; it cannot reveal a game absent from that profile. A future per-profile artwork preference would require a new ADR/migration. |
| `canonical_game_favorites` | `profile_owned` | Composite key begins with `profile_id`. |
| `media_assets` | `server_global` | Content-addressed/deduplicated media metadata and cache state. Serving an asset requires authorized selected-profile access; bytes contain no connection credentials. |
| `device_endpoints`, `device_inventories`, `device_game_installations`, `device_installation_events`, `device_install_preferences`, `device_emulator_preferences`, `device_emulator_core_preferences`, `device_save_domain_links` | `device_os_user` | Endpoint is one device/OS-user client. Installation/save authority is further bound to profile and/or client binding as represented by each record. |
| `device_grants` | `device_os_user` | Explicit endpoint/profile access join; it permits device actions, never access to another profile's library, connections, settings, or saves. |
| `device_commands` | `device_os_user` | Exact endpoint plus initiating profile and grant. Payload/result access follows the same session/grant check. |
| `device_pairing_challenges` | `opaque_capability` | Short-lived hashed challenge; redemption produces the endpoint/profile grant and cannot enumerate other challenges. |

The temporary migration tables `device_installation_events_v20` and historical
pre-profile schemas exist only inside a transaction while an immutable old
migration executes; they are not runtime persistence surfaces.

## Persisted files, protected keys, and cache directories

| Object | Scope | Boundary |
|---|---|---|
| Server `config.json`, plugin manifests/binaries, update state and packages, logs | `server_global` | Process/runtime configuration. Secrets are redacted from logs and API projections. |
| Integration `config_json` in SQLite | `profile_owned` | Provider tokens belong to the owning connection. The server never derives identity from the browser computer, MGA Client, Windows/Xbox app, or another connection. API responses strip tokens, authorization codes, PKCE material, and client secrets. |
| `%APPDATA%/mga/profiles/<profile_id>/sync_key.enc` (Windows) or equivalent per-profile key file | `profile_owned` | Windows key is DPAPI-protected for the server OS user and separated by safe profile ID. The pre-v3 global key is ignored because its owner is ambiguous. |
| Settings Sync remote `latest.json` and history | `profile_owned` | Payload v3 declares exactly one owner; connection and encryption key are selected-profile scoped. Whole-server backup is not Settings Sync. |
| Server save-sync cache | `profile_owned` | Path begins with safe owner profile ID, then integration/game/source/runtime/slot. Upload workers retain the initiating profile. |
| Server media cache | `server_global` | Content-addressed shared media; authorized game visibility is checked before URLs are obtained. |
| Source-cache directories | `profile_owned` | Entry/job owner is persisted and repository validated; cache keys/paths are not accepted as authority. |
| Client `config.json`, `endpoint_key.dpapi`, `identities/`, `mga-client.log` | `device_os_user` | Stored under the current OS user's application directory. Config schema v4 supports independent, unique server bindings. Private endpoint identity is DPAPI protected. |
| Client `installation-ownership.json`, `.mga` install manifests/failure markers, emulator cache | `device_os_user` | Installation owner is a client binding; use/adopt/release is explicit and path reservation prevents cross-server collisions. Legacy unlabelled ownership fails closed when multiple bindings exist. |
| Client `save-domain-authority.json`, `save-domains/` | `device_os_user` | One current writer binding; transfer enters reconciliation when local state could diverge. Save contents/paths never enter general logs or notifications. |

Install-root defaults are profile/device preferences at the server and resolve on
the device. Emulator defaults/cores are per endpoint; emulator catalog/plugin
health is server-global. Update availability is server-global because one server
binary serves every profile. Plugin process health is likewise global, while a
provider authentication failure is profile-owned.

## Browser persistence inventory

| Keys | Scope | Compatibility action |
|---|---|---|
| `mga.selectedProfileId` | `profile_owned` routing selector, not authentication | Changing it clears React Query before the new UI renders; server cookies remain authority. Cross-tab storage events clear/reconnect too. |
| `mga.profile.v2.<profile>.*` library/play preferences, recent games, scan job, review queue, duplicate selection, return scroll, browser-source/executable preference | `profile_owned` | Version-1 ambiguous keys are deleted, never adopted by the selected profile. Server preferences can repopulate authoritative values. |
| `mga.browserPlaySession.*` | `profile_owned` | Payload embeds `ownerProfileId`; all runtime players reject it unless it equals the current selected profile. Ambiguous old sessions are deleted. |
| `mga.notification-history.v*:<profile>` | `profile_owned` | Existing notification store already keys by selected profile; profile switch reconnects SSE before rendering. |
| `mga.clientEndpoint.<profile>` | `profile_owned` choice over `device_os_user` endpoints | Each profile remembers its own selected endpoint. |
| `mga.themeId`, `mga.dateTimeFormat` | `server_global` browser appearance/accessibility | Deliberately browser-global: these describe this browser's rendering, contain no player data, and remain stable at profile switch. |
| Update-apply recovery marker | `server_global` | Tracks the one server process restart/update, not a player's operation. |

React Query data is in-memory only. The equivalent profile cache boundary is a
synchronous `queryClient.clear()` after cancellation and before selected-profile
state changes; no profile-owned query result survives the boundary.

## In-memory registries and event families

| Object/family | Scope | Boundary |
|---|---|---|
| Scan, achievement-refresh, integration-refresh, save migration/prefetch/upload jobs | `profile_owned` | Immutable owner in the job/worker record. Foreign start sees only opaque busy; foreign status/cancel behaves not found. |
| OAuth state/drafts | `profile_owned` plus opaque state | Profile, plugin, optional connection, creation, expiry, callback claim/completion. Ten-minute TTL; one callback; process restart invalidates all state. |
| SSE: scan, achievements, connection CRUD/status, OAuth, settings/save sync, source cache, profile operation errors | `profile_owned` | Central bus derives/requires `profile_id`; ownerless publication is dropped. SSE itself requires selected-profile authorization. |
| SSE: update download/apply lifecycle, `update_available`, `plugin_process_exited` | `server_global` | Only the explicitly enumerated central allowlist is global. An unknown `update_*` name is not accepted, and callers cannot create a new global family by setting a flag. |
| Pairing, client-launch, archive/content/save-domain transfer tokens | `opaque_capability` | Short-lived, purpose-bound, one-time/non-enumerable tokens; possession grants only the named operation. |
| Active install/save path coordinator | `device_os_user` | Binding-tagged path lease prevents simultaneous local writers. |

## Persistence compatibility and migration conclusion

`NO_SQLITE_MIGRATION_NEEDED` for ADR-0028. The inventory shows every affected
SQLite row already has an explicit owner or an unambiguous owner join, and the
implementation changes authorization, worker context, and projections rather
than schema. Migrations 1–28 remain byte-for-byte immutable; migration 29 is not
created.

Settings Sync advances its remote JSON contract from v2 whole-document behavior
to a v3 single-owner payload. New pushes write v3. V3 pulls reject mixed owners.
V2 pulls accept only an exact selected owner or a single unambiguous owner and
otherwise stop with an actionable error. Older MGA versions cannot safely
interpret v3 and must not be used to push over a v3 profile backup; rollback
requires restoring the previous server binary/database backup and retains any
old v2 remote history.

Browser player-owned state moves to versioned v2 keys and ambiguous old state is
discarded. Rolling back may recreate legacy browser caches, which are disposable;
server state is unchanged. The old global Settings Sync protected-key file is
left untouched for rollback but ignored by the hardened version because assigning
it to a player would guess ownership. Each profile must store or supply its own
passphrase once. Client persistence formats are unchanged by ADR-0028.
