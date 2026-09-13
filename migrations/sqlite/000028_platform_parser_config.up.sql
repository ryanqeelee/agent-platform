-- SQLite/Lite counterpart of PostgreSQL migration 000107. The json_tree
-- signature provides deterministic key-order-independent equivalence while
-- retaining array order for parser rules. Secret-bearing legacy JSON cannot be
-- copied because it was stored as plaintext; it must be reconfigured through
-- the encrypted platform API after migration.
CREATE TABLE platform_parser_config (
    id         INTEGER PRIMARY KEY CHECK (id = 1),
    config     TEXT NOT NULL,
    updated_by TEXT NOT NULL,
    created_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP
);

CREATE TEMP TABLE _platform_parser_candidates (
    config TEXT NOT NULL,
    canonical TEXT NOT NULL
);

CREATE TEMP TABLE _platform_parser_validity_guard (
    invalid INTEGER NOT NULL CHECK (invalid = 0)
);

INSERT INTO _platform_parser_validity_guard (invalid)
SELECT CASE WHEN EXISTS (
    SELECT 1 FROM tenants
    WHERE parser_engine_config IS NOT NULL
      AND NOT json_valid(parser_engine_config)
) THEN 1 ELSE 0 END;

DROP TABLE _platform_parser_validity_guard;

INSERT INTO _platform_parser_candidates (config, canonical)
SELECT cleaned.config,
       COALESCE((
           SELECT json_group_object(entry.fullkey, entry.type || ':' || quote(entry.atom))
           FROM (
               SELECT fullkey, type, atom
               FROM json_tree(cleaned.config)
               ORDER BY fullkey
           ) AS entry
       ), '{}')
FROM (
    SELECT parser_engine_config AS config
    FROM tenants
    WHERE parser_engine_config IS NOT NULL
      AND json_valid(parser_engine_config)
) AS cleaned
WHERE cleaned.config NOT IN ('{}', 'null');

CREATE TEMP TABLE _platform_parser_secret_guard (
    plaintext_secret INTEGER NOT NULL CHECK (plaintext_secret = 0)
);

INSERT INTO _platform_parser_secret_guard (plaintext_secret)
SELECT CASE WHEN EXISTS (
    SELECT 1 FROM _platform_parser_candidates
    WHERE COALESCE(json_extract(config, '$.mineru_api_key'), '') <> ''
       OR COALESCE(json_extract(config, '$.paddleocr_vl_cloud_token'), '') <> ''
) THEN 1 ELSE 0 END;

DROP TABLE _platform_parser_secret_guard;

CREATE TEMP TABLE _platform_parser_conflict_guard (
    conflict INTEGER NOT NULL CHECK (conflict = 0)
);

INSERT INTO _platform_parser_conflict_guard (conflict)
SELECT CASE WHEN COUNT(DISTINCT canonical) > 1 THEN 1 ELSE 0 END
FROM _platform_parser_candidates;

INSERT INTO platform_parser_config (id, config, updated_by)
SELECT 1, config, 'migration-000028'
FROM _platform_parser_candidates
GROUP BY canonical;

DROP TABLE _platform_parser_conflict_guard;
DROP TABLE _platform_parser_candidates;

ALTER TABLE tenants DROP COLUMN parser_engine_config;
