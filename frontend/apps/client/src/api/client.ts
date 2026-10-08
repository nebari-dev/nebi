import axios from 'axios';
import { getApiBaseUrl } from '@/lib/basePath';

const API_BASE_URL = import.meta.env.VITE_API_URL || getApiBaseUrl();

export const apiClient = axios.create({
  baseURL: API_BASE_URL,
  headers: {
    'Content-Type': 'application/json',
  },
});
