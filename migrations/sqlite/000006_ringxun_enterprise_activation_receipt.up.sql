ALTER TABLE tenants ADD COLUMN ringxun_activation_id VARCHAR(128);
ALTER TABLE tenants ADD COLUMN ringxun_activation_request_sha256 VARCHAR(64);
ALTER TABLE tenants ADD COLUMN ringxun_initial_owner_user_id VARCHAR(36);

-- No deleted_at predicate: a soft-deleted receipt remains globally reserved.
CREATE UNIQUE INDEX IF NOT EXISTS idx_tenants_ringxun_activation_id_unique
    ON tenants(ringxun_activation_id)
    WHERE ringxun_activation_id IS NOT NULL;
