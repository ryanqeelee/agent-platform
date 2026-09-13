-- Irreversible: platform rows have no truthful tenant_id to restore.
CREATE TABLE migration_000033_requires_backup (
    guard INTEGER NOT NULL CHECK (guard = 1)
);
INSERT INTO migration_000033_requires_backup (guard) VALUES (0);
