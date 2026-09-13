DROP INDEX IF EXISTS idx_storage_backends_name_tenant;
DROP INDEX IF EXISTS idx_storage_backends_legacy_alias;
DROP INDEX IF EXISTS idx_storage_backends_tenant;

ALTER TABLE storage_backends ADD COLUMN is_default INTEGER NOT NULL DEFAULT 0;

UPDATE knowledge_bases
   SET storage_backend_id = (
       SELECT t.default_storage_backend_id FROM tenants AS t
        WHERE t.id = knowledge_bases.tenant_id
          AND t.default_storage_backend_id IS NOT NULL
          AND EXISTS (
              SELECT 1 FROM storage_backends AS b
               WHERE b.id = t.default_storage_backend_id
                 AND b.tenant_id = t.id
                 AND b.deleted_at IS NULL
          )
   )
 WHERE storage_backend_id IS NULL
   AND EXISTS (
       SELECT 1 FROM tenants AS t
        WHERE t.id = knowledge_bases.tenant_id
          AND t.default_storage_backend_id IS NOT NULL
          AND EXISTS (
              SELECT 1 FROM storage_backends AS b
               WHERE b.id = t.default_storage_backend_id
                 AND b.tenant_id = t.id
                 AND b.deleted_at IS NULL
          )
   );

UPDATE storage_backends
   SET name = substr(name, 1, 216) || ' [' || id || ']'
 WHERE deleted_at IS NULL
   AND name IN (
       SELECT name FROM storage_backends
        WHERE deleted_at IS NULL
        GROUP BY name HAVING COUNT(*) > 1
   );

ALTER TABLE storage_backends DROP COLUMN tenant_id;
ALTER TABLE storage_backends DROP COLUMN legacy_alias;
ALTER TABLE tenants DROP COLUMN default_storage_backend_id;
ALTER TABLE tenants DROP COLUMN storage_engine_config;

CREATE UNIQUE INDEX idx_storage_backends_name_global
    ON storage_backends(name) WHERE deleted_at IS NULL;
CREATE UNIQUE INDEX idx_storage_backends_single_default
    ON storage_backends(is_default) WHERE is_default = 1 AND deleted_at IS NULL;
CREATE INDEX IF NOT EXISTS idx_storage_backends_provider ON storage_backends(provider);
CREATE INDEX IF NOT EXISTS idx_storage_backends_deleted_at ON storage_backends(deleted_at);
