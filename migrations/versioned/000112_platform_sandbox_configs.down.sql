-- Ownership collapse is intentionally irreversible: a platform connection has
-- no truthful tenant_id to restore. Roll back with a pre-migration backup.
DO $$ BEGIN
    RAISE EXCEPTION 'platform sandbox ownership migration requires a pre-migration database backup to roll back';
END $$;
