import { del, get, post, postUpload, put } from "../../utils/request";
import type { CustomAgent, UpdateAgentRequest } from "../agent";
import type { ConfigSkillFileContent, ConfigSkillFileEntry } from "../system";

// Skill信息
export interface SkillInfo {
  name: string;
  description: string;
}

export interface SkillCatalogInstall {
  skill_id: string;
  sandbox_config_id: string;
  sandbox_config_name?: string;
  sandbox_type?: string;
  status: string;
  enabled: boolean;
  error?: string;
  bundle_sha256?: string;
  updated_at: string;
}

export interface SkillCatalogItem {
  id: string;
  name: string;
  version?: string;
  description?: string;
  bundle_sha256?: string;
  created_at: string;
  updated_at: string;
  installations: SkillCatalogInstall[];
}

export interface SkillCatalogRegisterResult {
  id: string;
  name: string;
  version?: string;
  description?: string;
}

function platformSkillPath(tenantId: number): string {
  if (!Number.isSafeInteger(tenantId) || Number(tenantId) <= 0) {
    throw new Error('请先选择企业')
  }
  return `/api/v1/system/admin/tenants/${tenantId}/skills`
}

// 获取当前沙箱配置上可执行的 Skills；未传 sandboxConfigId 或
// skills_available 为 false 时，前端应隐藏/禁用 Skills 配置
export function listSkills(tenantId: number, sandboxConfigId?: string) {
  return get<{ data: SkillInfo[]; skills_available?: boolean }>(platformSkillPath(tenantId), {
    params: sandboxConfigId ? { sandbox_config_id: sandboxConfigId } : {},
  });
}

export function listSkillCatalog(tenantId: number) {
  return get<{ data: SkillCatalogItem[] }>(`${platformSkillPath(tenantId)}/catalog`);
}

export function getSkillInstallerAgent(tenantId: number) {
  return get<{ data: CustomAgent }>(`${platformSkillPath(tenantId)}/installer-agent`);
}

export function updateSkillInstallerAgent(tenantId: number, data: UpdateAgentRequest) {
  return put<{ data: CustomAgent }>(`${platformSkillPath(tenantId)}/installer-agent`, data);
}

export function registerSkillCatalogFromSource(tenantId: number, source: string) {
  return post<{ data: SkillCatalogRegisterResult }>(`${platformSkillPath(tenantId)}/catalog`, { source }, {
    timeout: 2 * 60 * 1000,
  });
}

export function registerSkillCatalogFromFile(
  tenantId: number,
  file: File,
  onProgress?: (percent: number) => void,
) {
  const form = new FormData();
  form.append('file', file);
  return postUpload(`${platformSkillPath(tenantId)}/catalog`, form, (e: any) => {
    if (e.total) onProgress?.(Math.round((e.loaded * 100) / e.total));
  }, { timeout: 5 * 60 * 1000 }) as Promise<{ data: SkillCatalogRegisterResult }>;
}

export function installSkillCatalog(tenantId: number, catalogId: string, sandboxConfigIds: string[]) {
  return post<{ data: { installs: Record<string, string>; errors?: Record<string, string> } }>(
    `${platformSkillPath(tenantId)}/catalog/${catalogId}/install`,
    { sandbox_config_ids: sandboxConfigIds },
  );
}

export function deleteSkillCatalog(tenantId: number, catalogId: string) {
  return del(`${platformSkillPath(tenantId)}/catalog/${catalogId}`);
}

export function listCatalogSkillFiles(tenantId: number, catalogId: string) {
  return get<{ data: ConfigSkillFileEntry[] }>(`${platformSkillPath(tenantId)}/catalog/${catalogId}/files`);
}

export function getCatalogSkillFile(tenantId: number, catalogId: string, path: string) {
  return get<{ data: ConfigSkillFileContent }>(`${platformSkillPath(tenantId)}/catalog/${catalogId}/files/content`, {
    params: { path },
  });
}

export function listEmployeeSkills() {
  return get<{ data: SkillInfo[]; skills_available?: boolean }>('/api/v1/employee-assistant/skills');
}
