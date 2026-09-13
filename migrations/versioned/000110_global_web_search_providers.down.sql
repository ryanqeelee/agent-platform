-- Global provider ownership cannot be mapped back to arbitrary tenants
-- without inventing ownership. Refuse the unsafe downgrade explicitly.
DO $$
BEGIN
    RAISE EXCEPTION '000110 is not reversible: platform web-search providers have no tenant owner';
END $$;
