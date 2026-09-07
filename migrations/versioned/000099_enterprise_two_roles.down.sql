-- Role consolidation is intentionally irreversible: do not guess which
-- administrator used to be owner or restore broad contributor permissions.
DO $$ BEGIN RAISE EXCEPTION 'Two-role consolidation requires a pre-migration database backup to roll back'; END $$;
