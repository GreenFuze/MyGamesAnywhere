
# Acceptance & Verification

This section defines “done” in testable terms.

## A. App boot & persistence
- App boots to Library on Windows.
- Creating/editing a game persists after restart (SQLite).

## B. Plugin host
- Built-in plugins load via same mechanism as user plugins.
- User can install plugin zip; it appears and can be enabled.
- Disabling plugin prevents calls.
- Plugin errors surface with code + context and are logged.
- Circuit breaker disables a failing capability after N consecutive failures.

## C. Drive sync
- Connecting Drive creates `.mygamesanywhere/` folder and JSON files.
- Library edits are pushed to Drive and pulled on another device profile.
- Record-level LWW merges apply correctly.

## D. Drive installers scan & match
- User adds 2 Drive folders; scan stays within those trees.
- Scan finds setup files by extension.
- Each found file shows match candidates with confidence + explanation.
- User override persists and survives rescan.

## E. Steam
- Steam scan imports at least one installed game (on a Steam machine).
- Launch uses `steam://` protocol and starts the game.

## F. Metadata/media
- LaunchBox export import enables search/fetch.
- IGDB and SteamGridDB fetch works when keys are set.
- Media cached locally; not synced.

## Suggested automated tests
- Unit: schema validation, LWW merge, filename normalization, circuit breaker
- Integration: plugin install/enable/dispatch; SQLite migrations; Drive adapter (mock)
- E2E (Windows): basic launch attempt, UI flows

## Suggested commands (placeholder)
- `pnpm test`
- `pnpm lint`
- `pnpm typecheck`
