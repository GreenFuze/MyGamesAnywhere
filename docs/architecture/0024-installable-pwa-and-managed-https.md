# ADR-0024: Installable PWA and optional managed HTTPS

- **Status:** Accepted direction; implementation deferred
- **Date:** 2026-07-18
- **Scope:** MGA web installation, TLS/certificate lifecycle, canonical server
  origins, trusted-LAN HTTP compatibility, and MGA Client URL migration
- **Depends on:** ADR-0001, ADR-0019, ADR-0020, ADR-0023

## Context

MGA is primarily a web application, but players may want it to appear and
launch like an installed application. MGA also intentionally supports HTTP on
a trusted LAN so a server does not present a self-signed certificate warning on
every device. HTTP nevertheless exposes profile credentials, session cookies,
library activity, and device-control traffic to anyone able to observe or alter
that LAN traffic.

An installable Progressive Web App and most service-worker functionality require
HTTPS except on `localhost` or loopback. A remote LAN origin such as
`http://tv2:8900` does not receive that exception. HTTPS and PWA therefore form
one product sequence rather than two unrelated switches.

Let's Encrypt is an ACME certificate authority. MGA does **not** need a Let's
Encrypt API key. An ACME client creates and protects an ACME account key. The
validation method determines any additional credential:

- HTTP-01 needs a publicly reachable port 80 and no DNS API credential;
- DNS-01 needs control of a real public DNS name and normally a narrowly scoped
  API token from the player's **DNS provider**, not from Let's Encrypt;
- publicly trusted certificates cannot cover `localhost`, `.local`/internal
  names, or reserved/private LAN IP addresses;
- Let's Encrypt now supports public IP certificates, but they are short-lived
  and require HTTP-01 or TLS-ALPN-01. They do not make `192.168.x.x`, `10.x.x.x`,
  or another reserved LAN address eligible.

Useful primary references:

