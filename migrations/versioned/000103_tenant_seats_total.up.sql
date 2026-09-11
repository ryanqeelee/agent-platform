ALTER TABLE tenants ADD COLUMN IF NOT EXISTS seats_total INTEGER;

DO $$
BEGIN
    IF NOT EXISTS (
        SELECT 1 FROM pg_constraint WHERE conname = 'tenants_seats_total_positive'
    ) THEN
        ALTER TABLE tenants
            ADD CONSTRAINT tenants_seats_total_positive
            CHECK (seats_total IS NULL OR seats_total > 0);
    END IF;
END $$;
