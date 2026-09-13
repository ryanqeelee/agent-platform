-- Promote MCP connection definitions to platform ownership. Service IDs and
-- all OAuth/tool-policy foreign keys remain unchanged.

DROP INDEX IF EXISTS idx_mcp_services_tenant_id;
DROP INDEX IF EXISTS idx_tenant_name;

ALTER TABLE mcp_services DROP COLUMN tenant_id;

DO $$
BEGIN
    IF EXISTS (
        SELECT 1
        FROM mcp_tool_approvals
        GROUP BY service_id, tool_name
        HAVING COUNT(*) > 1
    ) THEN
        RAISE EXCEPTION 'cannot promote MCP tool approvals: duplicate service/tool policies require an explicit operator decision';
    END IF;
END $$;

DROP INDEX IF EXISTS idx_mcp_tool_approvals_tenant_svc_tool;
ALTER TABLE mcp_tool_approvals DROP COLUMN tenant_id;
CREATE UNIQUE INDEX idx_mcp_tool_approvals_svc_tool
    ON mcp_tool_approvals(service_id, tool_name);
