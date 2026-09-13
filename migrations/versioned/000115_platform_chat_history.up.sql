-- Message indexing is one platform policy. Tenant rows retain only a private
-- hidden-KB binding created lazily on the first eligible message.
CREATE TABLE platform_chat_history_config (
    id                 SMALLINT PRIMARY KEY CHECK (id = 1),
    enabled            BOOLEAN NOT NULL DEFAULT FALSE,
    embedding_model_id VARCHAR(64) NOT NULL DEFAULT '',
    updated_by         VARCHAR(36) NOT NULL,
    created_at         TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at         TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP,
    CHECK (NOT enabled OR embedding_model_id <> '')
);

INSERT INTO platform_chat_history_config (id, enabled, embedding_model_id, updated_by)
VALUES (1, FALSE, '', 'migration-000115');

-- Legacy KB pointers cannot be silently detached. The known deployment has no
-- such rows; any unexpected row requires an explicit operator cleanup/reindex
-- decision before this migration is replayed.
DO $$
BEGIN
    IF EXISTS (
        SELECT 1
        FROM tenants
        WHERE chat_history_config IS NOT NULL
          AND COALESCE(chat_history_config ->> 'knowledge_base_id', '') <> ''
    ) THEN
        RAISE EXCEPTION 'migration 000115: legacy chat-history KB bindings require explicit cleanup before platform consolidation';
    END IF;
END $$;

CREATE TABLE tenant_chat_history_indexes (
    tenant_id         INTEGER PRIMARY KEY REFERENCES tenants(id) ON DELETE CASCADE,
    knowledge_base_id VARCHAR(36) NOT NULL UNIQUE REFERENCES knowledge_bases(id),
    created_at        TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at        TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP
);

ALTER TABLE tenants DROP COLUMN chat_history_config;
