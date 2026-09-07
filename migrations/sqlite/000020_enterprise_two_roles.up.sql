-- SQLite rebuilds the table to change its default. Preserve the child grants
-- even when the migration connection enables ON DELETE CASCADE.
CREATE TEMP TABLE retained_business_role_members AS SELECT * FROM business_role_members;
DELETE FROM business_role_members;
CREATE TABLE tenant_members_two_roles (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    user_id VARCHAR(36) NOT NULL,
    tenant_id INTEGER NOT NULL,
    role VARCHAR(20) NOT NULL DEFAULT 'viewer',
    status VARCHAR(20) NOT NULL DEFAULT 'active',
    invited_by VARCHAR(36),
    joined_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
    created_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
    deleted_at DATETIME,
    operating_analysis_access BOOLEAN NOT NULL DEFAULT FALSE
);
INSERT INTO tenant_members_two_roles
SELECT id,user_id,tenant_id,
       CASE role WHEN 'owner' THEN 'admin' WHEN 'contributor' THEN 'viewer' ELSE role END,
       status,invited_by,joined_at,created_at,updated_at,deleted_at,operating_analysis_access
FROM tenant_members;
DROP TABLE tenant_members;
ALTER TABLE tenant_members_two_roles RENAME TO tenant_members;
CREATE UNIQUE INDEX idx_tenant_members_user_tenant_unique ON tenant_members(user_id,tenant_id) WHERE deleted_at IS NULL;
CREATE INDEX idx_tenant_members_tenant_role ON tenant_members(tenant_id,role);
CREATE INDEX idx_tenant_members_user ON tenant_members(user_id);
CREATE UNIQUE INDEX idx_tenant_members_tenant_id_for_business_roles ON tenant_members(tenant_id,id);
INSERT INTO business_role_members SELECT * FROM retained_business_role_members;
DROP TABLE retained_business_role_members;
UPDATE tenant_invitations SET role = 'admin' WHERE role = 'owner';
UPDATE tenant_invitations SET role = 'viewer' WHERE role = 'contributor';
