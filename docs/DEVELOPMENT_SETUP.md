# Development Setup

Step-by-step guide to set up MyGamesAnywhere development environment.

## Prerequisites

### Required Software

**Go 1.21+**
```bash
# Check version
go version

# Should output: go version go1.21.x or higher
```

Download from: https://go.dev/dl/

---

**Node.js 18 LTS**
```bash
# Check version
node --version

# Should output: v18.x.x or higher
```

Download from: https://nodejs.org/

---

**Docker Desktop**
```bash
# Check version
docker --version
docker-compose --version
```

Download from: https://www.docker.com/products/docker-desktop/

---

**Git**
```bash
# Check version
git --version
```

Download from: https://git-scm.com/downloads

---

### Optional but Recommended

**PostgreSQL Client (psql)** - For database inspection
```bash
# Check if installed
psql --version
```

**Make** - For running Makefile commands
```bash
# Check if installed
make --version
```

**VS Code** - Recommended IDE with extensions:
- Go (by Go Team)
- ESLint
- Prettier
- Nx Console

---

## Initial Setup

### 1. Clone Repository

```bash
git clone https://github.com/GreenFuze/MyGamesAnywhere.git
cd MyGamesAnywhere
```

---

### 2. Server Setup

#### Install Go Dependencies

```bash
cd server
go mod download
```

#### Set Up Environment Variables

```bash
# Copy example env file
cp .env.example .env

# Edit .env with your settings
# (Use your editor of choice)
```

**`.env` file:**
```bash
# Database
DATABASE_URL=postgresql://postgres:postgres@localhost:5432/mygamesanywhere?sslmode=disable

# JWT
JWT_SECRET=your-super-secret-jwt-key-change-in-production
JWT_EXPIRES_IN=3600
REFRESH_TOKEN_EXPIRES_IN=604800

# Server
PORT=8080
GIN_MODE=debug

# CORS
ALLOWED_ORIGINS=http://localhost:8100,http://localhost:3000
```

**⚠️ IMPORTANT:** Change `JWT_SECRET` to a secure random string!

#### Start PostgreSQL (Docker)

```bash
# From server directory
docker-compose up -d

# Check it's running
docker-compose ps

# Should show postgres container as "Up"
```

**Verify PostgreSQL connection:**
```bash
# Connect to database
psql postgresql://postgres:postgres@localhost:5432/mygamesanywhere

# Run a test query
SELECT version();

# Exit
\q
```

#### Run Migrations

```bash
# Install migrate tool (one-time)
go install -tags 'postgres' github.com/golang-migrate/migrate/v4/cmd/migrate@latest

# Run migrations
migrate -path migrations -database "postgresql://postgres:postgres@localhost:5432/mygamesanywhere?sslmode=disable" up

# Verify tables exist
psql postgresql://postgres:postgres@localhost:5432/mygamesanywhere -c "\dt"
# Should show: users, libraries, schema_migrations
```

#### Install sqlc (Code Generation)

```bash
# Install sqlc (one-time)
go install github.com/sqlc-dev/sqlc/cmd/sqlc@latest

# Generate database code
cd server
sqlc generate

# Verify generated files
ls internal/models/
# Should show: models.go, users.sql.go, libraries.sql.go
```

#### Run Server

```bash
# From server directory
go run cmd/server/main.go

# Should output:
# [GIN-debug] Listening and serving HTTP on :8080
```

**Test server:**
```bash
curl http://localhost:8080/health
# Should return: {"status":"ok"}
```

---

### 3. Client Setup

#### Install Dependencies

```bash
# From repository root
npm install

# This installs all packages in the monorepo
# Takes 2-5 minutes depending on internet speed
```

#### Verify Nx Installation

```bash
# Check Nx is installed
npx nx --version

# Should output: 17.x.x
```

#### Set Up Environment Variables

```bash
# From packages/desktop directory
cp .env.example .env

# Edit .env
```

**`packages/desktop/.env` file:**
```bash
VITE_API_URL=http://localhost:8080/api/v1
```

#### Run Desktop Client

