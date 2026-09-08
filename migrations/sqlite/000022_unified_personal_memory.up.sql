ALTER TABLE tenants ADD COLUMN memory_generation INTEGER NOT NULL DEFAULT 0;
ALTER TABLE memory_subjects ADD COLUMN generation INTEGER NOT NULL DEFAULT 0;
ALTER TABLE memory_subjects ADD COLUMN revision INTEGER NOT NULL DEFAULT 0;
ALTER TABLE memory_items ADD COLUMN scope TEXT NOT NULL DEFAULT 'employee';

CREATE INDEX IF NOT EXISTS idx_memory_items_projection
    ON memory_items (tenant_id, subject_id, scope, status, expires_at);

CREATE TABLE IF NOT EXISTS memory_command_receipts (
    id TEXT PRIMARY KEY,
    tenant_id INTEGER NOT NULL,
    subject_id TEXT NOT NULL,
    operation_id TEXT NOT NULL,
    command_hash TEXT NOT NULL,
    status TEXT NOT NULL,
    reason_code TEXT,
    revision INTEGER NOT NULL,
    workspace_generation INTEGER NOT NULL,
    subject_generation INTEGER NOT NULL,
    item_ids TEXT NOT NULL DEFAULT '[]',
    committed_at DATETIME,
    created_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
    UNIQUE (tenant_id, subject_id, operation_id)
);

CREATE TABLE IF NOT EXISTS memory_expressions (
    id TEXT PRIMARY KEY,
    tenant_id INTEGER NOT NULL,
    subject_id TEXT NOT NULL,
    expression_id TEXT NOT NULL,
    expression_hash TEXT NOT NULL,
    runtime TEXT NOT NULL,
    session_id TEXT NOT NULL,
    message_id TEXT NOT NULL,
    chat_model_id TEXT NOT NULL DEFAULT '',
    text TEXT,
    status TEXT NOT NULL,
    outcome_reason TEXT,
    workspace_generation INTEGER NOT NULL,
    subject_generation INTEGER NOT NULL,
    created_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
    processed_at DATETIME,
    UNIQUE (tenant_id, subject_id, expression_id)
);

CREATE INDEX IF NOT EXISTS idx_memory_expressions_pending
    ON memory_expressions (tenant_id, subject_id, status, created_at);
