ALTER TABLE tenants ADD COLUMN IF NOT EXISTS seats_total INTEGER;

DO $$
BEGIN
    IF NOT EXISTS (
        SELECT 1 FROM pg_constraint WHERE conname = 'tenants_seats_total_positive'
    ) THEN
        ALTER TABLE tenants
            ADD CONSTRAINT tenants_seats_total_positive
            CHECK (seats_total IS NULL OR seats_total > 0);
    END IF;
END $$;

CREATE TABLE IF NOT EXISTS platform_initial_administrator_receipts (
    command_id VARCHAR(128) PRIMARY KEY,
    request_sha256 VARCHAR(64) NOT NULL,
    user_id VARCHAR(36) NOT NULL UNIQUE REFERENCES users(id) ON DELETE RESTRICT,
    username VARCHAR(100) NOT NULL,
    email VARCHAR(255) NOT NULL,
    status VARCHAR(32) NOT NULL CHECK (status = 'created'),
    created_at TIMESTAMP WITH TIME ZONE NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at TIMESTAMP WITH TIME ZONE NOT NULL DEFAULT CURRENT_TIMESTAMP
);

-- Platform administrators are global identities. Remove development-era
-- enterprise bindings before enforcing the invariant for future writes.
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
