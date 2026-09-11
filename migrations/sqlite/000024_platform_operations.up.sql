ALTER TABLE tenants ADD COLUMN seats_total INTEGER CHECK (seats_total IS NULL OR seats_total > 0);

CREATE TABLE IF NOT EXISTS platform_initial_administrator_receipts (
    command_id VARCHAR(128) PRIMARY KEY,
    request_sha256 VARCHAR(64) NOT NULL,
    user_id VARCHAR(36) NOT NULL UNIQUE REFERENCES users(id) ON DELETE RESTRICT,
    username VARCHAR(100) NOT NULL,
    email VARCHAR(255) NOT NULL,
    status VARCHAR(32) NOT NULL CHECK (status = 'created'),
    created_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP
);

-- SQLite cannot add the equivalent users row CHECK without rebuilding the
-- table. Normalize development-era data; repositories enforce future writes.
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
