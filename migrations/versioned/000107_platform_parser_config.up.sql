-- Platform parser connections and default chat parser rules are deployment
-- infrastructure. Consolidate legacy copies only when every non-empty value is
-- JSONB-equivalent. Secret-bearing legacy JSON cannot be copied because it was
-- stored as plaintext; the operator must clear it and reconfigure through the
-- encrypted platform API after migration.
CREATE TABLE platform_parser_config (
    id         SMALLINT PRIMARY KEY CHECK (id = 1),
    config     JSONB NOT NULL,
    updated_by VARCHAR(36) NOT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP
);

DO $$
BEGIN
    IF (
        SELECT COUNT(DISTINCT parser_engine_config)
        FROM tenants
        WHERE parser_engine_config IS NOT NULL
          AND parser_engine_config <> '{}'::jsonb
          AND parser_engine_config <> 'null'::jsonb
    ) > 1 THEN
        RAISE EXCEPTION 'migration 000107: conflicting tenant parser engine configurations require explicit consolidation';
    END IF;

    IF EXISTS (
        SELECT 1
        FROM tenants
        WHERE parser_engine_config IS NOT NULL
          AND (
              COALESCE(parser_engine_config ->> 'mineru_api_key', '') <> ''
              OR COALESCE(parser_engine_config ->> 'paddleocr_vl_cloud_token', '') <> ''
          )
    ) THEN
        RAISE EXCEPTION 'migration 000107: plaintext tenant parser credentials cannot be copied; clear them and reconfigure through the platform parser API after migration';
    END IF;
END $$;

INSERT INTO platform_parser_config (id, config, updated_by)
SELECT 1, parser_engine_config, 'migration-000107'
FROM tenants
WHERE parser_engine_config IS NOT NULL
  AND parser_engine_config <> '{}'::jsonb
  AND parser_engine_config <> 'null'::jsonb
GROUP BY parser_engine_config;

ALTER TABLE tenants DROP COLUMN parser_engine_config;
