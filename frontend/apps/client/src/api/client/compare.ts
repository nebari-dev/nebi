// Owner: U4 (compare).
import type { CompareResult } from '@/types/contracts';
import { apiClient, CLIENT } from './base';

export const compareApi = {
  compare: async (
    projectId: string,
    base: number,
    target: number,
  ): Promise<CompareResult> =>
    (
      await apiClient.get(`${CLIENT}/projects/${projectId}/compare`, {
        params: { base, target },
      })
    ).data,
};
