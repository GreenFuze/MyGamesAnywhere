# ADR-0031: Persisted save compatibility and converter registry

- **Status:** Accepted
- **Date:** 2026-07-23
- **Scope:** MGA Server save-domain compatibility evidence, converter discovery,
  conversion safety, and SQLite persistence
- **Jira:** MGA-33
- **Canonical record:** [MGA Confluence ADR-0031](https://greenfuzer.atlassian.net/wiki/spaces/MG/pages/3342338/ADR-0031+Persisted+Save+Compatibility+and+Converter+Registry)
- **Depends on:** ADR-0017, ADR-0018, ADR-0026

## Context

Different routes for the same displayed game may use incompatible save formats.
A matching title, canonical-game membership, platform, filename, directory
name, or timestamp is not compatibility evidence.

## Decision

Compatibility is an explicit global catalog fact keyed by an ordered pair of
namespaced, versioned save formats. Each rule is `same_format` or `converter`
and records attribution, versioned evidence, direction, reversibility, enabled
state, and audit timestamps. The model has no title field or title input.

Converter metadata is persisted, but only a `builtin` converter whose running
code exactly matches the persisted ID, version, endpoints, attribution, and
reversibility may execute. Persisted metadata alone is never executable.

Converters receive a cloned bounded snapshot and return a separate candidate.
They cannot access the destination writer. The conversion service validates
the rule, code registration, output format, and candidate before invoking an
`AtomicSnapshotWriter`. Errors, panics, invalid output, cancellation, and
registry mismatch do not invoke the writer or mutate the input.

This primitive does not bypass local Save Domain authority, reconciliation,
provider-opaque boundaries, or conflict policy. It adds no cross-route player
action by itself.

## Persistence and compatibility

Migration 30 creates `save_converter_registry` and
`save_compatibility_rules`. Existing installations receive empty registries,
so no route becomes compatible automatically. Migration 29 is already applied
and must never be edited.

Disabling a rule or converter is the rollback path. Registry changes never
rewrite existing snapshots or local save files.

## Security and failure behavior

- Identifiers, versions, attribution, and evidence are trimmed, single-line,
  and length bounded.
- Evidence is bounded valid JSON and must not contain credentials or local
  paths.
- Unknown/disabled rules, absent or mismatched code, converter panics, and
  invalid candidates fail closed.
- Input and output bytes are cloned across the converter boundary.
- Provider-opaque routes remain inaccessible.

## Acceptance criteria

- Matching title is structurally incapable of establishing compatibility.
- Rules and converters are versioned, attributable, directional, persisted,
  revocable, and testable.
- Only exact code-backed converters can run.
- Failed conversion never invokes the atomic writer or changes the input.
- Migration and repository tests prove an empty safe upgrade, exact lookup,
  directionality, revocation, and idempotence.
