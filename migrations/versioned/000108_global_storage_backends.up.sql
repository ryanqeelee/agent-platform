-- Promote every stored connection to platform scope without changing its ID
-- or any knowledge/resource reference. No row is deduplicated.
DROP INDEX IF EXISTS idx_storage_backends_name_tenant;
DROP INDEX IF EXISTS idx_storage_backends_legacy_alias;
DROP INDEX IF EXISTS idx_storage_backends_tenant;

ALTER TABLE storage_backends ADD COLUMN IF NOT EXISTS is_default BOOLEAN NOT NULL DEFAULT FALSE;

-- Preserve an unequivocal old tenant default as an explicit KB binding before
-- the ownership/default columns disappear. Missing or invalid defaults stay
-- unbound and therefore fail closed until an operator inventories them.
UPDATE knowledge_bases AS kb
   SET storage_backend_id = t.default_storage_backend_id
  FROM tenants AS t
 WHERE kb.tenant_id = t.id
   AND kb.storage_backend_id IS NULL
   AND t.default_storage_backend_id IS NOT NULL
   AND EXISTS (
       SELECT 1 FROM storage_backends AS b
        WHERE b.id = t.default_storage_backend_id
          AND b.tenant_id = t.id
          AND b.deleted_at IS NULL
   );

-- A global display-name constraint needs deterministic disambiguation for
-- formerly tenant-local duplicates. The complete ID makes the result stable.
WITH duplicates AS (
    SELECT name
      FROM storage_backends
     WHERE deleted_at IS NULL
     GROUP BY name
    HAVING COUNT(*) > 1
)
UPDATE storage_backends AS b
   SET name = LEFT(b.name, 216) || ' [' || b.id || ']'
 WHERE b.deleted_at IS NULL
   AND b.name IN (SELECT name FROM duplicates);

ALTER TABLE storage_backends DROP COLUMN IF EXISTS tenant_id;
ALTER TABLE storage_backends DROP COLUMN IF EXISTS legacy_alias;
ALTER TABLE tenants DROP COLUMN IF EXISTS default_storage_backend_id;
ALTER TABLE tenants DROP COLUMN IF EXISTS storage_engine_config;

CREATE UNIQUE INDEX idx_storage_backends_name_global
    ON storage_backends(name) WHERE deleted_at IS NULL;
CREATE UNIQUE INDEX idx_storage_backends_single_default
    ON storage_backends(is_default) WHERE is_default = TRUE AND deleted_at IS NULL;
CREATE INDEX IF NOT EXISTS idx_storage_backends_provider ON storage_backends(provider);
CREATE INDEX IF NOT EXISTS idx_storage_backends_deleted_at ON storage_backends(deleted_at);

COMMENT ON COLUMN storage_backends.is_default IS
    'Sole platform fallback for operations without an explicit storage_backend_id. Initially unset.';
