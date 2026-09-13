ALTER TABLE tenants DROP CONSTRAINT IF EXISTS tenants_ringxun_activation_plan_version_fk;
DROP INDEX IF EXISTS idx_tenants_ringxun_activation_idempotency_unique;
DROP INDEX IF EXISTS idx_tenants_governed_enterprise_id_unique;

ALTER TABLE tenants
    DROP COLUMN IF EXISTS ringxun_activation_last_error_code,
    DROP COLUMN IF EXISTS ringxun_activation_completed_at,
    DROP COLUMN IF EXISTS ringxun_activation_plan_version_id,
    DROP COLUMN IF EXISTS ringxun_activation_idempotency_key_sha256,
    DROP COLUMN IF EXISTS analysis_enabled,
    DROP COLUMN IF EXISTS governed_enterprise_id;
