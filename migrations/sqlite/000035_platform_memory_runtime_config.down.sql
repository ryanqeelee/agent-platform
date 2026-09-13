CREATE TEMP TABLE _memory_runtime_downgrade_guard (
    configured INTEGER NOT NULL CHECK (configured = 0)
);

INSERT INTO _memory_runtime_downgrade_guard (configured)
SELECT CASE WHEN EXISTS (
    SELECT 1 FROM platform_memory_runtime_config
    WHERE generation <> 0 OR json(runtime) <> json(json_object(
        'extract_model_id', '', 'max_items', 200,
        'extract_delay_seconds', 90, 'extract_min_interval_seconds', 300,
        'extract_instructions', '', 'interest_threshold', 3,
        'embedding_model_id', '', 'vector_recall', NULL,
        'retrieval_conditioning', NULL
    ))
) THEN 1 ELSE 0 END;

DROP TABLE _memory_runtime_downgrade_guard;
DROP TABLE platform_memory_runtime_config;
