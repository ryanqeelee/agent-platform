ALTER TABLE users DROP CONSTRAINT IF EXISTS users_system_admin_tenantless;
DROP TABLE IF EXISTS platform_initial_administrator_receipts;
ALTER TABLE tenants DROP CONSTRAINT IF EXISTS tenants_seats_total_positive;
ALTER TABLE tenants DROP COLUMN IF EXISTS seats_total;
