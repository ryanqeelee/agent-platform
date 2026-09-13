-- Irreversible after platform skill state exists. Restore a pre-migration DB.
CREATE TABLE migration_000034_requires_backup (
    guard INTEGER NOT NULL CHECK (guard = 1)
);
INSERT INTO migration_000034_requires_backup (guard) VALUES (0);
