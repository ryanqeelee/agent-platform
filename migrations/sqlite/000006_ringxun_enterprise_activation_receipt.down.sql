DROP INDEX IF EXISTS idx_tenants_ringxun_activation_id_unique;

ALTER TABLE tenants DROP COLUMN ringxun_initial_owner_user_id;
ALTER TABLE tenants DROP COLUMN ringxun_activation_request_sha256;
ALTER TABLE tenants DROP COLUMN ringxun_activation_id;
