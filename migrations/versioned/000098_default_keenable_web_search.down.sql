-- This data migration is intentionally retained on downgrade. Provider rows
-- may already be referenced or administered, so deleting them is unsafe.
DO $$ BEGIN RAISE NOTICE '[Migration 000098] No-op rollback (provisioning is one-way)'; END $$;
