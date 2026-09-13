-- SQLite/Lite counterpart of PostgreSQL migration 000114.
CREATE TEMP TABLE _memory_runtime_guard (
    invalid INTEGER NOT NULL CHECK (invalid = 0)
);

INSERT INTO _memory_runtime_guard (invalid)
SELECT CASE WHEN EXISTS (
    SELECT 1 FROM tenants
    WHERE memory_config IS NOT NULL
      AND (NOT json_valid(memory_config) OR
           (json_type(memory_config) IS NOT NULL AND json_type(memory_config) <> 'object'))
) THEN 1 ELSE 0 END;

DELETE FROM _memory_runtime_guard;

INSERT INTO _memory_runtime_guard (invalid)
SELECT CASE WHEN EXISTS (
    SELECT 1
    FROM tenants, json_each(CASE
        WHEN memory_config IS NULL OR json_type(memory_config) IS NULL THEN '{}'
        ELSE memory_config
    END)
    WHERE json_each.key NOT IN (
        'enabled', 'write_mode', 'extract_model_id', 'max_items',
        'extract_delay_seconds', 'extract_min_interval_seconds',
        'extract_instructions', 'interest_threshold', 'embedding_model_id',
        'vector_recall', 'retrieval_conditioning'
    )
) THEN 1 ELSE 0 END;

DELETE FROM _memory_runtime_guard;

INSERT INTO _memory_runtime_guard (invalid)
SELECT CASE WHEN EXISTS (
    SELECT 1 FROM tenants
    WHERE memory_config IS NOT NULL
      AND json_type(memory_config) = 'object'
      AND (
          CASE WHEN json_type(memory_config, '$.extract_model_id') IS NOT NULL
              THEN json_type(memory_config, '$.extract_model_id') NOT IN ('text', 'null')
                   OR trim(COALESCE(json_extract(memory_config, '$.extract_model_id'), '')) <> ''
              ELSE 0 END
          OR CASE WHEN json_type(memory_config, '$.embedding_model_id') IS NOT NULL
              THEN json_type(memory_config, '$.embedding_model_id') NOT IN ('text', 'null')
                   OR trim(COALESCE(json_extract(memory_config, '$.embedding_model_id'), '')) <> ''
              ELSE 0 END
          OR CASE WHEN json_type(memory_config, '$.extract_instructions') IS NOT NULL
              THEN json_type(memory_config, '$.extract_instructions') NOT IN ('text', 'null')
                   OR trim(COALESCE(json_extract(memory_config, '$.extract_instructions'), '')) <> ''
              ELSE 0 END
          OR CASE WHEN json_type(memory_config, '$.max_items') IS NOT NULL
              THEN json_type(memory_config, '$.max_items') NOT IN ('integer', 'null')
                   OR (json_type(memory_config, '$.max_items') = 'integer' AND
                       NOT (json_extract(memory_config, '$.max_items') <= 0 OR json_extract(memory_config, '$.max_items') = 200))
              ELSE 0 END
          OR CASE WHEN json_type(memory_config, '$.extract_delay_seconds') IS NOT NULL
              THEN json_type(memory_config, '$.extract_delay_seconds') NOT IN ('integer', 'null')
                   OR (json_type(memory_config, '$.extract_delay_seconds') = 'integer' AND
                       NOT (json_extract(memory_config, '$.extract_delay_seconds') <= 0 OR json_extract(memory_config, '$.extract_delay_seconds') = 90))
              ELSE 0 END
          OR CASE WHEN json_type(memory_config, '$.extract_min_interval_seconds') IS NOT NULL
              THEN json_type(memory_config, '$.extract_min_interval_seconds') NOT IN ('integer', 'null')
                   OR (json_type(memory_config, '$.extract_min_interval_seconds') = 'integer' AND
                       NOT (json_extract(memory_config, '$.extract_min_interval_seconds') <= 0 OR json_extract(memory_config, '$.extract_min_interval_seconds') = 300))
              ELSE 0 END
          OR CASE WHEN json_type(memory_config, '$.interest_threshold') IS NOT NULL
              THEN json_type(memory_config, '$.interest_threshold') NOT IN ('integer', 'null')
                   OR (json_type(memory_config, '$.interest_threshold') = 'integer' AND
                       NOT (json_extract(memory_config, '$.interest_threshold') <= 0 OR json_extract(memory_config, '$.interest_threshold') = 3))
              ELSE 0 END
          OR CASE WHEN json_type(memory_config, '$.vector_recall') IS NOT NULL
              THEN json_type(memory_config, '$.vector_recall') NOT IN ('true', 'null')
              ELSE 0 END
          OR CASE WHEN json_type(memory_config, '$.retrieval_conditioning') IS NOT NULL
              THEN json_type(memory_config, '$.retrieval_conditioning') NOT IN ('true', 'null')
              ELSE 0 END
      )
) THEN 1 ELSE 0 END;

DROP TABLE _memory_runtime_guard;

CREATE TABLE platform_memory_runtime_config (
    id         INTEGER PRIMARY KEY CHECK (id = 1),
    runtime    TEXT NOT NULL CHECK (json_valid(runtime) AND json_type(runtime) = 'object'),
    generation INTEGER NOT NULL DEFAULT 0 CHECK (generation >= 0),
    updated_by TEXT NOT NULL,
    created_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP
);

INSERT INTO platform_memory_runtime_config (id, runtime, generation, updated_by)
VALUES (1, json_object(
    'extract_model_id', '',
    'max_items', 200,
    'extract_delay_seconds', 90,
    'extract_min_interval_seconds', 300,
    'extract_instructions', '',
    'interest_threshold', 3,
    'embedding_model_id', '',
    'vector_recall', NULL,
    'retrieval_conditioning', NULL
), 0, 'migration-000035');

UPDATE tenants
SET memory_config = json_object(
    'enabled', json(CASE
        WHEN json_type(memory_config, '$.enabled') = 'true' THEN 'true'
        ELSE 'false'
    END),
    'write_mode', CASE
        WHEN json_extract(memory_config, '$.write_mode') = 'auto' THEN 'auto'
        ELSE 'explicit_only'
    END
);
