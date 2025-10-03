# Dependencies

All dependencies with versions and rationale for MyGamesAnywhere.

## Server Dependencies (Go)

### Core Framework

```go
// go.mod
module github.com/GreenFuze/MyGamesAnywhere/server

go 1.21

require (
    // Web framework
    github.com/gin-gonic/gin v1.9.1

    // Database
    github.com/lib/pq v1.10.9                    // PostgreSQL driver
    github.com/mattn/go-sqlite3 v1.14.18         // SQLite driver (for local server)

    // Database tooling
    github.com/golang-migrate/migrate/v4 v4.16.2 // Database migrations

    // Authentication
    github.com/golang-jwt/jwt/v5 v5.1.0          // JWT tokens
    golang.org/x/crypto v0.15.0                  // bcrypt password hashing

    // Configuration
    github.com/joho/godotenv v1.5.1              // Load .env files

    // Validation
    github.com/go-playground/validator/v10 v10.16.0

    // UUID
    github.com/google/uuid v1.4.0
)
```

### Why These?

**Gin v1.9.1**
- Most popular Go web framework (70k+ stars)
- Fast (uses httprouter)
- Excellent middleware ecosystem
- Good documentation
- **Alternative considered:** Echo (similar, slightly less popular)

**lib/pq v1.10.9**
- Standard PostgreSQL driver
- Pure Go implementation
- Well-maintained
- **Alternative considered:** pgx (more features, more complex)

**golang-migrate v4.16.2**
- Standard migration tool
- Supports up/down migrations
- CLI and library
- Works with multiple databases
- **Alternative considered:** goose (simpler, less features)

**golang-jwt/jwt v5.1.0**
- Official JWT library continuation
- Type-safe
- Supports all algorithms
- **Alternative considered:** jwt-go (unmaintained)

**bcrypt (in golang.org/x/crypto)**
- Standard for password hashing
- Slow by design (secure)
- Built into Go's extended packages
- **Alternative considered:** argon2 (newer, overkill for now)

---

## Client Dependencies (npm)

### Root Package

```json
// package.json (root)
{
  "name": "mygamesanywhere",
  "private": true,
  "workspaces": ["packages/*"],
  "devDependencies": {
    "@nx/workspace": "^17.1.0",
    "@nx/vite": "^17.1.0",
    "@nx/eslint-plugin": "^17.1.0",
    "nx": "^17.1.0",
    "typescript": "^5.2.2",
    "prettier": "^3.1.0",
    "eslint": "^8.54.0"
  }
}
```

### Core Package (`packages/core/`)

```json
{
  "name": "@mygamesanywhere/core",
  "version": "0.1.0",
  "type": "module",
  "dependencies": {
    // State management
    "zustand": "^4.4.6",

    // HTTP client
    "axios": "^1.6.2",

    // Validation
    "zod": "^3.22.4"
  },
  "devDependencies": {
    // Testing
    "vitest": "^1.0.0",
    "@vitest/ui": "^1.0.0",

    // TypeScript
    "typescript": "^5.2.2",

    // Build tool
    "vite": "^5.0.0"
  }
}
```

**Why These?**

**Zustand v4.4.6**
- Minimalistic state management (3kb)
- Simple API
- No boilerplate
- Excellent TypeScript support
- Middleware support (devtools, persist)
- **Decision:** Chosen over Redux for simplicity (can migrate if needed)

**Axios v1.6.2**
- De facto HTTP client
- Interceptors (for auth tokens)
- Request/response transformers
- Better error handling than fetch
- **Alternative considered:** fetch (built-in, less features)

**Zod v3.22.4**
- TypeScript-first validation
- Type inference
- Composable schemas
- Great error messages
- **Alternative considered:** Yup (more established, less TypeScript-focused)

**Vitest v1.0.0**
- Vite-native testing
- Faster than Jest
- Compatible with Jest API
- Better watch mode
- **Decision:** Chosen over Jest for speed and Vite integration

---

### UI Shared Package (`packages/ui-shared/`)

```json
{
  "name": "@mygamesanywhere/ui-shared",
  "version": "0.1.0",
  "dependencies": {
    "react": "^18.2.0",
    "react-dom": "^18.2.0"
  },
  "devDependencies": {
    "@types/react": "^18.2.0",
    "@types/react-dom": "^18.2.0",
    "typescript": "^5.2.2",
    "vitest": "^1.0.0",
    "@testing-library/react": "^14.1.2",
    "@testing-library/jest-dom": "^6.1.5"
  }
}
```

**React v18.2.0**
- Most popular UI library
- Huge ecosystem
- Concurrent features
- Great TypeScript support
- **Alternative considered:** Vue 3, Svelte (less ecosystem for Ionic)

---

### Desktop Package (`packages/desktop/`)

