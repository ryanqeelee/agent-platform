-- Memory runtime tuning is deployment infrastructure. Enterprises retain only
-- consent (enabled/write_mode); non-default legacy runtime values require an
-- explicit operator decision instead of selecting one enterprise implicitly.
CREATE TABLE platform_memory_runtime_config (
    id         SMALLINT PRIMARY KEY CHECK (id = 1),
    runtime    JSONB NOT NULL CHECK (jsonb_typeof(runtime) = 'object'),
    generation BIGINT NOT NULL DEFAULT 0 CHECK (generation >= 0),
    updated_by VARCHAR(36) NOT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP
);

DO $$
BEGIN
    IF EXISTS (
        SELECT 1
        FROM tenants
        WHERE memory_config IS NOT NULL
          AND memory_config <> 'null'::jsonb
          AND jsonb_typeof(memory_config) <> 'object'
    ) THEN
        RAISE EXCEPTION 'migration 000114: tenant memory configuration must be an object';
    END IF;

    IF EXISTS (
        SELECT 1
        FROM tenants t, LATERAL jsonb_object_keys(COALESCE(NULLIF(t.memory_config, 'null'::jsonb), '{}'::jsonb)) AS key
        WHERE key NOT IN (
            'enabled', 'write_mode', 'extract_model_id', 'max_items',
            'extract_delay_seconds', 'extract_min_interval_seconds',
            'extract_instructions', 'interest_threshold', 'embedding_model_id',
            'vector_recall', 'retrieval_conditioning'
        )
    ) THEN
        RAISE EXCEPTION 'migration 000114: tenant memory configuration contains an unknown field';
    END IF;

    IF EXISTS (
        SELECT 1
        FROM tenants
        WHERE memory_config IS NOT NULL
          AND memory_config <> 'null'::jsonb
          AND (
              CASE WHEN memory_config ? 'extract_model_id'
                  THEN jsonb_typeof(memory_config->'extract_model_id') NOT IN ('string', 'null')
                       OR btrim(COALESCE(memory_config->>'extract_model_id', '')) <> ''
                  ELSE FALSE END
              OR CASE WHEN memory_config ? 'embedding_model_id'
                  THEN jsonb_typeof(memory_config->'embedding_model_id') NOT IN ('string', 'null')
                       OR btrim(COALESCE(memory_config->>'embedding_model_id', '')) <> ''
                  ELSE FALSE END
              OR CASE WHEN memory_config ? 'extract_instructions'
                  THEN jsonb_typeof(memory_config->'extract_instructions') NOT IN ('string', 'null')
                       OR btrim(COALESCE(memory_config->>'extract_instructions', '')) <> ''
                  ELSE FALSE END
              OR CASE WHEN memory_config ? 'max_items'
                  THEN jsonb_typeof(memory_config->'max_items') NOT IN ('number', 'null')
                       OR (jsonb_typeof(memory_config->'max_items') = 'number' AND
                           ((memory_config->>'max_items')::numeric <> trunc((memory_config->>'max_items')::numeric)
                            OR NOT ((memory_config->>'max_items')::numeric <= 0 OR (memory_config->>'max_items')::numeric = 200)))
                  ELSE FALSE END
              OR CASE WHEN memory_config ? 'extract_delay_seconds'
                  THEN jsonb_typeof(memory_config->'extract_delay_seconds') NOT IN ('number', 'null')
                       OR (jsonb_typeof(memory_config->'extract_delay_seconds') = 'number' AND
                           ((memory_config->>'extract_delay_seconds')::numeric <> trunc((memory_config->>'extract_delay_seconds')::numeric)
                            OR NOT ((memory_config->>'extract_delay_seconds')::numeric <= 0 OR (memory_config->>'extract_delay_seconds')::numeric = 90)))
                  ELSE FALSE END
              OR CASE WHEN memory_config ? 'extract_min_interval_seconds'
                  THEN jsonb_typeof(memory_config->'extract_min_interval_seconds') NOT IN ('number', 'null')
                       OR (jsonb_typeof(memory_config->'extract_min_interval_seconds') = 'number' AND
                           ((memory_config->>'extract_min_interval_seconds')::numeric <> trunc((memory_config->>'extract_min_interval_seconds')::numeric)
                            OR NOT ((memory_config->>'extract_min_interval_seconds')::numeric <= 0 OR (memory_config->>'extract_min_interval_seconds')::numeric = 300)))
                  ELSE FALSE END
              OR CASE WHEN memory_config ? 'interest_threshold'
                  THEN jsonb_typeof(memory_config->'interest_threshold') NOT IN ('number', 'null')
                       OR (jsonb_typeof(memory_config->'interest_threshold') = 'number' AND
                           ((memory_config->>'interest_threshold')::numeric <> trunc((memory_config->>'interest_threshold')::numeric)
                            OR NOT ((memory_config->>'interest_threshold')::numeric <= 0 OR (memory_config->>'interest_threshold')::numeric = 3)))
                  ELSE FALSE END
              OR CASE WHEN memory_config ? 'vector_recall'
                  THEN jsonb_typeof(memory_config->'vector_recall') NOT IN ('boolean', 'null')
                       OR (jsonb_typeof(memory_config->'vector_recall') = 'boolean' AND NOT (memory_config->>'vector_recall')::boolean)
                  ELSE FALSE END
              OR CASE WHEN memory_config ? 'retrieval_conditioning'
                  THEN jsonb_typeof(memory_config->'retrieval_conditioning') NOT IN ('boolean', 'null')
                       OR (jsonb_typeof(memory_config->'retrieval_conditioning') = 'boolean' AND NOT (memory_config->>'retrieval_conditioning')::boolean)
                  ELSE FALSE END
          )
    ) THEN
        RAISE EXCEPTION 'migration 000114: non-default tenant memory runtime configuration requires explicit platform consolidation';
    END IF;
END $$;

INSERT INTO platform_memory_runtime_config (id, runtime, generation, updated_by)
VALUES (1, jsonb_build_object(
    'extract_model_id', '',
    'max_items', 200,
    'extract_delay_seconds', 90,
    'extract_min_interval_seconds', 300,
    'extract_instructions', '',
    'interest_threshold', 3,
    'embedding_model_id', '',
    'vector_recall', NULL,
    'retrieval_conditioning', NULL
), 0, 'migration-000114');

UPDATE tenants
SET memory_config = jsonb_build_object(
    'enabled', CASE
        WHEN jsonb_typeof(memory_config->'enabled') = 'boolean'
            THEN (memory_config->>'enabled')::boolean
        ELSE FALSE
    END,
    'write_mode', CASE
        WHEN memory_config->>'write_mode' = 'auto' THEN 'auto'
        ELSE 'explicit_only'
    END
);
