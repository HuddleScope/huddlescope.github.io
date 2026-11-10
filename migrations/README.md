# Database Migrations

## Policy

This repository uses **forward-only versioned SQL migrations** for schema evolution.

### Why forward-only?

- Turso Cloud (serverless SQLite) does not support `DROP COLUMN` without recreating the table.
- Reversible down-migrations often fail in practice due to data changes, irreversible operations, or third-party dependencies.
- For rollbacks, restore from a recent backup instead of running down-migrations.

### Naming

Files are named `NNN_description.sql` where:

- `NNN` is a zero-padded integer (001, 002, ...) — determines order.
- `description` is snake_case (e.g., `add_team_slug`).
- `git` sort order must match numeric order.

Example:

```sh
migrations/
├── 001_initial_schema.sql
├── 002_add_archive_column.sql
└── ...
```

### Adding a new migration

1. Find the highest existing number, increment it.
2. Create `migrations/NNN_your_description.sql`.
3. Add `PRAGMA foreign_keys = ON;` at the top if your script uses foreign keys.
4. Write forward-only SQL; assume the table/column already exists and use `IF NOT EXISTS`.
5. Commit both the migration file and `go.sum` (when dependencies change).

### Turso compatibility

- Use `strftime('%Y-%m-%dT%H:%M:%fZ','now')` for UTC timestamps.
- Use `CHECK (length(col)=20 AND substr(col,20)='Z')` to enforce ISO-8601 UTC.
- Use `PRAGMA foreign_keys=ON` before schema-altering operations.
- Do not use `DROP TABLE IF EXISTS` unless you are certain it is safe; prefer `RENAME TABLE`.

## Backup and Restore

See `scripts/backup-restore.sh` for documented ops procedures.

For development, you can run:

```bash
# Export schema and data (SQLite-style dump)
sqlite3 path/to/db "SELECT 'PRAGMA foreign_keys=OFF;'; .dump" > backup.sql

# Restore
sqlite3 path/to/db < backup.sql
```

For Turso, use `turso db shell` with the same `SELECT .dump` pattern.
