-- Older SQLite installs used an unconditional membership index, which made a
-- soft-deleted member impossible to rejoin. Replace it before the business
-- role foreign keys begin to reference membership IDs.
DROP INDEX IF EXISTS idx_tenant_members_user_tenant_unique;
CREATE UNIQUE INDEX IF NOT EXISTS idx_tenant_members_user_tenant_unique
    ON tenant_members(user_id, tenant_id) WHERE deleted_at IS NULL;

CREATE TABLE IF NOT EXISTS business_roles (
    id VARCHAR(36) PRIMARY KEY,
    tenant_id INTEGER NOT NULL REFERENCES tenants(id) ON DELETE CASCADE,
    name VARCHAR(128) NOT NULL,
    enabled BOOLEAN NOT NULL DEFAULT 1,
    created_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
    deleted_at DATETIME
);
CREATE UNIQUE INDEX IF NOT EXISTS idx_business_roles_tenant_id
    ON business_roles (tenant_id, id);
CREATE UNIQUE INDEX IF NOT EXISTS idx_tenant_members_tenant_id_for_business_roles
    ON tenant_members (tenant_id, id);
CREATE UNIQUE INDEX IF NOT EXISTS idx_business_roles_tenant_name_active
    ON business_roles (tenant_id, name) WHERE deleted_at IS NULL;
CREATE INDEX IF NOT EXISTS idx_business_roles_tenant_enabled
    ON business_roles (tenant_id, enabled) WHERE deleted_at IS NULL;

CREATE TABLE IF NOT EXISTS business_role_members (
    tenant_id INTEGER NOT NULL REFERENCES tenants(id) ON DELETE CASCADE,
    role_id VARCHAR(36) NOT NULL,
    tenant_member_id INTEGER NOT NULL,
    created_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
    PRIMARY KEY (tenant_id, role_id, tenant_member_id),
    FOREIGN KEY (tenant_id, role_id) REFERENCES business_roles(tenant_id, id) ON DELETE CASCADE,
    FOREIGN KEY (tenant_id, tenant_member_id) REFERENCES tenant_members(tenant_id, id) ON DELETE CASCADE
);
CREATE INDEX IF NOT EXISTS idx_business_role_members_tenant_user
    ON business_role_members (tenant_id, tenant_member_id, role_id);

CREATE UNIQUE INDEX IF NOT EXISTS idx_knowledge_bases_tenant_id_for_access_grants
    ON knowledge_bases (tenant_id, id);
CREATE TABLE IF NOT EXISTS knowledge_base_role_grants (
    tenant_id INTEGER NOT NULL REFERENCES tenants(id) ON DELETE CASCADE,
    knowledge_base_id VARCHAR(36) NOT NULL,
    role_id VARCHAR(36) NOT NULL,
    created_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
    PRIMARY KEY (tenant_id, knowledge_base_id, role_id),
    FOREIGN KEY (tenant_id, role_id) REFERENCES business_roles(tenant_id, id) ON DELETE CASCADE,
    FOREIGN KEY (tenant_id, knowledge_base_id) REFERENCES knowledge_bases(tenant_id, id) ON DELETE CASCADE
);
CREATE INDEX IF NOT EXISTS idx_kb_role_grants_tenant_role
    ON knowledge_base_role_grants (tenant_id, role_id, knowledge_base_id);
