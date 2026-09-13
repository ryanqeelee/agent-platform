DROP INDEX IF EXISTS idx_vector_stores_name_tenant;
DROP INDEX IF EXISTS idx_vector_stores_tenant_id;

ALTER TABLE vector_stores ADD COLUMN is_default INTEGER NOT NULL DEFAULT 0;

UPDATE vector_stores
   SET name = substr(name, 1, 216) || ' [' || id || ']'
 WHERE deleted_at IS NULL
   AND name IN (
       SELECT name FROM vector_stores
        WHERE deleted_at IS NULL
        GROUP BY name HAVING COUNT(*) > 1
   );

ALTER TABLE vector_stores DROP COLUMN tenant_id;

CREATE UNIQUE INDEX idx_vector_stores_name_global
    ON vector_stores(name) WHERE deleted_at IS NULL;
CREATE UNIQUE INDEX idx_vector_stores_single_default
    ON vector_stores(is_default) WHERE is_default = 1 AND deleted_at IS NULL;
CREATE INDEX IF NOT EXISTS idx_vector_stores_engine_type ON vector_stores(engine_type);
CREATE INDEX IF NOT EXISTS idx_vector_stores_deleted_at ON vector_stores(deleted_at);