```bash
# From repository root
npx nx serve desktop

# Or use npm script
npm run dev:desktop

# Should output:
# VITE v5.x.x ready in xxx ms
# ➜ Local: http://localhost:8100/
```

**Open browser:** http://localhost:8100

You should see the app running!

---

## Verify Full Stack

### Test Complete Flow

1. **Server running:** http://localhost:8080
2. **Client running:** http://localhost:8100
3. **PostgreSQL running:** `docker-compose ps` shows "Up"

### Create Test User

**Via API:**
```bash
curl -X POST http://localhost:8080/api/v1/auth/register \
  -H "Content-Type: application/json" \
  -d '{
    "email": "test@example.com",
    "username": "testuser",
    "password": "TestPassword123!"
  }'

# Should return user object with token
```

**Via UI:**
1. Open http://localhost:8100
2. Click "Register"
3. Fill form and submit
4. Should see dashboard with default library

---

## Common Commands

### Server

```bash
cd server

# Run server
go run cmd/server/main.go

# Run tests
go test ./...

# Run tests with coverage
go test -cover ./...

# Lint code
golangci-lint run

# Run migrations up
migrate -path migrations -database $DATABASE_URL up

# Run migrations down
migrate -path migrations -database $DATABASE_URL down

# Generate sqlc code
sqlc generate

# Build binary
go build -o bin/server cmd/server/main.go

# Run binary
./bin/server
```

### Client

```bash
# From repository root

# Serve desktop app (dev mode)
npx nx serve desktop

# Build desktop app
npx nx build desktop

# Run tests
npx nx test desktop

# Run all tests
npx nx run-many --target=test --all

# Lint
npx nx lint desktop

# Lint all
npx nx run-many --target=lint --all

# Generate component
npx nx generate @nx/react:component MyComponent --project=ui-shared
```

### Docker

```bash
cd server

# Start PostgreSQL
docker-compose up -d

# Stop PostgreSQL
docker-compose down

# View logs
docker-compose logs -f postgres

# Reset database (⚠️ DESTRUCTIVE)
docker-compose down -v
docker-compose up -d
# Then re-run migrations
```

---

## IDE Setup

### VS Code

**Recommended Extensions:**
```json
{
  "recommendations": [
    "golang.go",
    "dbaeumer.vscode-eslint",
    "esbenp.prettier-vscode",
    "nrwl.angular-console",
    "bradlc.vscode-tailwindcss"
  ]
}
```

**Settings (`.vscode/settings.json`):**
```json
{
  "go.useLanguageServer": true,
  "go.lintTool": "golangci-lint",
  "editor.formatOnSave": true,
  "editor.defaultFormatter": "esbenp.prettier-vscode",
  "[go]": {
    "editor.defaultFormatter": "golang.go"
  },
  "typescript.tsdk": "node_modules/typescript/lib"
}
```

---

## Troubleshooting

### Server Issues

**Problem: `go mod download` fails**
```bash
# Solution: Check Go version and proxy
go version  # Must be 1.21+
go env GOPROXY  # Should not be "off"
```

**Problem: Database connection fails**
```bash
# Solution: Check PostgreSQL is running
docker-compose ps

# Restart if needed
docker-compose restart postgres

# Check logs
docker-compose logs postgres
```

**Problem: Migrations fail**
```bash
# Solution: Reset database
docker-compose down -v
docker-compose up -d
# Wait 5 seconds for postgres to start
migrate -path migrations -database $DATABASE_URL up
```

**Problem: Port 8080 already in use**
```bash
# Solution: Find and kill process
# Windows
netstat -ano | findstr :8080
taskkill /PID <PID> /F

# Mac/Linux
lsof -ti:8080 | xargs kill
```

---

### Client Issues

**Problem: `npm install` fails**
```bash
# Solution: Clear cache and retry
rm -rf node_modules package-lock.json
npm cache clean --force
npm install
```

**Problem: Nx commands fail**
```bash
# Solution: Install Nx globally
npm install -g nx

# Or always use npx
npx nx <command>
```

