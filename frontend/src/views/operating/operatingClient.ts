// Keep this projection in lockstep with the runtime-owned contract at
// agent/apps/web/src/operating/contracts.ts. The runtime is loaded from the
// fixed same-origin entry below, so these types are compile-time only.

export type OperatingLocation = {
  surface: 'analysis' | 'brief'
  sessionId: string | null
  artifactId: string | null
  mode: 'auto' | 'quick' | 'deep' | 'extract'
  tab: 'report' | 'chart' | 'table' | 'caliber'
}

export type OperatingProcess = {
  planItems: Array<{ id: string; text: string; status: string }>
  queries: Array<{
    id: string
    intent: string
    status?: string
    rowCount?: number
    queryExecutionId?: string
    preview?: {
      columns: string[]
      rows: Array<Record<string, string | null>>
      truncated: boolean
    }
    detailAvailable?: boolean
  }>
  calculations: Array<{ id: string; status: 'running' | 'succeeded' | 'failed' }>
}

export type OperatingMessage = {
  id: string
  role: 'user' | 'assistant'
  text: string
  attachments?: Array<{ id: string; name: string }>
  createdAt: string | null
  status: 'running' | 'completed' | 'cancelled' | 'failed' | 'incomplete'
  process: OperatingProcess
  report: null | { artifactId: string; title: string; summary: string }
}

type OperatingQueryReadingTarget = {
  kind: 'query'
  artifactId: string | null
  title: string
  columns: string[]
  columnLabels?: Record<string, string>
  rows: Array<Record<string, string | null>>
  rowCount: number
  truncated: boolean
  source: 'report' | 'preview'
}

export type OperatingReadingIntent =
  | { kind: 'following-current' }
  | { kind: 'inspect'; artifactId: string; target: { kind: 'report' } | OperatingQueryReadingTarget }
  | { kind: 'inspect-query'; target: OperatingQueryReadingTarget & { artifactId: null; source: 'preview' } }
  | { kind: 'closed' }

export type OperatingSnapshot = {
  auth: 'loading' | 'ready' | 'error'
  error: string | null
  title: string
  location: OperatingLocation
  draft: string
  running: boolean
  disabled: boolean
  cancelling: boolean
  sessionsLoading: boolean
  sessions: Array<{ id: string; title: string; updatedAt: string }>
  sessionLoadError: { sessionId: string; message: string; retrying: boolean } | null
  messages: OperatingMessage[]
  attachments: Array<{ id: string; name: string; size: number; status: 'ready' }>
  uploadAvailable: boolean
  uploadPending: boolean
  artifacts: Array<{ id: string; title: string }>
  selectedArtifactId: string | null
  reading: OperatingReadingIntent
}

export interface OperatingController {
  getSnapshot(): OperatingSnapshot
  subscribe(listener: () => void): () => void
  setDraft(text: string): void
  send(text: string): boolean
  cancel(): void
  upload(files: File[]): Promise<void>
  removeAttachment(id: string): Promise<void>
  openSession(id: string): Promise<void>
  retrySession(): Promise<void>
  newSession(): void
  renameSession(id: string, title: string): Promise<void>
  deleteSession(id: string): Promise<void>
  selectArtifact(id: string): void
  inspectQuery(messageId: string, queryId: string): void
  closeReading(): void
  returnToReport(): void
  navigate(location: OperatingLocation): void
  setActive(active: boolean): void
  mountReport(target: HTMLElement | null): void
  mountControls(target: HTMLElement | null): void
  mountBrief(target: HTMLElement | null): void
  dispose(): void
}

export type CreateOperatingControllerOptions = {
  runtimeRoot: HTMLElement
  initialLocation: OperatingLocation
  onNavigate(location: OperatingLocation, mode: 'push' | 'replace'): void
}

export type OperatingClientModule = {
  createController(options: CreateOperatingControllerOptions): OperatingController
}

export const OPERATING_CLIENT_MODULE_PATH = '/app/operating-client.js'

type ModuleImporter = () => Promise<unknown>

const importOperatingClient: ModuleImporter = () =>
  import(/* @vite-ignore */ OPERATING_CLIENT_MODULE_PATH)

export async function loadOperatingClient(
  importer: ModuleImporter = importOperatingClient,
): Promise<OperatingClientModule> {
  let module: unknown
  try {
    module = await importer()
  } catch (cause) {
    console.error('Operating client module could not load', cause)
    throw new Error('经营分析组件加载失败，请重试。', { cause })
  }

  if (!module || typeof module !== 'object'
    || typeof (module as Partial<OperatingClientModule>).createController !== 'function') {
    throw new Error('经营分析组件版本不兼容，请刷新后重试。')
  }

  return module as OperatingClientModule
}
