// Owner: U1 (projects).
import type {
  Capabilities,
  CreateProjectRequest,
  CreateProjectResult,
  JobSummary,
  ManifestParseResult,
  ManifestRenderRequest,
  ManifestRenderResult,
  NameCheckResult,
  PickFolderRequest,
  PickFolderResult,
  ProjectListItem,
} from '@/types/contracts';
import { apiClient, CLIENT } from './base';

export const projectsApi = {
  list: async (): Promise<ProjectListItem[]> =>
    (await apiClient.get(`${CLIENT}/projects`)).data,
  get: async (id: string): Promise<ProjectListItem> =>
    (await apiClient.get(`${CLIENT}/projects/${id}`)).data,
  create: async (req: CreateProjectRequest): Promise<CreateProjectResult> =>
    (await apiClient.post(`${CLIENT}/projects`, req)).data,
  remove: async (id: string): Promise<JobSummary> =>
    (await apiClient.delete(`${CLIENT}/projects/${id}`)).data,
  checkName: async (name: string): Promise<NameCheckResult> =>
    (await apiClient.get(`${CLIENT}/projects/name-check`, { params: { name } }))
      .data,
  parseManifest: async (pixiToml: string): Promise<ManifestParseResult> =>
    (await apiClient.post(`${CLIENT}/manifest/parse`, { pixi_toml: pixiToml }))
      .data,
  renderManifest: async (
    req: ManifestRenderRequest,
  ): Promise<ManifestRenderResult> =>
    (await apiClient.post(`${CLIENT}/manifest/render`, req)).data,
  capabilities: async (): Promise<Capabilities> =>
    (await apiClient.get(`${CLIENT}/capabilities`)).data,
  pickFolder: async (req: PickFolderRequest = {}): Promise<PickFolderResult> =>
    (await apiClient.post(`${CLIENT}/desktop/pick-folder`, req)).data,
};
