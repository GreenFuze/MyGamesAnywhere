
# Metadata & Media Providers (Plugins)

## LaunchBox export provider (MVP)
- Input: user selects LaunchBox export file (local).
- Plugin parses export and supports:
  - `search(title)` returning candidate games
  - `fetch(id)` returning structured metadata

## IGDB metadata provider (MVP)
- BYO API credentials; stored locally (keychain).
- Supports `search` and `fetch`.
- Rate limiting and caching are required (local cache in SQLite).

## SteamGridDB media provider (MVP)
- BYO API key; stored locally.
- Fetches artwork (cover, hero, logo, icon).
- Caches media in local filesystem; stores only cache keys in SQLite.

## Media cache rules
- Cache path is local-only, never synced.
- Eviction:
  - simple LRU or max size threshold (configurable)
