CREATE TABLE IF NOT EXISTS operating_brief_refreshes (
    id UUID NOT NULL UNIQUE,
    tenant_id BIGINT NOT NULL REFERENCES tenants(id) ON DELETE CASCADE,
    requester_user_id TEXT NOT NULL,
    scope_kind VARCHAR(32) NOT NULL CHECK (scope_kind IN ('all_authorized', 'store')),
    store_id TEXT NOT NULL DEFAULT '',
    status VARCHAR(16) NOT NULL CHECK (status IN ('queued', 'running', 'succeeded', 'failed')),
    error_code VARCHAR(64) NOT NULL DEFAULT '',
    created_at TIMESTAMP WITH TIME ZONE NOT NULL,
	updated_at TIMESTAMP WITH TIME ZONE NOT NULL,
	PRIMARY KEY (tenant_id, scope_kind, store_id),
    CHECK ((scope_kind = 'all_authorized' AND store_id = '') OR (scope_kind = 'store' AND store_id <> ''))
);
CREATE TABLE IF NOT EXISTS operating_brief_snapshots (
	snapshot_ref UUID NOT NULL UNIQUE,
    tenant_id BIGINT NOT NULL REFERENCES tenants(id) ON DELETE CASCADE,
    scope_kind VARCHAR(32) NOT NULL CHECK (scope_kind IN ('all_authorized', 'store')),
    store_id TEXT NOT NULL DEFAULT '',
    source_id TEXT NOT NULL,
    binding_id TEXT NOT NULL,
    binding_revision BIGINT NOT NULL,
    deployment_revision BIGINT NOT NULL,
    catalog_version TEXT NOT NULL,
    freshness_token TEXT NOT NULL,
    state VARCHAR(16) NOT NULL CHECK (state IN ('ready', 'no_data')),
    quality VARCHAR(16) NOT NULL DEFAULT '',
    reason_code VARCHAR(64) NOT NULL DEFAULT '',
    input_set_digest TEXT NOT NULL DEFAULT '',
    revision BIGINT,
    ready_at TEXT NOT NULL DEFAULT '',
    data_json JSONB NOT NULL,
    handoff_questions_json JSONB NOT NULL,
    snapshot_metadata_json JSONB NOT NULL,
    fixed_slots_json JSONB NOT NULL,
	generated_at TIMESTAMP WITH TIME ZONE NOT NULL,
	PRIMARY KEY (tenant_id, scope_kind, store_id),
    CHECK ((scope_kind = 'all_authorized' AND store_id = '') OR (scope_kind = 'store' AND store_id <> ''))
);
CREATE TABLE IF NOT EXISTS operating_brief_scope_refs (
    scope_ref UUID PRIMARY KEY,
    tenant_id BIGINT NOT NULL REFERENCES tenants(id) ON DELETE CASCADE,
    store_id TEXT NOT NULL,
    label TEXT NOT NULL,
    updated_at TIMESTAMP WITH TIME ZONE NOT NULL,
    UNIQUE (tenant_id, store_id)
);
CREATE INDEX IF NOT EXISTS idx_operating_brief_scope_ref_tenant
    ON operating_brief_scope_refs(tenant_id, scope_ref);
