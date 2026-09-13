-- Global MCP definitions cannot be mapped back to arbitrary tenants without
-- fabricating ownership. Refuse the unsafe downgrade explicitly.
DO $$
BEGIN
    RAISE EXCEPTION '000111 is not reversible: platform MCP services have no tenant owner';
END $$;
