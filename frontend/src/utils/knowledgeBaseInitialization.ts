interface KnowledgeBaseModelConfig {
  summary_model_id?: string
  embedding_model_id?: string
  indexing_strategy?: { vector_enabled?: boolean; keyword_enabled?: boolean }
}

// Enterprise responses omit infrastructure fields. Only platform admins can
// diagnose configuration from these fields; processing is validated server-side.
export function needsKnowledgeBaseConfiguration(kb: KnowledgeBaseModelConfig, isSystemAdmin: boolean): boolean {
  if (!isSystemAdmin) return false
  if (!kb.summary_model_id) return true
  const strategy = kb.indexing_strategy
  const needsEmbedding = !strategy || strategy.vector_enabled || strategy.keyword_enabled
  return !!needsEmbedding && !kb.embedding_model_id
}
