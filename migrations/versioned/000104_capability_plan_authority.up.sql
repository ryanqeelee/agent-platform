CREATE TABLE IF NOT EXISTS ai_capability_plan_versions (
    version_id TEXT PRIMARY KEY,
    contract_version TEXT NOT NULL CHECK (contract_version = 'AICapabilityPlanV1'),
    service_level TEXT NOT NULL,
    employee_assistant_request_runtime_ref TEXT NOT NULL,
    operating_analysis_request_runtime_ref TEXT NOT NULL,
    embedding_ref TEXT NOT NULL,
    reranking_ref TEXT NOT NULL,
    parsing_ref TEXT NOT NULL,
    created_by TEXT NOT NULL,
    created_at TIMESTAMP WITH TIME ZONE NOT NULL
);

CREATE TABLE IF NOT EXISTS ai_capability_plan_default (
    singleton_key BOOLEAN PRIMARY KEY CHECK (singleton_key),
    version_id TEXT NOT NULL REFERENCES ai_capability_plan_versions(version_id) ON DELETE RESTRICT,
    updated_by TEXT NOT NULL,
    updated_at TIMESTAMP WITH TIME ZONE NOT NULL
);

CREATE TABLE IF NOT EXISTS tenant_ai_capability_plan_assignments (
    tenant_id INTEGER PRIMARY KEY REFERENCES tenants(id) ON DELETE CASCADE,
    version_id TEXT NOT NULL REFERENCES ai_capability_plan_versions(version_id) ON DELETE RESTRICT,
    updated_by TEXT NOT NULL,
    updated_at TIMESTAMP WITH TIME ZONE NOT NULL
);

CREATE TABLE IF NOT EXISTS assistant_scenario_capability_default (
    singleton_key BOOLEAN PRIMARY KEY CHECK (singleton_key),
    external_search BOOLEAN NOT NULL,
    mcp BOOLEAN NOT NULL,
    tools BOOLEAN NOT NULL,
    updated_by TEXT NOT NULL,
    updated_at TIMESTAMP WITH TIME ZONE NOT NULL
);

CREATE TABLE IF NOT EXISTS tenant_assistant_scenario_capability_overrides (
    tenant_id INTEGER PRIMARY KEY REFERENCES tenants(id) ON DELETE CASCADE,
    external_search BOOLEAN NOT NULL,
    mcp BOOLEAN NOT NULL,
    tools BOOLEAN NOT NULL,
    updated_by TEXT NOT NULL,
    updated_at TIMESTAMP WITH TIME ZONE NOT NULL
);
