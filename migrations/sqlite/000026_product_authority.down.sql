DROP INDEX IF EXISTS idx_tenants_ringxun_activation_idempotency_unique;
DROP INDEX IF EXISTS idx_tenants_governed_enterprise_id_unique;

ALTER TABLE tenants DROP COLUMN ringxun_activation_last_error_code;
ALTER TABLE tenants DROP COLUMN ringxun_activation_completed_at;
ALTER TABLE tenants DROP COLUMN ringxun_activation_plan_version_id;
ALTER TABLE tenants DROP COLUMN ringxun_activation_idempotency_key_sha256;
ALTER TABLE tenants DROP COLUMN analysis_enabled;
ALTER TABLE tenants DROP COLUMN governed_enterprise_id;
