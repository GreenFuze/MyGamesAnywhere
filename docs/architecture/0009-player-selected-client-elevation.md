# ADR-0009: Player-selected MGA Client elevation

- **Status:** Accepted
- **Date:** 2026-07-16

## Decision

MGA Client stays a per-user, non-service process. It does not start with
Windows. When its endpoint is disconnected, a Manage-authorized player may
explicitly select either **Run MGA Client** or **Run MGA Client as
administrator** in MGA.

An elevated choice launches the same signed, per-user MGA Client executable
through Windows `runas`; Windows owns the UAC consent. The elevated client uses
the existing endpoint identity and reconnects as the same device/user pair. It
continues in that mode until the player selects tray **Exit**, signs out, or
restarts Windows. MGA never requests elevation remotely after a client is
already connected and never installs a service, scheduled task, or elevated
auto-start entry.

The server records the *current reported execution mode* (`standard` or
`elevated`) on the endpoint and exposes it in device UI. It is display/runtime
state, not an authorization grant. Manage authorization, verified installer
family/signature checks, fixed typed commands, and local destructive
confirmations are unchanged. Elevation does not suppress the MGA cleanup or
uninstall confirmation.

The short-lived `mga://start` launch challenge carries the requested mode and
the client signs that mode when redeeming the challenge. An unelevated protocol
handler re-launches itself with `runas` before redeeming an elevated challenge;
therefore a UAC cancellation leaves the challenge unacknowledged and no agent
runs. Existing callers that omit a mode mean `standard`.

The launch URL must match the paired server origin. Host comparison is exact
for every non-loopback server. For a server that is local on both sides,
`localhost`, an IPv4 loopback literal, and an IPv6 loopback literal are treated
as the same host only when scheme, effective port, and path also match. This
allows a browser opened through `127.0.0.1` to wake a client originally paired
through `localhost` without accepting a LAN/DNS host substitution.
The paired origin may use plain HTTP and may identify another LAN computer;
neither client launch nor elevation assumes that MGA Server is local. HTTP is
an explicitly accepted trusted-LAN deployment tradeoff and provides no
transport confidentiality or integrity on an untrusted network.

The server prepares the short-lived launch challenge while the disconnected
client menu is open. The visible standard/elevated actions are direct
`mga://start` links, so Windows receives the custom-protocol activation from
the player's actual click. Creating a challenge asynchronously and navigating
to the custom protocol only after that request returns is forbidden because
browsers may discard the user-activation context and silently block the local
handler. Unanswered attempts expose an explicit retry with a new challenge.

## Persistence and compatibility

Migration 19 adds `device_endpoints.execution_mode TEXT NOT NULL DEFAULT
'standard'`. Existing paired endpoints remain valid and display as standard
until their next connection. Client pairing identity and config remain
unchanged. The installer removes the old per-user `Run` auto-start value during
upgrade; this is an installer-owned registry cleanup, not a client config or
database migration.

`NO_MIGRATION_NEEDED` for loopback-origin canonicalization: it changes only
runtime comparison of the short-lived launch URL. Stored pairing identity,
server URL, endpoint key, client config schema, and database rows are unchanged.

## Failure behavior

- UAC declined/cancelled: no launch acknowledgement; UI remains disconnected.
- Elevation unavailable/non-Windows: fail closed; no standard fallback.
- An already-running endpoint remains the single per-user client instance; MGA
  does not create a second device or silently change its privilege mode.
- A disconnected client is started only by a player selecting one of the two
  launch actions.
