import type { LoginRequest, LoginResponse } from '@/types';
import { apiClient } from './client';

export const authApi = {
  login: async (credentials: LoginRequest): Promise<LoginResponse> => {
    const { data } = await apiClient.post('/auth/login', credentials);
    return data;
  },
};
