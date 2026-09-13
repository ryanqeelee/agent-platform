ALTER TABLE tenants ADD COLUMN chat_history_config TEXT;

UPDATE tenants
SET chat_history_config = (
    SELECT json_object(
        'enabled', json(CASE WHEN cfg.enabled = 1 THEN 'true' ELSE 'false' END),
        'embedding_model_id', kb.embedding_model_id,
        'knowledge_base_id', idx.knowledge_base_id
    )
    FROM tenant_chat_history_indexes AS idx
    JOIN knowledge_bases AS kb ON kb.id = idx.knowledge_base_id
    CROSS JOIN platform_chat_history_config AS cfg
    WHERE idx.tenant_id = tenants.id
      AND cfg.id = 1
);

DROP TABLE tenant_chat_history_indexes;
DROP TABLE platform_chat_history_config;
