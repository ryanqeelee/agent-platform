-- Never resurrect historical Owner invitations during rollback.
DROP INDEX IF EXISTS idx_tenant_members_one_active_owner;
