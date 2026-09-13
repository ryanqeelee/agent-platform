ALTER TABLE tenants ADD COLUMN governed_enterprise_id VARCHAR(128);
ALTER TABLE tenants ADD COLUMN analysis_enabled BOOLEAN NOT NULL DEFAULT 0;
ALTER TABLE tenants ADD COLUMN ringxun_activation_idempotency_key_sha256 VARCHAR(64);
ALTER TABLE tenants ADD COLUMN ringxun_activation_plan_version_id TEXT REFERENCES ai_capability_plan_versions(version_id) ON DELETE RESTRICT;
ALTER TABLE tenants ADD COLUMN ringxun_activation_completed_at DATETIME;
ALTER TABLE tenants ADD COLUMN ringxun_activation_last_error_code VARCHAR(64);

CREATE UNIQUE INDEX idx_tenants_governed_enterprise_id_unique
    ON tenants(governed_enterprise_id)
    WHERE governed_enterprise_id IS NOT NULL;
CREATE UNIQUE INDEX idx_tenants_ringxun_activation_idempotency_unique
    ON tenants(ringxun_activation_idempotency_key_sha256)
    WHERE ringxun_activation_idempotency_key_sha256 IS NOT NULL;
