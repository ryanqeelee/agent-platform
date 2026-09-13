PRAGMA foreign_keys=OFF;

CREATE TABLE web_search_providers_global (
    id VARCHAR(36) NOT NULL PRIMARY KEY,
    name VARCHAR(255) NOT NULL,
    provider VARCHAR(50) NOT NULL,
    description TEXT,
    parameters JSON,
    is_default BOOLEAN NOT NULL DEFAULT 0,
    created_at DATETIME DEFAULT CURRENT_TIMESTAMP,
    updated_at DATETIME DEFAULT CURRENT_TIMESTAMP,
    deleted_at DATETIME NULL
);

INSERT INTO web_search_providers_global (
    id, name, provider, description, parameters, is_default, created_at, updated_at, deleted_at
)
SELECT id, name, provider, description, parameters, 0, created_at, updated_at, deleted_at
FROM web_search_providers;

DROP TABLE web_search_providers;
ALTER TABLE web_search_providers_global RENAME TO web_search_providers;
CREATE INDEX idx_web_search_providers_provider ON web_search_providers(provider);
CREATE INDEX idx_web_search_providers_deleted_at ON web_search_providers(deleted_at);
CREATE UNIQUE INDEX idx_web_search_providers_single_default
    ON web_search_providers(is_default)
    WHERE is_default = 1 AND deleted_at IS NULL;

ALTER TABLE tenants DROP COLUMN web_search_config;

PRAGMA foreign_keys=ON;
