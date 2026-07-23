1. Enforce fast fail policy.
2. Always prefer object-oriented design when possible.
3. Begin with `docs/agent-bootstrap.md`.
4. Jira MGA is the only source of truth for open work, priority, assignment, and progress. Set `Assigned agent` when claiming work and keep the issue status current. Do not create local task lists or roadmaps.
5. Confluence MGA is the source of truth for current product, UX, architecture, security, and operating guidance.
6. Git remains authoritative for executable code, immutable migrations, tests, release artifacts, and code-coupled rules. Persisted-data changes require a versioned migration or an explicit `NO_MIGRATION_NEEDED` note.
7. Follow `docs/architecture/agent-responsibility-boundary.md` for unresolved decisions and escalation.
