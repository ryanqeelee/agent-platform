ALTER TABLE tenants
    ADD COLUMN sandbox_scripts_disabled BOOLEAN NOT NULL DEFAULT 0;

UPDATE tenants
SET sandbox_scripts_disabled = 1
WHERE EXISTS (
    SELECT 1
    FROM tenant_sandbox_configs AS legacy
    WHERE legacy.tenant_id = tenants.id
      AND legacy.name = '__workspace_scripts_policy__'
      AND legacy.deleted_at IS NULL
);

DELETE FROM tenant_sandbox_configs
WHERE name = '__workspace_scripts_policy__';

DROP INDEX IF EXISTS uq_tenant_sandbox_configs_tenant_name;
DROP INDEX IF EXISTS idx_tenant_sandbox_configs_deleted_at;
DROP INDEX IF EXISTS idx_tenant_sandbox_configs_tenant_id;

ALTER TABLE tenant_sandbox_configs RENAME TO platform_sandbox_configs;
ALTER TABLE platform_sandbox_configs DROP COLUMN tenant_id;
ALTER TABLE platform_sandbox_configs
    ADD COLUMN is_default BOOLEAN NOT NULL DEFAULT 0;

CREATE INDEX IF NOT EXISTS idx_platform_sandbox_configs_name
    ON platform_sandbox_configs(name) WHERE deleted_at IS NULL;
CREATE INDEX IF NOT EXISTS idx_platform_sandbox_configs_deleted_at
    ON platform_sandbox_configs(deleted_at);
CREATE UNIQUE INDEX IF NOT EXISTS uq_platform_sandbox_configs_active_default
    ON platform_sandbox_configs(is_default)
    WHERE is_default = 1 AND deleted_at IS NULL;
