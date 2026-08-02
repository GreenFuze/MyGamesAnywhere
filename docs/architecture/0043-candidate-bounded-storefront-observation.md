# ADR-0043: Candidate-Bounded Storefront Observation and Launch-Only Grants

Status: Accepted for implementation
Date: 2026-08-01
Jira: MGA-20
Canonical decision: Confluence page `ADR-0043 — Candidate-Bounded Storefront Observation and Launch-Only Grants`

This local mirror records the protocol and migration contract next to the code. Confluence remains authoritative.

## Decision

- The server supplies a bounded list of exact storefront candidates already present in the authenticated profile's library. The client never uploads a general installed-application list.
- Provider adapters use authoritative identities only. The initial Steam adapter matches a numeric App ID to the exact `appmanifest_<appid>.acf` under registered Steam libraries; title matching is forbidden.
- Observation is not authority. `Use existing` requires a native device confirmation and creates a binding-local launch-only grant.
- Storefront launch commands contain typed provider/product identities. The client constructs the provider route and revalidates both the grant and local installation before every launch.
- Storefront-owned games never enter MGA's managed ownership catalog and cannot be updated, repaired, cleaned up, moved, or uninstalled through managed-install APIs. `Pick up` remains exclusive to released MGA-managed installations.
- Launch grants use a separate versioned client catalog; candidate sets remain request-scoped and are never persisted by the client. Server observations/grants use separate persistence from `device_game_installations`.
- Manual and scheduled server reconciliation send profile-scoped candidates through the same typed inventory command. Unsolicited client inventory has no storefront observations because it has no authenticated profile scope.
- Inventory schema advances from 7 to 8. Server migration 37 adds profile-scoped storefront observation/grant persistence. Existing installs remain intact.
- Providers without an exact local mapping fail closed. Later Xbox/GOG adapters must use the same bounded contract and may not fall back to title or broad filesystem matching.

## Compatibility and privacy

Old clients do not advertise the new capabilities, so the UI hides the flow. Candidate and result counts and field sizes are bounded. Only exact matches are reported, and unrelated applications, accounts, registry entries, and files remain local.
