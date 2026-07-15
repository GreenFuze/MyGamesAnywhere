# MGA Client

MGA Client is MGA's standalone per-OS-user device agent and command-line tool.
It is intentionally isolated in this top-level module: it does not embed the web
interface and does not import server implementation packages.

The current v1 foundation implements per-user pairing, a DPAPI-protected
Ed25519 identity on Windows, an authenticated outbound WebSocket, heartbeat
presence, typed endpoint commands, diagnostics, single-instance enforcement,
`mga://pair` and signed `mga://start` handling, per-user installer registration,
bounded device inventory reporting, and transactional ZIP installation. Stopping
the client, refreshing inventory, installing ZIP-backed portable games, and
manifest-guarded uninstall are implemented. Game launch/stop, EXE/BIN and
storefront installation, emulator management, and client self-update remain
later command families. No unrestricted shell command will be added.

On Windows the installed background process is the windowless
`mga-client-agent.exe` notification-area application. Its tray menu provides
**Show logs** and **Exit**. The installer starts the per-user agent immediately,
registers it for startup at sign-in, and points `mga://` at it. The separate
`mga-client.exe` remains a console executable for intentional CLI use.

## Security boundary

- The endpoint identity is one physical host context + OS user + client
  installation. Another OS user gets a separate endpoint.
- The agent runs without permanent administrator or service privileges.
- HTTP/WS is supported for trusted LAN installations so MGA does not require a
  locally trusted certificate. HTTPS/WSS remains supported and is strongly
  recommended outside a trusted LAN; never expose an HTTP MGA Server directly
  to the internet because credentials and session tokens are not transport
  encrypted.
- The private key is protected with current-user DPAPI and never leaves the
  endpoint.
- The browser talks only to MGA Server. It does not expose or call a local client
  HTTP service.
- Commands are allow-listed, versioned, authorized by the server, and checked
  against the endpoint's advertised capabilities.

## Commands

Run from `client/` during development, or use the built executable:

```powershell
go test ./...
go run ./cmd/mga-client version
go run ./cmd/mga-client pair --server http://127.0.0.1:8900 --code <one-time-code>
go run ./cmd/mga-client agent
go run ./cmd/mga-client status
go run ./cmd/mga-client doctor
go run ./cmd/mga-client unpair
go run ./cmd/mga-client protocol "mga://pair?server=...&code=..."
go run ./cmd/mga-client protocol "mga://start?server=...&launch_id=...&token=..."
```

`pair` creates the local per-user identity. `agent` holds the outbound
connection. `unpair` fails while that agent instance is running, then removes
the versioned local configuration and protected endpoint key so the OS user can
pair again.

Build the Windows binary with:

```powershell
.\build.ps1
```

Build the per-user Inno Setup installer with:

```powershell
.\package-installer.ps1
```

The installer registers `mga://` and starts the agent at current-user login via
an HKCU startup entry;
it requests no elevation. Packaging fails fast when `ISCC.exe` is unavailable.
MGA Server serves a packaged installer from
`<app-dir>/downloads/mga-client-windows-amd64-installer.exe`, or from the
absolute `MGA_CLIENT_INSTALLER_PATH` environment override; otherwise the
Devices tab links to the latest published GitHub release artifact.

## Persistence and migration impact

The client currently persists `config.json` at schema version 1 and stores its
private key separately with Windows DPAPI. A clean installation has no previous
client state to migrate. Future changes to this JSON schema require a versioned
client migration; a newer unknown schema stops the client with an explicit
error. Installed ZIP games carry a separate `.mga-install.json` manifest at
schema version 1. Durable reconnect/idempotency storage remains a required
future migration before MGA automatically retries interrupted mutations; the
current client never replays them automatically.

`NO_MIGRATION_NEEDED` for the client `config.json`: archive support does not
change its schema or the DPAPI identity. Existing paired installations remain
compatible and simply do not advertise archive capabilities until updated.

Server migrations 11, 12, 14, and 15 add profile credentials/sessions, device
control-plane data, versioned inventory snapshots, and archive installation
state respectively. See
[ADR-0001](../docs/architecture/0001-mga-client-architecture.md) and the
[implemented v1 protocol foundation](../docs/architecture/mga-device-protocol-v1.md).
