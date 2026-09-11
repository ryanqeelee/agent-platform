-- Platform administrators are global identities. Remove legacy enterprise
-- bindings before enforcing the row invariant for future writes.
UPDATE tenant_members
SET deleted_at = COALESCE(deleted_at, CURRENT_TIMESTAMP),
    updated_at = CURRENT_TIMESTAMP
WHERE deleted_at IS NULL
  AND user_id IN (SELECT id FROM users WHERE is_system_admin = TRUE);

UPDATE users
SET tenant_id = NULL,
    can_access_all_tenants = FALSE,
    updated_at = CURRENT_TIMESTAMP
WHERE is_system_admin = TRUE;

ALTER TABLE users
    ADD CONSTRAINT users_system_admin_tenantless
    CHECK (NOT is_system_admin OR (tenant_id IS NULL AND can_access_all_tenants = FALSE));
