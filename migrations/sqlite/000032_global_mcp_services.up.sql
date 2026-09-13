PRAGMA foreign_keys=OFF;

CREATE TABLE mcp_services_global (
    id VARCHAR(36) PRIMARY KEY,
    name VARCHAR(255) NOT NULL,
    description TEXT,
    enabled BOOLEAN DEFAULT 1,
    transport_type VARCHAR(50) NOT NULL,
    url VARCHAR(512),
    headers JSON,
    auth_config JSON,
    advanced_config JSON,
    stdio_config JSON,
    env_vars JSON,
    is_builtin BOOLEAN NOT NULL DEFAULT 0,
    created_at DATETIME DEFAULT CURRENT_TIMESTAMP,
    updated_at DATETIME DEFAULT CURRENT_TIMESTAMP,
    deleted_at DATETIME
);

INSERT INTO mcp_services_global (
    id, name, description, enabled, transport_type, url, headers, auth_config,
    advanced_config, stdio_config, env_vars, is_builtin, created_at, updated_at, deleted_at
)
SELECT id, name, description, enabled, transport_type, url, headers, auth_config,
       advanced_config, stdio_config, env_vars, is_builtin, created_at, updated_at, deleted_at
FROM mcp_services;

DROP TABLE mcp_services;
ALTER TABLE mcp_services_global RENAME TO mcp_services;
CREATE INDEX idx_mcp_services_enabled ON mcp_services(enabled);
CREATE INDEX idx_mcp_services_deleted_at ON mcp_services(deleted_at);
CREATE INDEX idx_mcp_services_is_builtin ON mcp_services(is_builtin);

CREATE TABLE mcp_tool_approvals_global (
    id VARCHAR(36) PRIMARY KEY,
    service_id VARCHAR(36) NOT NULL,
    tool_name VARCHAR(512) NOT NULL,
    require_approval BOOLEAN NOT NULL DEFAULT 0,
    created_at DATETIME DEFAULT CURRENT_TIMESTAMP,
    updated_at DATETIME DEFAULT CURRENT_TIMESTAMP,
    FOREIGN KEY (service_id) REFERENCES mcp_services(id) ON DELETE CASCADE,
    UNIQUE (service_id, tool_name)
);

INSERT INTO mcp_tool_approvals_global (
    id, service_id, tool_name, require_approval, created_at, updated_at
)
SELECT id, service_id, tool_name, require_approval, created_at, updated_at
FROM mcp_tool_approvals;

DROP TABLE mcp_tool_approvals;
ALTER TABLE mcp_tool_approvals_global RENAME TO mcp_tool_approvals;
CREATE UNIQUE INDEX idx_mcp_tool_approvals_svc_tool
    ON mcp_tool_approvals(service_id, tool_name);
CREATE INDEX idx_mcp_tool_approvals_service_id ON mcp_tool_approvals(service_id);

PRAGMA foreign_keys=ON;
