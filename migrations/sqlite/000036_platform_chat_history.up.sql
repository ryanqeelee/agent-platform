CREATE TABLE platform_chat_history_config (
    id                 INTEGER PRIMARY KEY CHECK (id = 1),
    enabled            INTEGER NOT NULL DEFAULT 0,
    embedding_model_id TEXT NOT NULL DEFAULT '',
    updated_by         TEXT NOT NULL,
    created_at         DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at         DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
    CHECK (NOT enabled OR embedding_model_id <> '')
);

INSERT INTO platform_chat_history_config (id, enabled, embedding_model_id, updated_by)
VALUES (1, 0, '', 'migration-000036');

CREATE TEMP TABLE _legacy_chat_history_binding_guard (
    invalid INTEGER NOT NULL CHECK (invalid = 0)
);

INSERT INTO _legacy_chat_history_binding_guard (invalid)
SELECT CASE WHEN EXISTS (
    SELECT 1
    FROM tenants
    WHERE CASE
        WHEN chat_history_config IS NULL THEN 0
        WHEN NOT json_valid(chat_history_config) THEN 1
        WHEN COALESCE(json_extract(chat_history_config, '$.knowledge_base_id'), '') <> '' THEN 1
        ELSE 0
    END = 1
) THEN 1 ELSE 0 END;

DROP TABLE _legacy_chat_history_binding_guard;

CREATE TABLE tenant_chat_history_indexes (
    tenant_id         INTEGER PRIMARY KEY REFERENCES tenants(id) ON DELETE CASCADE,
    knowledge_base_id TEXT NOT NULL UNIQUE REFERENCES knowledge_bases(id),
    created_at        DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at        DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP
);

ALTER TABLE tenants DROP COLUMN chat_history_config;
