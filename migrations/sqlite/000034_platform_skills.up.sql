CREATE TABLE IF NOT EXISTS platform_skill_catalog (
    id VARCHAR(36) PRIMARY KEY,
    name VARCHAR(255) NOT NULL,
    version VARCHAR(64),
    description TEXT,
    instructions TEXT,
    bundle_ref VARCHAR(1024),
    bundle_sha256 VARCHAR(64),
    created_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
    deleted_at DATETIME
);

CREATE INDEX IF NOT EXISTS idx_platform_skill_catalog_name
    ON platform_skill_catalog(name) WHERE deleted_at IS NULL;
CREATE INDEX IF NOT EXISTS idx_platform_skill_catalog_deleted_at
    ON platform_skill_catalog(deleted_at);

CREATE TABLE IF NOT EXISTS platform_skills (
    id VARCHAR(36) PRIMARY KEY,
    sandbox_config_id VARCHAR(36) NOT NULL,
    catalog_id VARCHAR(36),
    name VARCHAR(255) NOT NULL,
    version VARCHAR(64),
    description TEXT,
    instructions TEXT,
    bundle_ref VARCHAR(1024),
    bundle_sha256 VARCHAR(64),
    enabled BOOLEAN NOT NULL DEFAULT 1,
    installed_snapshot_id VARCHAR(255),
    install_run_id VARCHAR(36) NOT NULL DEFAULT '',
    install_transcript TEXT,
    envs TEXT,
    status VARCHAR(32) NOT NULL,
    error TEXT,
    installing_since DATETIME,
    created_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
    deleted_at DATETIME
);

CREATE UNIQUE INDEX IF NOT EXISTS uq_platform_skills_config_name
    ON platform_skills(sandbox_config_id, name) WHERE deleted_at IS NULL;
CREATE INDEX IF NOT EXISTS idx_platform_skills_catalog
    ON platform_skills(catalog_id);
CREATE INDEX IF NOT EXISTS idx_platform_skills_deleted_at
    ON platform_skills(deleted_at);

CREATE TABLE IF NOT EXISTS platform_skill_snapshots (
    id VARCHAR(36) PRIMARY KEY,
    sandbox_config_id VARCHAR(36) NOT NULL,
    skill_id VARCHAR(36),
    snapshot_id VARCHAR(255),
    parent_snapshot_id VARCHAR(255),
    generation INTEGER NOT NULL DEFAULT 0,
    planned_name VARCHAR(255),
    trigger VARCHAR(16) NOT NULL,
    state VARCHAR(16) NOT NULL,
    superseded_at DATETIME,
    created_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP
);

CREATE INDEX IF NOT EXISTS idx_platform_skill_snapshots_config
    ON platform_skill_snapshots(sandbox_config_id);
CREATE INDEX IF NOT EXISTS idx_platform_skill_snapshots_skill
    ON platform_skill_snapshots(skill_id);
CREATE INDEX IF NOT EXISTS idx_platform_skill_snapshots_state
    ON platform_skill_snapshots(state);

CREATE TABLE IF NOT EXISTS tenant_user_env_vars (
    id VARCHAR(36) PRIMARY KEY,
    tenant_id INTEGER NOT NULL,
    principal_type VARCHAR(32) NOT NULL,
    principal_id VARCHAR(512) NOT NULL,
    sandbox_config_id VARCHAR(36) NOT NULL,
    skill_id VARCHAR(36) NOT NULL DEFAULT '',
    name VARCHAR(255) NOT NULL,
    value TEXT,
    created_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP
);

CREATE UNIQUE INDEX IF NOT EXISTS uq_user_env_var
    ON tenant_user_env_vars(
        tenant_id, principal_type, principal_id, sandbox_config_id, skill_id, name
    );
CREATE INDEX IF NOT EXISTS idx_user_env_var_skill
    ON tenant_user_env_vars(tenant_id, skill_id);
CREATE INDEX IF NOT EXISTS idx_user_env_var_config
    ON tenant_user_env_vars(tenant_id, sandbox_config_id);
