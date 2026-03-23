# MyGamesAnywhere — Web UI (Phase 1)

## Dev (API on localhost)

1. Start the Go server (e.g. from `server/bin` or `go run ./cmd/server` with `config.json`).
2. From this directory:

```bash
npm install
npm run dev
```

Vite serves on **http://127.0.0.1:5173** and proxies `/api` and `/health` to **`VITE_API_PROXY_TARGET`** (default `http://127.0.0.1:8900`).

## Production (same origin as API)

```bash
npm run build
```

Output: `server/frontend/dist/`. The Go server serves it when **`FRONTEND_DIST`** resolves to that folder relative to the process working directory (default `./frontend/dist`).

**From repo root (`server/`):** `build.ps1` runs `npm ci` + `npm run build` and copies **`dist` → `bin/frontend/dist`**. Then use **`run.ps1`** or **`start.ps1`** (or **`build_and_start.ps1`**) to launch **`bin/mga_server`** — no npm at runtime.

## Themes

Eleven presets live in `src/theme/presets.ts`. The active `themeId` is saved to **`POST /api/config/frontend`** and `localStorage`.

## OpenAPI

Types for games/health are hand-maintained in `src/api/client.ts`. Regenerating a full client from `openapi.yaml` is still optional (roadmap).
