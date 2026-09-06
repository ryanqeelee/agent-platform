-- Migration: 000098_default_keenable_web_search
-- Description: Provision keyless Keenable for tenants with no provider history.
-- Soft-deleted rows count as history so an administrator's removal is preserved.

INSERT INTO web_search_providers (
    id,
    tenant_id,
    name,
    provider,
    description,
    parameters,
    is_default,
    created_at,
    updated_at
)
SELECT
    gen_random_uuid()::varchar(36),
    tenants.id,
    'Keenable',
    'keenable',
    'Platform-provisioned keyless web search',
    '{}'::jsonb,
    TRUE,
    CURRENT_TIMESTAMP,
    CURRENT_TIMESTAMP
FROM tenants
WHERE tenants.deleted_at IS NULL
  AND NOT EXISTS (
      SELECT 1
      FROM web_search_providers
      WHERE web_search_providers.tenant_id = tenants.id
  );
