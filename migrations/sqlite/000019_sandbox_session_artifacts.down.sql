DROP INDEX IF EXISTS uq_tenant_sandbox_configs_tenant_name;
DROP INDEX IF EXISTS idx_tenant_sandbox_configs_deleted_at;
DROP INDEX IF EXISTS idx_tenant_sandbox_configs_tenant_id;
DROP TABLE IF EXISTS tenant_sandbox_configs;

ALTER TABLE messages DROP COLUMN artifacts;
ALTER TABLE sessions DROP COLUMN sandbox_config_id;
