DROP TABLE IF EXISTS memory_expressions;
DROP TABLE IF EXISTS memory_command_receipts;
DROP INDEX IF EXISTS idx_memory_items_projection;
ALTER TABLE memory_items DROP COLUMN IF EXISTS scope;
ALTER TABLE memory_subjects DROP COLUMN IF EXISTS revision;
ALTER TABLE memory_subjects DROP COLUMN IF EXISTS generation;
ALTER TABLE tenants DROP COLUMN IF EXISTS memory_generation;

-- Retain the compatible source-ID widening on rollback: shrinking to 36 could
-- destroy accepted non-UUID Runtime/migration provenance.
