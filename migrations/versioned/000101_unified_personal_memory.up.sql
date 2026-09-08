ALTER TABLE tenants
    ADD COLUMN IF NOT EXISTS memory_generation BIGINT NOT NULL DEFAULT 0;

ALTER TABLE memory_subjects
    ADD COLUMN IF NOT EXISTS generation BIGINT NOT NULL DEFAULT 0;
ALTER TABLE memory_subjects
    ADD COLUMN IF NOT EXISTS revision BIGINT NOT NULL DEFAULT 0;

ALTER TABLE memory_items
    ADD COLUMN IF NOT EXISTS scope VARCHAR(16) NOT NULL DEFAULT 'employee';

CREATE INDEX IF NOT EXISTS idx_memory_items_projection
    ON memory_items (tenant_id, subject_id, scope, status, expires_at);

CREATE TABLE IF NOT EXISTS memory_command_receipts (
    id VARCHAR(36) PRIMARY KEY,
    tenant_id BIGINT NOT NULL,
    subject_id VARCHAR(512) NOT NULL,
    operation_id VARCHAR(128) NOT NULL,
    command_hash VARCHAR(64) NOT NULL,
    status VARCHAR(16) NOT NULL,
    reason_code VARCHAR(32),
    revision BIGINT NOT NULL,
    workspace_generation BIGINT NOT NULL,
    subject_generation BIGINT NOT NULL,
    item_ids JSONB NOT NULL DEFAULT '[]'::jsonb,
    committed_at TIMESTAMP WITH TIME ZONE,
    created_at TIMESTAMP WITH TIME ZONE NOT NULL DEFAULT CURRENT_TIMESTAMP,
    CONSTRAINT memory_command_receipts_scope_unique
        UNIQUE (tenant_id, subject_id, operation_id)
);

CREATE TABLE IF NOT EXISTS memory_expressions (
    id VARCHAR(36) PRIMARY KEY,
    tenant_id BIGINT NOT NULL,
    subject_id VARCHAR(512) NOT NULL,
    expression_id VARCHAR(128) NOT NULL,
    expression_hash VARCHAR(64) NOT NULL,
    runtime VARCHAR(16) NOT NULL,
    session_id VARCHAR(128) NOT NULL,
    message_id VARCHAR(128) NOT NULL,
    chat_model_id VARCHAR(64) NOT NULL DEFAULT '',
    text TEXT,
    status VARCHAR(16) NOT NULL,
    outcome_reason VARCHAR(64),
    workspace_generation BIGINT NOT NULL,
    subject_generation BIGINT NOT NULL,
    created_at TIMESTAMP WITH TIME ZONE NOT NULL DEFAULT CURRENT_TIMESTAMP,
    processed_at TIMESTAMP WITH TIME ZONE,
    CONSTRAINT memory_expressions_scope_unique
        UNIQUE (tenant_id, subject_id, expression_id)
);

CREATE INDEX IF NOT EXISTS idx_memory_expressions_pending
    ON memory_expressions (tenant_id, subject_id, status, created_at);

-- Runtime/migration source IDs are opaque strings, not necessarily UUIDs.
ALTER TABLE memory_items ALTER COLUMN source_session_id TYPE VARCHAR(128);
ALTER TABLE memory_items ALTER COLUMN source_message_id TYPE VARCHAR(128);
ALTER TABLE memory_tombstones ALTER COLUMN source_message_id TYPE VARCHAR(128);
