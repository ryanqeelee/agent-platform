-- Promote web-search provider definitions to platform ownership. IDs stay
-- unchanged so every agent/session pin continues to reference the same row.
-- Existing per-tenant defaults are intentionally not collapsed into a global
-- default: deployments with more than one prior default require an explicit
-- operator choice through the platform provider API.

DROP INDEX IF EXISTS idx_web_search_providers_tenant_id;

ALTER TABLE web_search_providers
    DROP COLUMN tenant_id;

-- Provider selection, credentials, and proxy settings are no longer tenant
-- state. Per-agent request limits already live on custom_agents.config.
ALTER TABLE tenants
    DROP COLUMN web_search_config;

UPDATE web_search_providers SET is_default = FALSE;

CREATE UNIQUE INDEX idx_web_search_providers_single_default
    ON web_search_providers (is_default)
    WHERE is_default = TRUE AND deleted_at IS NULL;
