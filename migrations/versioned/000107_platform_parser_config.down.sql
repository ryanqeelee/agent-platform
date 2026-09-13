-- A platform-wide value cannot be projected back into independent tenant
-- authority without inventing ownership. Refuse a lossy downgrade when a
-- non-empty global configuration exists.
DO $$
BEGIN
    IF EXISTS (
        SELECT 1 FROM platform_parser_config
        WHERE config <> '{}'::jsonb AND config <> 'null'::jsonb
    ) THEN
        RAISE EXCEPTION 'migration 000107 down: platform parser configuration must be cleared before downgrade';
    END IF;
END $$;

ALTER TABLE tenants ADD COLUMN parser_engine_config JSONB;
DROP TABLE platform_parser_config;