**Problem: Port 8100 already in use**
```bash
# Solution: Use different port
npx nx serve desktop --port=3000
```

**Problem: "Cannot find module '@mygamesanywhere/core'"**
```bash
# Solution: Rebuild workspace
npx nx reset
npm install
npx nx build core
npx nx serve desktop
```

---

### Docker Issues

**Problem: Docker daemon not running**
```bash
# Solution: Start Docker Desktop
# Then verify
docker ps
```

**Problem: PostgreSQL container won't start**
```bash
# Solution: Check port 5432 is free
# Windows
netstat -ano | findstr :5432

# Mac/Linux
lsof -ti:5432

# If occupied, stop that process or change port in docker-compose.yml
```

**Problem: Permission denied (Linux)**
```bash
# Solution: Add user to docker group
sudo usermod -aG docker $USER
# Log out and back in
```

---

## Database Management

### Connecting to Database

**Via psql:**
```bash
psql postgresql://postgres:postgres@localhost:5432/mygamesanywhere
```

**Via TablePlus/DBeaver/pgAdmin:**
- Host: localhost
- Port: 5432
- Database: mygamesanywhere
- User: postgres
- Password: postgres

### Useful Queries

**List tables:**
```sql
\dt
```

**Show users:**
```sql
SELECT id, email, username, created_at FROM users;
```

**Show libraries:**
```sql
SELECT l.id, l.name, u.username as owner, l.is_default
FROM libraries l
JOIN users u ON l.user_id = u.id;
```

**Clear all data (⚠️ DESTRUCTIVE):**
```sql
TRUNCATE users CASCADE;
-- This deletes all users and their libraries (due to CASCADE)
```

---

## Running Tests

### Server Tests

```bash
cd server

# Run all tests
go test ./...

# Run specific package
go test ./internal/services

# Run with coverage
go test -cover ./...

# Generate coverage report
go test -coverprofile=coverage.out ./...
go tool cover -html=coverage.out
```

### Client Tests

```bash
# Run all tests
npx nx run-many --target=test --all

# Run specific package
npx nx test core

# Run with UI
npx nx test core --ui

# Run with coverage
npx nx test core --coverage
```

---

## Building for Production

### Server

```bash
cd server

# Build binary
go build -o bin/server cmd/server/main.go

# Build for different OS
GOOS=linux GOARCH=amd64 go build -o bin/server-linux cmd/server/main.go
GOOS=windows GOARCH=amd64 go build -o bin/server.exe cmd/server/main.go
GOOS=darwin GOARCH=amd64 go build -o bin/server-mac cmd/server/main.go
```

### Client

```bash
# Build desktop app
npx nx build desktop

# Output in: packages/desktop/dist/

# Build Electron app
npx nx build desktop --configuration=production
# Then use Electron builder (Phase 2)
```

---

## Next Steps

Once development environment is working:

1. ✅ **Read `CODING_STANDARDS.md`** - Understand code conventions
2. ✅ **Read `PHASE1_DETAILED.md`** - See what to build
3. ✅ **Pick a task** - Start with Week 1 tasks
4. ✅ **Create feature branch** - `git checkout -b feature/auth-api`
5. ✅ **Write code** - Follow standards
6. ✅ **Write tests** - Test as you go
7. ✅ **Submit PR** - Get code reviewed

---

## Getting Help

- **Documentation:** Check `docs/` folder first
- **API Issues:** See `docs/API.md`
- **Database Issues:** See `docs/DATABASE_SCHEMA.md`
- **Architecture Questions:** See `docs/ARCHITECTURE.md`

---

## Summary

**Setup Time:** ~30 minutes (first time)

**What You Need:**
- Go 1.21+
- Node.js 18+
- Docker Desktop
- Git

**To Start Developing:**
```bash
# Terminal 1: Start PostgreSQL
cd server && docker-compose up

# Terminal 2: Start server
cd server && go run cmd/server/main.go

# Terminal 3: Start client
npm run dev:desktop

# Open browser: http://localhost:8100
```

**You're ready to code!** 🚀
