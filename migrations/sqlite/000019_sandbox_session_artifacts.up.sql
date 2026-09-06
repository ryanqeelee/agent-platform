ALTER TABLE sessions ADD COLUMN sandbox_config_id VARCHAR(36);

ALTER TABLE messages ADD COLUMN artifacts TEXT DEFAULT '[]';

CREATE TABLE IF NOT EXISTS tenant_sandbox_configs (
    id VARCHAR(36) PRIMARY KEY,
    tenant_id INTEGER NOT NULL,
    name VARCHAR(255) NOT NULL,
    description TEXT,
    sandbox_type VARCHAR(32) NOT NULL,
    config TEXT NOT NULL DEFAULT '{}',
    cordoned_at DATETIME,
    created_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
    deleted_at DATETIME
);

CREATE INDEX IF NOT EXISTS idx_tenant_sandbox_configs_tenant_id
    ON tenant_sandbox_configs(tenant_id);
CREATE INDEX IF NOT EXISTS idx_tenant_sandbox_configs_deleted_at
    ON tenant_sandbox_configs(deleted_at);
CREATE UNIQUE INDEX IF NOT EXISTS uq_tenant_sandbox_configs_tenant_name
    ON tenant_sandbox_configs(tenant_id, name) WHERE deleted_at IS NULL;
