-- Promote the shared catalog, installations and snapshot ledger in place.
-- Tenant-private environment values and historical business sessions remain
-- tenant scoped and are not copied, rewritten or deleted here.
DROP INDEX IF EXISTS uq_tenant_skill_catalog_name;
ALTER TABLE tenant_skill_catalog RENAME TO platform_skill_catalog;
ALTER TABLE platform_skill_catalog DROP COLUMN tenant_id;

CREATE INDEX IF NOT EXISTS idx_platform_skill_catalog_name
    ON platform_skill_catalog (name) WHERE deleted_at IS NULL;
CREATE INDEX IF NOT EXISTS idx_platform_skill_catalog_deleted_at
    ON platform_skill_catalog (deleted_at);

DROP INDEX IF EXISTS uq_tenant_skills_config_name;
DROP INDEX IF EXISTS idx_tenant_skills_catalog;
ALTER TABLE tenant_skills RENAME TO platform_skills;

ALTER TABLE platform_skills
    DROP COLUMN tenant_id,
    DROP COLUMN install_session_id,
    DROP COLUMN install_message_id,
    ADD COLUMN install_run_id VARCHAR(36) NOT NULL DEFAULT '',
    ADD COLUMN install_transcript JSONB;

CREATE UNIQUE INDEX IF NOT EXISTS uq_platform_skills_config_name
    ON platform_skills (sandbox_config_id, name) WHERE deleted_at IS NULL;
CREATE INDEX IF NOT EXISTS idx_platform_skills_catalog
    ON platform_skills (catalog_id);
CREATE INDEX IF NOT EXISTS idx_platform_skills_deleted_at
    ON platform_skills (deleted_at);
DROP INDEX IF EXISTS idx_tenant_skill_snapshots_config;
DROP INDEX IF EXISTS idx_tenant_skill_snapshots_state;
ALTER TABLE tenant_skill_snapshots RENAME TO platform_skill_snapshots;
ALTER TABLE platform_skill_snapshots DROP COLUMN tenant_id;

CREATE INDEX IF NOT EXISTS idx_platform_skill_snapshots_config
    ON platform_skill_snapshots (sandbox_config_id);
CREATE INDEX IF NOT EXISTS idx_platform_skill_snapshots_skill
    ON platform_skill_snapshots (skill_id);
CREATE INDEX IF NOT EXISTS idx_platform_skill_snapshots_state
    ON platform_skill_snapshots (state);
