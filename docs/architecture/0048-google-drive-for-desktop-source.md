# ADR-0048: Google Drive for Desktop as its own source type

Status: Accepted
Date: 2026-09-05
Jira: MGA-120
Canonical decision: Confluence page `ADR-0048 — Google Drive for Desktop as its own source type` (23461889)

This local mirror records the contract and the reasoning next to the code. Confluence remains authoritative.

## Context

A Drive folder synced by Drive for Desktop was already readable. `game-source-local` (MGA-113) walks any absolute path, and a browse of `G:/My Drive/Games` through the running server returned its contents on the first try, including folders with Hebrew names on a sibling account.

The gap was never capability. It was that a user had to already know "Local Folder" was the answer, then type the path — and afterwards the connection was indistinguishable from any other folder.

Measured on the owner's machine: `GoogleDriveFS` running with **three** accounts mounted, on `G:`, `H:` and `I:`. The Drive API connection reaches exactly one of them and costs `drive.DriveScope` — full read and write across the entire Drive. A mounted folder reaches all three and costs no OAuth scope at all.

## Decision

`game-source-google-drive-desktop` is its own plugin id, executing the same `local.exe` as `game-source-local`. This follows the pattern the `drive` directory already uses to serve three ids from one binary.

The alternative — a wizard preset that filled in a path and created a local-folder connection — was rejected by the owner. It is less code, but the resulting connection would present as a plain folder forever after, which is the thing the ticket exists to avoid.

### The binary stopped asserting its own identity

`plugin.init` now carries `plugin_id` from the host. A binary backing several manifests could not previously learn which one started it, and no amount of inference from the request would have been honest. A host that sends nothing leaves the plugin answering as the local source, so nothing older breaks.

The entire behavioural difference between the two ids is which folders they offer as a starting point. Everything else — walking, containment, deletion refusals, config validation — is one implementation.

### Detection reads the volume label

Windows records the account there: a drive mounted for `someone@example.com` is labelled `someone@example.com - Google Drive`.

**Windows caps a volume label at 32 characters**, so the accounts most worth finding arrive truncated. This machine reports `green.fuzer@gmail.com - Googl...`. Matching the full suffix would therefore miss precisely the common case, so the check accepts any leading run of "Google Drive" while still requiring the ` - ` separator and an address in front of it. A first version required four characters of tail and failed against the real `... - Go...`; the prefix match was already doing the work, and the length floor was only excluding valid input.

The alternative was Drive FS's own configuration under `%LOCALAPPDATA%`, which knows the accounts but not reliably which letter each was given, and changes shape between client versions. The volume label is also what the user already sees in Explorer, so it is the answer they will recognise.

macOS reads `~/Library/CloudStorage/GoogleDrive-<address>`, plus the older `/Volumes/GoogleDrive` which cannot name an account. **Linux finds nothing, and that is correct rather than broken:** Google ships no Drive client for it.

The offered folder is `My Drive` when present, not the bare mount, because that is where personal files live and the mount root also holds shared drives.

## What MGA refuses to guess

Two notes appear on the configure step, both shaped by what the server cannot know:

- **Overlap with the Drive API connection** is raised only when such a connection exists, and phrased as "if this folder holds the same games". MGA genuinely cannot tell: the API connection stores Drive-side paths, this one stores a local mount, and neither records which Google account it belongs to. The cost of asserting a false overlap is someone deleting a working connection.
- **Streaming is not detected at all.** Drive for Desktop decides per folder whether content is on disk or fetched on read, and there is no reliable way to ask from here. Since MGA serves the actual bytes, a scan can pull anything not yet local — so the user is told what to check rather than shown a guess dressed as a fact.

## When nothing is found

The folder picker offers mount instructions instead of "No subfolders found". rclone for Linux and macOS, the official client for Windows and macOS, and **text to copy on every platform rather than a `.ps1` to download** — Windows blocks a downloaded script under the default execution policy, so a file would be something the user cannot run. The platform is guessed from the browser and switchable in one click, because the machine someone browses from is usually not the machine running MGA.

## Verification

Against the running server, with three real accounts mounted:

| Check | Result |
|---|---|
| Both ids discovered from one binary | `game-source-google-drive-desktop` and `game-source-local`, both 1.1.0 |
| Browse the new source, no path | Three accounts by address, each pointing at its `My Drive` |
| Browse the local source, no path | `C:`, `G:`, `H:`, `I:` — unchanged |
| Drill into a chosen account | Its folders, including `Games` |
| `check_config` with a Drive base | `ok`, with a stable source identity |
| Wizard | "Google Drive for Desktop — Location required", distinct from "Google Drive — Sign-in required" |
| Both configure-step notes | Render; the overlap one only because a Drive API connection exists here |

The empty-mount branch cannot occur on a machine with Drive mounted, so it was forced by making the browse call return nothing; the Windows and Linux variants were then read back from the rendered panel.

No connection was created. A second source over the same Drive would add its games again alongside the existing API connection, and that is the owner's decision rather than a verification step.

## Migration

`NO_MIGRATION_NEEDED`. A new plugin id over the existing `base_path` plus relative `include_paths` contract. No durable state changes and no existing connection is touched.

The new id had to be registered everywhere the local source already was — filesystem scope, content delivery, duplicate-connection messages, save-domain resolution, and four frontend registrations. Omitting it from `sourcescope` would have let `NormalizeConfig` strip `base_path` before schema validation and fail every attempt to create a connection, which is the trap ADR-0044 already documents.
