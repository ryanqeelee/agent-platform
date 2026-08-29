-- Migration: 000084_ringxun_enterprise_activation_receipt
-- Private, tenant-local receipt for the Ringxun enterprise activation adapter.

ALTER TABLE tenants
    ADD COLUMN IF NOT EXISTS ringxun_activation_id VARCHAR(128),
    ADD COLUMN IF NOT EXISTS ringxun_activation_request_sha256 VARCHAR(64),
    ADD COLUMN IF NOT EXISTS ringxun_initial_owner_user_id VARCHAR(36);

-- Deliberately not filtered by deleted_at: abandon/soft-delete must never let
-- another tenant claim the same external activation receipt.
CREATE UNIQUE INDEX IF NOT EXISTS idx_tenants_ringxun_activation_id_unique
    ON tenants(ringxun_activation_id)
    WHERE ringxun_activation_id IS NOT NULL;
