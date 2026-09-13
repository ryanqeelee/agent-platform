CREATE TABLE operating_brief_refreshes (
    id TEXT NOT NULL UNIQUE,
    tenant_id INTEGER NOT NULL REFERENCES tenants(id) ON DELETE CASCADE,
    requester_user_id TEXT NOT NULL,
    scope_kind TEXT NOT NULL CHECK (scope_kind IN ('all_authorized', 'store')),
    store_id TEXT NOT NULL DEFAULT '',
    status TEXT NOT NULL CHECK (status IN ('queued', 'running', 'succeeded', 'failed')),
    error_code TEXT NOT NULL DEFAULT '',
    created_at DATETIME NOT NULL,
	updated_at DATETIME NOT NULL,
	PRIMARY KEY (tenant_id, scope_kind, store_id),
    CHECK ((scope_kind = 'all_authorized' AND store_id = '') OR (scope_kind = 'store' AND store_id <> ''))
);
CREATE TABLE operating_brief_snapshots (
	snapshot_ref TEXT NOT NULL UNIQUE,
    tenant_id INTEGER NOT NULL REFERENCES tenants(id) ON DELETE CASCADE,
    scope_kind TEXT NOT NULL CHECK (scope_kind IN ('all_authorized', 'store')),
    store_id TEXT NOT NULL DEFAULT '',
    source_id TEXT NOT NULL,
    binding_id TEXT NOT NULL,
    binding_revision INTEGER NOT NULL,
    deployment_revision INTEGER NOT NULL,
    catalog_version TEXT NOT NULL,
    freshness_token TEXT NOT NULL,
    state TEXT NOT NULL CHECK (state IN ('ready', 'no_data')),
    quality TEXT NOT NULL DEFAULT '',
    reason_code TEXT NOT NULL DEFAULT '',
    input_set_digest TEXT NOT NULL DEFAULT '',
    revision INTEGER,
    ready_at TEXT NOT NULL DEFAULT '',
    data_json TEXT NOT NULL,
    handoff_questions_json TEXT NOT NULL,
    snapshot_metadata_json TEXT NOT NULL,
    fixed_slots_json TEXT NOT NULL,
	generated_at DATETIME NOT NULL,
	PRIMARY KEY (tenant_id, scope_kind, store_id),
    CHECK ((scope_kind = 'all_authorized' AND store_id = '') OR (scope_kind = 'store' AND store_id <> ''))
);
CREATE TABLE operating_brief_scope_refs (
    scope_ref TEXT PRIMARY KEY,
    tenant_id INTEGER NOT NULL REFERENCES tenants(id) ON DELETE CASCADE,
    store_id TEXT NOT NULL,
    label TEXT NOT NULL,
    updated_at DATETIME NOT NULL,
    UNIQUE (tenant_id, store_id)
);
CREATE INDEX idx_operating_brief_scope_ref_tenant
    ON operating_brief_scope_refs(tenant_id, scope_ref);
