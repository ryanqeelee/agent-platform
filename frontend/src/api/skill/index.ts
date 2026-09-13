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

const platformSkillPath = '/api/v1/system/admin/skills';

export function listSkillCatalog() {
  return get<{ data: SkillCatalogItem[] }>(platformSkillPath);
}

export function getSkillInstallerAgent() {
  return get<{ data: CustomAgent }>('/api/v1/system/admin/agents/builtin-skill-installer');
}

export function updateSkillInstallerAgent(data: UpdateAgentRequest) {
  return put<{ data: CustomAgent }>('/api/v1/system/admin/agents/builtin-skill-installer', data);
}

export function registerSkillCatalogFromSource(source: string) {
  return post<{ data: SkillCatalogRegisterResult }>(platformSkillPath, { source }, {
    timeout: 2 * 60 * 1000,
  });
}

export function registerSkillCatalogFromFile(
  file: File,
  onProgress?: (percent: number) => void,
) {
  const form = new FormData();
  form.append('file', file);
  return postUpload(platformSkillPath, form, (e: any) => {
    if (e.total) onProgress?.(Math.round((e.loaded * 100) / e.total));
  }, { timeout: 5 * 60 * 1000 }) as Promise<{ data: SkillCatalogRegisterResult }>;
}

export function installSkillCatalog(catalogId: string, sandboxConfigIds: string[]) {
  return post<{ data: { installs: Record<string, string>; errors?: Record<string, string> } }>(
    `${platformSkillPath}/${catalogId}/install`,
    { sandbox_config_ids: sandboxConfigIds },
  );
}

export function deleteSkillCatalog(catalogId: string) {
  return del(`${platformSkillPath}/${catalogId}`);
}

export function listCatalogSkillFiles(catalogId: string) {
  return get<{ data: ConfigSkillFileEntry[] }>(`${platformSkillPath}/${catalogId}/files`);
}

export function getCatalogSkillFile(catalogId: string, path: string) {
  return get<{ data: ConfigSkillFileContent }>(`${platformSkillPath}/${catalogId}/files/content`, {
    params: { path },
  });
}

export function listEmployeeSkills() {
  return get<{ data: SkillInfo[]; skills_available?: boolean }>('/api/v1/employee-assistant/skills');
}
