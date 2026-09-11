-- SQLite cannot add the equivalent users row CHECK without rebuilding the
-- table. Normalize legacy data; repositories enforce the invariant on writes.
UPDATE tenant_members
SET deleted_at = COALESCE(deleted_at, CURRENT_TIMESTAMP),
    updated_at = CURRENT_TIMESTAMP
WHERE deleted_at IS NULL
  AND user_id IN (SELECT id FROM users WHERE is_system_admin = 1);

UPDATE users
SET tenant_id = NULL,
    can_access_all_tenants = 0,
    updated_at = CURRENT_TIMESTAMP
WHERE is_system_admin = 1;
