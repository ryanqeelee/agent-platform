ALTER TABLE tenants
    ADD COLUMN IF NOT EXISTS governed_enterprise_id VARCHAR(128),
    ADD COLUMN IF NOT EXISTS analysis_enabled BOOLEAN NOT NULL DEFAULT FALSE,
    ADD COLUMN IF NOT EXISTS ringxun_activation_idempotency_key_sha256 VARCHAR(64),
    ADD COLUMN IF NOT EXISTS ringxun_activation_plan_version_id TEXT,
    ADD COLUMN IF NOT EXISTS ringxun_activation_completed_at TIMESTAMP WITH TIME ZONE,
    ADD COLUMN IF NOT EXISTS ringxun_activation_last_error_code VARCHAR(64);

CREATE UNIQUE INDEX IF NOT EXISTS idx_tenants_governed_enterprise_id_unique
    ON tenants(governed_enterprise_id)
    WHERE governed_enterprise_id IS NOT NULL;
CREATE UNIQUE INDEX IF NOT EXISTS idx_tenants_ringxun_activation_idempotency_unique
    ON tenants(ringxun_activation_idempotency_key_sha256)
    WHERE ringxun_activation_idempotency_key_sha256 IS NOT NULL;

DO $$
BEGIN
    IF NOT EXISTS (
        SELECT 1 FROM pg_constraint
        WHERE conname = 'tenants_ringxun_activation_plan_version_fk'
    ) THEN
        ALTER TABLE tenants
            ADD CONSTRAINT tenants_ringxun_activation_plan_version_fk
            FOREIGN KEY (ringxun_activation_plan_version_id)
            REFERENCES ai_capability_plan_versions(version_id)
            ON DELETE RESTRICT;
    END IF;
END $$;
