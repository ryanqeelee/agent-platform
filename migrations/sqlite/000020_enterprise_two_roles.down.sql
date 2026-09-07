-- Refuse a lossy role rollback. Restore a pre-migration backup instead.
SELECT * FROM two_role_rollback_requires_pre_migration_backup;
