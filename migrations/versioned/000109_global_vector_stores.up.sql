-- Promote every stored connection to platform scope without changing its ID
-- or any knowledge-base reference. No row is deduplicated.
DROP INDEX IF EXISTS idx_vector_stores_name_tenant;
DROP INDEX IF EXISTS idx_vector_stores_tenant_id;

ALTER TABLE vector_stores ADD COLUMN IF NOT EXISTS is_default BOOLEAN NOT NULL DEFAULT FALSE;

WITH duplicates AS (
    SELECT name
      FROM vector_stores
     WHERE deleted_at IS NULL
     GROUP BY name
    HAVING COUNT(*) > 1
)
UPDATE vector_stores AS v
   SET name = LEFT(v.name, 216) || ' [' || v.id || ']'
 WHERE v.deleted_at IS NULL
   AND v.name IN (SELECT name FROM duplicates);

ALTER TABLE vector_stores DROP COLUMN IF EXISTS tenant_id;

CREATE UNIQUE INDEX idx_vector_stores_name_global
    ON vector_stores(name) WHERE deleted_at IS NULL;
CREATE UNIQUE INDEX idx_vector_stores_single_default
    ON vector_stores(is_default) WHERE is_default = TRUE AND deleted_at IS NULL;
CREATE INDEX IF NOT EXISTS idx_vector_stores_engine_type ON vector_stores(engine_type);
CREATE INDEX IF NOT EXISTS idx_vector_stores_deleted_at ON vector_stores(deleted_at);

COMMENT ON COLUMN vector_stores.is_default IS
    'Sole platform fallback for knowledge bases without an explicit vector_store_id. Initially unset.';
