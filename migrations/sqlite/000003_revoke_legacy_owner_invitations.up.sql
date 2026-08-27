-- Existing Lite databases must receive the owner invariant too; 000000_init
-- only covers fresh files. Creating this first makes historical multi-owner
-- state fail closed instead of choosing an authority silently.
CREATE UNIQUE INDEX IF NOT EXISTS idx_tenant_members_one_active_owner
    ON tenant_members(tenant_id)
    WHERE role = 'owner' AND status = 'active' AND deleted_at IS NULL;

-- SQLite incremental equivalent of versioned/000081. Existing Lite databases
-- must be migrated; changing only 000000_init would miss deployed files.
UPDATE tenant_invitations
SET status = 'revoked',
    responded_at = CURRENT_TIMESTAMP,
    updated_at = CURRENT_TIMESTAMP
WHERE status = 'pending'
  AND role = 'owner'
  AND deleted_at IS NULL;
