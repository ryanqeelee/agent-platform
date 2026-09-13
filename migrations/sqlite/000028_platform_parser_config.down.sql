CREATE TEMP TABLE _platform_parser_downgrade_guard (
    configured INTEGER NOT NULL CHECK (configured = 0)
);

INSERT INTO _platform_parser_downgrade_guard (configured)
SELECT CASE WHEN EXISTS (
    SELECT 1 FROM platform_parser_config
    WHERE config NOT IN ('{}', 'null')
) THEN 1 ELSE 0 END;

DROP TABLE _platform_parser_downgrade_guard;

ALTER TABLE tenants ADD COLUMN parser_engine_config TEXT;
DROP TABLE platform_parser_config;
