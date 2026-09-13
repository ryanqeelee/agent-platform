DO $$
BEGIN
    IF EXISTS (
        SELECT 1 FROM platform_memory_runtime_config
        WHERE generation <> 0 OR runtime <> jsonb_build_object(
            'extract_model_id', '', 'max_items', 200,
            'extract_delay_seconds', 90, 'extract_min_interval_seconds', 300,
            'extract_instructions', '', 'interest_threshold', 3,
            'embedding_model_id', '', 'vector_recall', NULL,
            'retrieval_conditioning', NULL
        )
    ) THEN
        RAISE EXCEPTION 'migration 000114 down: platform memory runtime configuration must be reset before downgrade';
    END IF;
END $$;

DROP TABLE platform_memory_runtime_config;