- [Let's Encrypt challenge types](https://letsencrypt.org/docs/challenge-types/)
- [Let's Encrypt integration guide](https://letsencrypt.org/docs/integration-guide/)
- [Let's Encrypt localhost certificates](https://letsencrypt.org/docs/certificates-for-localhost/)
- [Let's Encrypt IP certificate availability](https://letsencrypt.org/2026/01/15/6day-and-ip-general-availability)
- [W3C Web Application Manifest](https://www.w3.org/TR/appmanifest/)
- [W3C Secure Contexts](https://www.w3.org/TR/secure-contexts/)

## Decision

### Priority

Do not interrupt the current device-installation roadmap merely to add a
manifest or certificate wizard. Finish richer OS/storefront product identity,
the locally confirmed **Use existing installation** grant, and explicit
save-sync ownership transfer first.

After that, implement this decision in the following order:

1. canonical-origin and client-binding migration contract;
2. custom certificate and external reverse-proxy support;
3. managed ACME DNS-01 with a provider adapter;
4. installable PWA shell;
5. carefully bounded network-first service-worker caching.

PWA metadata is small, but shipping it before remote-LAN HTTPS would advertise
an installation feature that browsers cannot offer on normal MGA LAN URLs.

### Trusted-LAN HTTP remains supported

HTTP remains a supported explicit mode and the default for existing installs.
MGA must describe it plainly as **Trusted LAN (HTTP)** rather than claiming it
is secure. No release may silently require TLS, invalidate an existing LAN
server, or force a self-signed warning.

When a valid HTTPS configuration is active, ordinary browser HTTP requests
redirect to the canonical HTTPS origin. MGA does not emit HSTS in the first TLS
release because an expired/misconfigured certificate must be recoverable through
the explicitly enabled HTTP path.

### TLS modes

MGA global settings will expose four mutually exclusive modes:

1. **Trusted LAN (HTTP)** — current behavior.
2. **Use my certificate** — player supplies a PFX or PEM chain/private key.
3. **Automatic certificate (ACME)** — player supplies a real hostname and a
   supported validation/provider configuration.
4. **HTTPS handled by another server** — an existing reverse proxy terminates
   TLS; MGA trusts forwarded origin headers only from an explicit proxy
   allowlist.

The player-facing settings must not use “API key” as a generic field. ACME
DNS-01 asks for a provider, hostname, optional ACME contact email, and the
provider-specific narrowly scoped token/fields. HTTP-01 is an advanced mode and
must warn that public inbound port 80 is required; it is not the LAN default.

### Recommended LAN ACME configuration

The recommended automatic path is a real player-owned domain with DNS-01 and
split-horizon DNS:

```text
mga.home.example.com --public DNS ownership proof--> Let's Encrypt
mga.home.example.com --LAN DNS answer-------------> private MGA address
```

The DNS-provider token is stored through MGA's protected secret store, is never
returned by the API after saving, and is never written to logs, SQLite JSON,
notifications, diagnostics, or release reports. Provider adapters request only
the permission needed to create and remove `_acme-challenge` TXT records.

Certificate Transparency makes issued hostnames public. Settings must warn
players not to put a person's name, address, or other private information in the
hostname.

### Certificate lifecycle and failure behavior

MGA owns issuance, renewal, activation, and status visibility when automatic
ACME is selected:

- use the ACME staging directory during setup validation before production;
- schedule renewal with jitter and well before expiry;
- validate the hostname, chain, private-key match, validity window, and server
  binding before activating a certificate;
- atomically swap a verified certificate without restarting active downloads or
  device commands when the server runtime permits it;
- retain the last valid certificate if renewal fails;
- show Healthy, Renewing, Action needed, and Expired states with exact expiry,
  last attempt, provider, and an actionable retry/settings link;
- notify administrators without including provider secrets;
- if no valid certificate remains, stop HTTPS redirection and expose the
  explicitly configured HTTP recovery origin with a prominent warning.

HTTPS setup fails closed: MGA never redirects to a hostname whose certificate
or listener has not passed a local end-to-end probe.

### Canonical origin and MGA Client migration

Changing `http://tv2:8900` to `https://mga.home.example.com` changes the web
origin and currently looks like a different MGA Client binding. A blind HTTP
redirect would therefore break `mga://start`, WebSocket/WSS connection matching,
and potentially OAuth callback configuration.

TLS activation must be a two-phase origin migration:

1. the existing authenticated server advertises the new canonical HTTPS origin
   and proves it uses the same server installation identity;
2. MGA Client validates the public certificate, records the new URL as an alias
   of the existing stable binding ID, reconnects through WSS, and acknowledges;
3. the web UI reports clients that still need migration;
4. browser HTTP redirect becomes active only after the HTTPS probe succeeds.

Reinstalling a server or merely reusing its hostname does not prove identity.
ADR-0023 ownership remains attached to the client-local binding ID. HTTPS
migration changes an endpoint alias, never installation ownership.

HTTP and HTTPS are separate browser origins. Server-side profiles, games, and
settings remain intact, but cookies and browser-local notification history do
not automatically cross origins. MGA must sign the player in again and explain
that old browser-only history remains at the former HTTP origin; it must not
silently copy browser storage across origins.

### PWA scope

The first PWA release is an installable online shell, not an offline MGA server.
It includes:

- a web app manifest with stable app ID, `/` scope and start URL, standalone
  display, theme/background colors, and dedicated maskable 192/512 icons;
- player-facing **Install MGA** guidance when the browser and secure origin
  support installation;
- normal deep links into Play, Library, game details, and Settings;
- an explicit offline/server-unavailable screen.

The first release must not cache authenticated API responses, cover art without
bounds, ROMs, installers, save files, device commands, OAuth callbacks, update
assets, or profile credentials. If a service worker is added, it is network-first
and limited to versioned static shell assets. A newly deployed MGA version must
not be hidden behind a stale service worker; update and rollback tests are
release gates.

PWA installation complements the MGA Client. It does not replace device
inventory, installation, launch, emulator, save, or elevation capabilities.

## Persistence and migration

`NO_MIGRATION_NEEDED` for recording this decision: no runtime configuration or
persisted schema changes are made by this ADR alone.

Implementation will require versioned persistence for global TLS mode,
canonical origin, provider ID, non-secret ACME metadata, certificate status, and
protected-secret references. Migration 26 is locked; an SQLite implementation
must add migration 27 or later. Certificate/private keys, ACME account keys, and
DNS tokens belong in the protected keystore, not ordinary SQLite/config JSON.

Client config will require a versioned additive migration that records canonical
and legacy endpoint aliases against the same stable binding ID. Unknown schema
versions continue to fail closed.

## Acceptance criteria for the later implementation

- Existing HTTP installations continue working until an administrator opts in.
- Settings explain exactly which credential is required and never request a
  fictional Let's Encrypt API key.
- Private IP/internal-name configurations are rejected with useful guidance.
- Custom certificate, reverse proxy, and DNS-01 modes fail closed and expose
  actionable health/renewal status.
- Secrets never round-trip through normal APIs or logs.
- Redirect begins only after a valid HTTPS end-to-end probe.
- Existing clients retain their endpoint identity and installation ownership
  across the HTTP-to-HTTPS origin change.
- OAuth callbacks and browser sign-in are tested on the canonical origin.
- PWA installation works on a remote LAN device over HTTPS and is not falsely
  offered on an insecure non-loopback origin.
- Service-worker update, rollback, offline, authentication, and cache-boundary
  tests prove that sensitive/dynamic content is never cached.

