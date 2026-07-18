# Next Release Notes (Development)

These notes track upgrade-sensitive work after v0.2.3. They are renamed or
folded into the numbered release notes when the next version is selected.

## Changes

- MGA Client now separates new managed installations by MGA Server and blocks
  cross-server path/product races.
- Devices show games managed here, managed elsewhere, or released for pickup.
  Release and Pick up require native client confirmation and preserve files.

## Upgrade and migration notes

- Client config schema 3 adds stable per-server binding IDs and migrates older
  bindings only after their protected keys are verified.
- Client ownership-catalog schema 1, archive manifest schema 3, and GOG
  manifest schema 4 enforce local owner identity. A single-binding legacy
  manifest is claimed lazily; ambiguous multi-binding legacy state fails closed.
- Server migration 26 adds managed-install observations to device inventory.
