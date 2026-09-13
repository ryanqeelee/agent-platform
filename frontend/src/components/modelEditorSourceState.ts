export type ModelEditorSource = 'local' | 'remote'

export type ModelEditorType = 'chat' | 'embedding' | 'rerank' | 'vllm' | 'asr'

export function normalizeModelEditorSource(
  source: ModelEditorSource,
  modelType: ModelEditorType,
  platformApiOnly: boolean,
): ModelEditorSource {
  return platformApiOnly || modelType === 'rerank' ? 'remote' : source
}

export function shouldShowOllamaUnavailableTip(
  source: ModelEditorSource,
  modelType: ModelEditorType,
  ollamaServiceStatus: boolean | null,
): boolean {
  return source === 'local' && modelType !== 'rerank' && ollamaServiceStatus === false
}
