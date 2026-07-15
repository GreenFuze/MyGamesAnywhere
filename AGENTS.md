1. Enforce fast fail policy.
2. Always prefer object-oriented design when possible.
3. Always prefer using SuitCode when possible. As it is slower, it saves tokens.
4. DB schema, persisted SQLite data, or persisted JSON/config changes require a versioned migration or an explicit `NO_MIGRATION_NEEDED` note explaining why existing installs remain safe.
5. Follow `docs/architecture/agent-responsibility-boundary.md`. A regular implementation agent must stop before unresolved architecture, persistence, protocol, security, elevation, identity, destructive-filesystem, dependency, or product-policy decisions; an architecture-capable agent records the decision and bounded implementation contract first.
