ALTER TABLE tenants ADD COLUMN chat_history_config JSONB;

UPDATE tenants AS t
SET chat_history_config = jsonb_build_object(
    'enabled', cfg.enabled,
    'embedding_model_id', kb.embedding_model_id,
    'knowledge_base_id', idx.knowledge_base_id
)
FROM tenant_chat_history_indexes AS idx
JOIN knowledge_bases AS kb ON kb.id = idx.knowledge_base_id
CROSS JOIN platform_chat_history_config AS cfg
WHERE t.id = idx.tenant_id
  AND cfg.id = 1;

DROP TABLE tenant_chat_history_indexes;
DROP TABLE platform_chat_history_config;
