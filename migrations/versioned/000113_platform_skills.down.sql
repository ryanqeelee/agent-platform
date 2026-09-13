-- Ownership collapse is intentionally irreversible: catalog/install/snapshot
-- rows no longer carry a truthful tenant_id. Restore a pre-migration backup.
DO $$ BEGIN
    RAISE EXCEPTION 'platform skill ownership migration requires a pre-migration database backup to roll back';
END $$;
