-- Memberships removed by the up migration cannot be reconstructed safely.
ALTER TABLE users DROP CONSTRAINT IF EXISTS users_system_admin_tenantless;
