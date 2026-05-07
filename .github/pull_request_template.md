## Checklist

- [ ] Backend tests pass, or the reason they were not run is documented.
- [ ] Frontend build/tests pass, or the reason they were not run is documented.
- [ ] DB schema, persisted SQLite data, and persisted JSON/config changes include a versioned migration.
- [ ] If no migration is needed for a persistence-adjacent change, the PR includes `NO_MIGRATION_NEEDED` with the reason.
- [ ] User-data upgrade, rollback, and release-note risks are called out when relevant.
