ALTER TABLE tenants DROP CONSTRAINT IF EXISTS tenants_seats_total_positive;
ALTER TABLE tenants DROP COLUMN IF EXISTS seats_total;
