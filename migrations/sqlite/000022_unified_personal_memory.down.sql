DROP TABLE IF EXISTS memory_expressions;
DROP TABLE IF EXISTS memory_command_receipts;
DROP INDEX IF EXISTS idx_memory_items_projection;
ALTER TABLE memory_items DROP COLUMN scope;
ALTER TABLE memory_subjects DROP COLUMN revision;
ALTER TABLE memory_subjects DROP COLUMN generation;
ALTER TABLE tenants DROP COLUMN memory_generation;
