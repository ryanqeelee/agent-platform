-- Enterprise roles are admin and viewer. Preserve membership status and
-- business-role grants; retiring contributor must not grant administration.
DROP INDEX IF EXISTS idx_tenant_members_one_active_owner;
UPDATE tenant_members SET role = 'admin' WHERE role = 'owner';
UPDATE tenant_members SET role = 'viewer' WHERE role = 'contributor';
ALTER TABLE tenant_members ALTER COLUMN role SET DEFAULT 'viewer';
-- Old owner invitations were already revoked by migration 81. Never revive them.
UPDATE tenant_invitations SET role = 'admin' WHERE role = 'owner';
UPDATE tenant_invitations SET role = 'viewer' WHERE role = 'contributor';