```json
{
  "name": "@mygamesanywhere/desktop",
  "version": "0.1.0",
  "dependencies": {
    // Framework
    "react": "^18.2.0",
    "react-dom": "^18.2.0",

    // Ionic
    "@ionic/react": "^7.5.5",
    "@ionic/react-router": "^7.5.5",

    // Capacitor
    "@capacitor/core": "^5.5.1",
    "@capacitor/app": "^5.0.6",

    // Routing
    "react-router-dom": "^6.20.0",

    // Forms
    "react-hook-form": "^7.48.2",
    "@hookform/resolvers": "^3.3.2",

    // Styling
    "tailwindcss": "^3.3.5",

    // Internal packages
    "@mygamesanywhere/core": "*",
    "@mygamesanywhere/ui-shared": "*"
  },
  "devDependencies": {
    // Capacitor Desktop
    "@capacitor-community/electron": "^5.0.1",

    // Capacitor CLI
    "@capacitor/cli": "^5.5.1",

    // Build tools
    "@vitejs/plugin-react": "^4.2.0",
    "vite": "^5.0.0",

    // TypeScript
    "typescript": "^5.2.2",
    "@types/react": "^18.2.0",
    "@types/react-dom": "^18.2.0",

    // Tailwind
    "autoprefixer": "^10.4.16",
    "postcss": "^8.4.32",

    // Testing
    "vitest": "^1.0.0",
    "@testing-library/react": "^14.1.2"
  }
}
```

**Why These?**

**@ionic/react v7.5.5**
- Cross-platform UI components
- Native look and feel
- Excellent mobile support
- Good documentation
- **Decision:** Core framework for cross-platform

**@capacitor/core v5.5.1**
- Modern Cordova alternative
- Better performance
- Native plugin API
- Active development
- **Decision:** Enables mobile + desktop from one codebase

**@capacitor-community/electron v5.0.1**
- Capacitor plugin for Electron
- Enables desktop builds
- Community maintained but stable
- **Decision:** Desktop support without separate codebase

**react-router-dom v6.20.0**
- Standard routing for React
- Type-safe routes
- Nested routing
- **Alternative considered:** TanStack Router (newer, less ecosystem)

**react-hook-form v7.48.2**
- Performant forms
- Less re-renders
- Great TypeScript support
- Integrates with Zod
- **Alternative considered:** Formik (older, more re-renders)

**tailwindcss v3.3.5**
- Utility-first CSS
- Highly customizable
- Excellent documentation
- Great with React
- Works alongside Ionic components
- **Decision:** Primary styling approach, use Ionic for complex components

---

## Development Dependencies

### Linting

```json
{
  "devDependencies": {
    "eslint": "^8.54.0",
    "@typescript-eslint/eslint-plugin": "^6.13.0",
    "@typescript-eslint/parser": "^6.13.0",
    "eslint-plugin-react": "^7.33.2",
    "eslint-plugin-react-hooks": "^4.6.0",
    "prettier": "^3.1.0",
    "eslint-config-prettier": "^9.1.0"
  }
}
```

**ESLint + TypeScript**
- Standard linting
- Type-aware rules
- Integrates with TypeScript

**Prettier v3.1.0**
- Code formatting
- Opinionated (less config)
- Works with ESLint

---

## Infrastructure Dependencies

### Docker

```yaml
# docker-compose.yml
services:
  postgres:
    image: postgres:15-alpine  # PostgreSQL 15 (lightweight)
```

**Why PostgreSQL 15?**
- Latest stable
- JSON improvements
- Better performance
- Alpine for smaller image size

---

## Version Strategy

### Server (Go)

- **Go version:** 1.21+ (latest stable)
- **Update policy:** Patch versions immediately, minor versions when stable
- **Locked versions:** Yes (via go.sum)

### Client (npm)

- **Node version:** 18 LTS (active LTS)
- **Update policy:**
  - Framework updates: test thoroughly first
  - Patch versions: auto-update safe
  - Major versions: evaluate carefully
- **Locked versions:** Yes (via package-lock.json)

---

## Installation Commands

### Server

```bash
cd server
go mod download
```

### Client

```bash
# From root
npm install

# Or with pnpm (faster)
pnpm install
```

---

## Updating Dependencies

### Check for Updates

```bash
# Server
cd server
go list -u -m all

# Client
npx npm-check-updates
```

### Update

```bash
# Server (specific package)
cd server
go get -u github.com/gin-gonic/gin

# Client (all packages)
npx npm-check-updates -u
npm install
```

---

## Known Issues / Compatibility Notes

### Capacitor + Electron

**Issue:** @capacitor-community/electron is community-maintained
**Mitigation:** Pin to v5.0.1, test thoroughly
**Fallback:** Can switch to pure Electron if needed

### Zustand + React 18

**Issue:** No issues, fully compatible
**Note:** Zustand 4.4+ required for React 18

### Vite + Ionic

**Issue:** No issues, well-supported
**Note:** Use @vitejs/plugin-react

---

## Future Dependencies (Phase 2+)

Dependencies to add later:

### Phase 2 (Plugins)

**Server:**
- `github.com/hashicorp/go-plugin` - Plugin system

**Client:**
- None (UI plugins are just React components)

### Phase 3 (Downloads)

**Client:**
- `@capacitor/filesystem` - File operations
- Download progress tracking (custom)

### Phase 4 (Emulation)

**Server:**
- OS-specific exec packages

---

## Summary

**Total Dependencies:**
- Server: ~15 packages
- Client (per package): ~20-30 packages
- Total size: ~200MB node_modules (acceptable)

**Why these versions?**
- Latest stable versions as of October 2024
- Proven in production
- Active maintenance
- Good documentation

**Update frequency:**
- Security patches: immediately
- Minor versions: monthly review
- Major versions: quarterly evaluation

See [PROJECT_STRUCTURE.md](./PROJECT_STRUCTURE.md) for how these are organized and [DEVELOPMENT_SETUP.md](./DEVELOPMENT_SETUP.md) for installation instructions.
