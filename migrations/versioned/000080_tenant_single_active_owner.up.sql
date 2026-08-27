-- A tenant has exactly one active Owner once ownership lifecycle is enabled.
-- Existing historical multi-owner data is deliberately not repaired here:
-- selecting one automatically would silently change authority. Operators must
-- resolve it explicitly before re-running this migration.
DO $$
DECLARE
    conflicted_tenants BIGINT;
BEGIN
    SELECT COUNT(*) INTO conflicted_tenants
      FROM (
          SELECT tenant_id
            FROM tenant_members
           WHERE role = 'owner'
             AND status = 'active'
             AND deleted_at IS NULL
           GROUP BY tenant_id
          HAVING COUNT(*) > 1
      ) conflicts;
    IF conflicted_tenants > 0 THEN
        RAISE EXCEPTION
            '[Migration 000080] % tenant(s) have multiple active owners. Resolve them explicitly before retrying; no owner was selected automatically.',
            conflicted_tenants;
    END IF;
END $$;

CREATE UNIQUE INDEX IF NOT EXISTS idx_tenant_members_one_active_owner
    ON tenant_members (tenant_id)
    WHERE role = 'owner'
      AND status = 'active'
      AND deleted_at IS NULL;
