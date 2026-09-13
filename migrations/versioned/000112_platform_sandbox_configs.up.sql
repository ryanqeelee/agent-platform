-- Sandbox connections are platform control-plane records. Promote the table
-- in place so config IDs, encrypted payloads, timestamps and soft-delete state
-- retain one identity; there is no second tenant-owned compatibility copy.
ALTER TABLE tenants
    ADD COLUMN IF NOT EXISTS sandbox_scripts_disabled BOOLEAN NOT NULL DEFAULT FALSE;

UPDATE tenants AS t
SET sandbox_scripts_disabled = TRUE
WHERE EXISTS (
    SELECT 1
    FROM tenant_sandbox_configs AS legacy
    WHERE legacy.tenant_id = t.id
      AND legacy.name = '__workspace_scripts_policy__'
      AND legacy.deleted_at IS NULL
);

DELETE FROM tenant_sandbox_configs
WHERE name = '__workspace_scripts_policy__';

DROP INDEX IF EXISTS uq_tenant_sandbox_configs_tenant_name;
DROP INDEX IF EXISTS idx_tenant_sandbox_configs_tenant;

ALTER TABLE tenant_sandbox_configs
    RENAME TO platform_sandbox_configs;

ALTER TABLE platform_sandbox_configs
    DROP COLUMN tenant_id,
    ADD COLUMN IF NOT EXISTS is_default BOOLEAN NOT NULL DEFAULT FALSE;

COMMENT ON TABLE platform_sandbox_configs IS
    'Platform-owned sandbox provider connections. Enterprise sessions and permissions remain tenant-owned.';
COMMENT ON COLUMN platform_sandbox_configs.config IS
    'Encrypted provider configuration; shape matches types.TenantSandboxConfig.';
COMMENT ON COLUMN platform_sandbox_configs.is_default IS
    'Explicit platform default used only when a new business session has no pinned config.';

CREATE INDEX IF NOT EXISTS idx_platform_sandbox_configs_name
    ON platform_sandbox_configs (name) WHERE deleted_at IS NULL;
CREATE INDEX IF NOT EXISTS idx_platform_sandbox_configs_deleted_at
    ON platform_sandbox_configs (deleted_at);
CREATE UNIQUE INDEX IF NOT EXISTS uq_platform_sandbox_configs_active_default
    ON platform_sandbox_configs (is_default)
    WHERE is_default AND deleted_at IS NULL;
