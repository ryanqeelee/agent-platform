DROP INDEX IF EXISTS idx_tenants_ringxun_activation_id_unique;

ALTER TABLE tenants
    DROP COLUMN IF EXISTS ringxun_initial_owner_user_id,
    DROP COLUMN IF EXISTS ringxun_activation_request_sha256,
    DROP COLUMN IF EXISTS ringxun_activation_id;
