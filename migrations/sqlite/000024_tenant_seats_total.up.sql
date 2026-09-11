ALTER TABLE tenants ADD COLUMN seats_total INTEGER CHECK (seats_total IS NULL OR seats_total > 0);
