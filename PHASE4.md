# Phase 4: User Interface

## Overview

Phase 4 focuses on building a modern, responsive React frontend for Darb. The UI will provide an intuitive interface for managing environments, installing packages, and monitoring job progress in real-time.

## Current State (Phase 3 Complete ✅)

The backend is fully functional:
- ✅ REST API with all environment operations
- ✅ Authentication with JWT tokens
- ✅ Job queue with async processing
- ✅ Package manager operations (pixi)
- ✅ Real-time job tracking and logs

## Phase 4 Goals

1. **React Frontend** - Modern SPA with TypeScript
2. **Authentication UI** - Login and session management
3. **Environment Management** - View, create, delete environments
4. **Package Management** - Install and remove packages
5. **Job Monitoring** - Real-time job status and log viewing
6. **Responsive Design** - Works on desktop and mobile

## Tech Stack

### Core
- **React 18** - UI framework
- **TypeScript** - Type safety
- **Vite** - Fast build tool and dev server

### Styling
- **Tailwind CSS** - Utility-first CSS framework
- **Headless UI** - Unstyled, accessible UI components
- **Heroicons** - Beautiful hand-crafted SVG icons

### State Management & Data Fetching
- **TanStack Query (React Query)** - Server state management
- **Axios** - HTTP client
- **Zustand** - Lightweight client state (for auth token)

### Routing
- **React Router v6** - Client-side routing

### Form Handling
- **React Hook Form** - Performant form validation

## Project Structure

```
frontend/
├── public/
│   └── vite.svg
├── src/
│   ├── api/
│   │   ├── client.ts          # Axios instance with auth interceptor
│   │   ├── auth.ts            # Authentication API calls
│   │   ├── environments.ts    # Environment API calls
│   │   ├── jobs.ts            # Job API calls
│   │   └── packages.ts        # Package API calls
│   ├── components/
│   │   ├── layout/
│   │   │   ├── Header.tsx
│   │   │   ├── Sidebar.tsx
│   │   │   └── Layout.tsx
│   │   ├── environments/
│   │   │   ├── EnvironmentList.tsx
│   │   │   ├── EnvironmentCard.tsx
│   │   │   ├── CreateEnvironmentModal.tsx
│   │   │   └── EnvironmentDetails.tsx
│   │   ├── packages/
│   │   │   ├── PackageList.tsx
│   │   │   ├── InstallPackageModal.tsx
│   │   │   └── PackageCard.tsx
│   │   ├── jobs/
│   │   │   ├── JobList.tsx
│   │   │   ├── JobStatusBadge.tsx
│   │   │   └── JobLogs.tsx
│   │   └── common/
│   │       ├── Button.tsx
│   │       ├── Input.tsx
│   │       ├── Card.tsx
│   │       ├── Modal.tsx
│   │       └── Spinner.tsx
│   ├── hooks/
│   │   ├── useAuth.ts
│   │   ├── useEnvironments.ts
│   │   ├── useJobs.ts
│   │   └── usePackages.ts
│   ├── pages/
│   │   ├── Login.tsx
│   │   ├── Dashboard.tsx
│   │   ├── Environments.tsx
│   │   ├── EnvironmentDetail.tsx
│   │   └── Jobs.tsx
│   ├── store/
│   │   └── authStore.ts       # Zustand store for auth
│   ├── types/
│   │   ├── api.ts             # API response types
│   │   ├── models.ts          # Domain models
│   │   └── index.ts
│   ├── utils/
│   │   ├── format.ts          # Date/time formatting
│   │   └── constants.ts       # App constants
│   ├── App.tsx
│   ├── main.tsx
│   └── index.css
├── index.html
├── package.json
├── tsconfig.json
├── vite.config.ts
├── tailwind.config.js
└── postcss.config.js
```

## Implementation Plan

### Step 1: Project Setup

**Initialize Vite React Project:**
```bash
cd /Users/aktech/dev/darb
npm create vite@latest frontend -- --template react-ts
cd frontend
npm install
```

**Install Dependencies:**
```bash
# Core dependencies
npm install react-router-dom axios @tanstack/react-query zustand

# UI dependencies
npm install -D tailwindcss postcss autoprefixer
npm install @headlessui/react @heroicons/react

# Form handling
npm install react-hook-form

# Development dependencies
npm install -D @types/node
```

**Setup Tailwind CSS:**
```bash
npx tailwindcss init -p
```

Update `tailwind.config.js`:
```js
/** @type {import('tailwindcss').Config} */
export default {
  content: [
    "./index.html",
    "./src/**/*.{js,ts,jsx,tsx}",
  ],
  theme: {
    extend: {},
  },
  plugins: [],
}
```

Update `src/index.css`:
```css
@tailwind base;
@tailwind components;
@tailwind utilities;
```

### Step 2: API Client Setup

Create `src/api/client.ts`:
```typescript
import axios from 'axios';

const API_BASE_URL = import.meta.env.VITE_API_URL || 'http://localhost:8080/api/v1';

export const apiClient = axios.create({
  baseURL: API_BASE_URL,
  headers: {
    'Content-Type': 'application/json',
  },
});

// Request interceptor to add auth token
apiClient.interceptors.request.use(
  (config) => {
    const token = localStorage.getItem('auth_token');
    if (token) {
      config.headers.Authorization = `Bearer ${token}`;
    }
    return config;
  },
  (error) => Promise.reject(error)
);

// Response interceptor for error handling
apiClient.interceptors.response.use(
  (response) => response,
  (error) => {
    if (error.response?.status === 401) {
      localStorage.removeItem('auth_token');
      window.location.href = '/login';
    }
    return Promise.reject(error);
  }
);
```

Create `src/api/auth.ts`:
```typescript
import { apiClient } from './client';

export interface LoginRequest {
  username: string;
  password: string;
}

export interface LoginResponse {
  token: string;
  user: {
    id: number;
    username: string;
    email: string;
  };
}

export const authApi = {
  login: async (credentials: LoginRequest): Promise<LoginResponse> => {
    const { data } = await apiClient.post('/auth/login', credentials);
    return data;
  },
};
```

Create `src/api/environments.ts`:
```typescript
import { apiClient } from './client';
import { Environment, CreateEnvironmentRequest } from '../types/models';

export const environmentsApi = {
  list: async (): Promise<Environment[]> => {
    const { data } = await apiClient.get('/environments');
    return data;
  },

  get: async (id: number): Promise<Environment> => {
    const { data } = await apiClient.get(`/environments/${id}`);
    return data;
  },

  create: async (req: CreateEnvironmentRequest): Promise<Environment> => {
    const { data } = await apiClient.post('/environments', req);
    return data;
  },

  delete: async (id: number): Promise<void> => {
    await apiClient.delete(`/environments/${id}`);
  },
};
```

Similar files for `jobs.ts` and `packages.ts`.

### Step 3: Type Definitions

Create `src/types/models.ts`:
```typescript
export interface User {
  id: number;
  username: string;
  email: string;
  created_at: string;
  updated_at: string;
}

export type EnvironmentStatus = 'pending' | 'creating' | 'ready' | 'failed' | 'deleting';

export interface Environment {
  id: number;
  name: string;
  owner_id: number;
  status: EnvironmentStatus;
  package_manager: string;
  created_at: string;
  updated_at: string;
}

export interface CreateEnvironmentRequest {
  name: string;
  package_manager?: string;
}

export type JobType = 'create' | 'delete' | 'install' | 'remove' | 'update';
export type JobStatus = 'pending' | 'running' | 'completed' | 'failed';

export interface Job {
  id: number;
  environment_id: number;
  type: JobType;
  status: JobStatus;
  logs: string;
  error?: string;
  metadata?: Record<string, any>;
  created_at: string;
  started_at?: string;
  completed_at?: string;
}

export interface Package {
  id: number;
  environment_id: number;
  name: string;
  version?: string;
  installed_at: string;
}

export interface InstallPackagesRequest {
  packages: string[];
}
```

### Step 4: Authentication Store

Create `src/store/authStore.ts`:
```typescript
import { create } from 'zustand';
import { persist } from 'zustand/middleware';

interface AuthState {
  token: string | null;
  user: {
    id: number;
    username: string;
    email: string;
  } | null;
  setAuth: (token: string, user: AuthState['user']) => void;
  clearAuth: () => void;
  isAuthenticated: () => boolean;
}

export const useAuthStore = create<AuthState>()(
  persist(
    (set, get) => ({
      token: null,
      user: null,
      setAuth: (token, user) => {
        localStorage.setItem('auth_token', token);
        set({ token, user });
      },
      clearAuth: () => {
        localStorage.removeItem('auth_token');
        set({ token: null, user: null });
      },
      isAuthenticated: () => !!get().token,
    }),
    {
      name: 'auth-storage',
    }
  )
);
```

### Step 5: Custom Hooks

Create `src/hooks/useEnvironments.ts`:
```typescript
import { useQuery, useMutation, useQueryClient } from '@tanstack/react-query';
import { environmentsApi } from '../api/environments';
import { CreateEnvironmentRequest } from '../types/models';

export const useEnvironments = () => {
  return useQuery({
    queryKey: ['environments'],
    queryFn: environmentsApi.list,
    refetchInterval: 2000, // Poll every 2 seconds for status updates
  });
};

export const useEnvironment = (id: number) => {
  return useQuery({
    queryKey: ['environments', id],
    queryFn: () => environmentsApi.get(id),
    enabled: !!id,
    refetchInterval: 2000,
  });
};

export const useCreateEnvironment = () => {
  const queryClient = useQueryClient();

  return useMutation({
    mutationFn: (data: CreateEnvironmentRequest) => environmentsApi.create(data),
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: ['environments'] });
    },
  });
};

export const useDeleteEnvironment = () => {
  const queryClient = useQueryClient();

  return useMutation({
    mutationFn: (id: number) => environmentsApi.delete(id),
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: ['environments'] });
    },
  });
};
```

### Step 6: Core Components

**Login Page** (`src/pages/Login.tsx`):
```tsx
import { useState } from 'react';
import { useNavigate } from 'react-router-dom';
import { useAuthStore } from '../store/authStore';
import { authApi } from '../api/auth';

export const Login = () => {
  const [username, setUsername] = useState('');
  const [password, setPassword] = useState('');
  const [error, setError] = useState('');
  const [loading, setLoading] = useState(false);

  const setAuth = useAuthStore((state) => state.setAuth);
  const navigate = useNavigate();

  const handleSubmit = async (e: React.FormEvent) => {
    e.preventDefault();
    setError('');
    setLoading(true);

    try {
      const response = await authApi.login({ username, password });
      setAuth(response.token, response.user);
      navigate('/');
    } catch (err: any) {
      setError(err.response?.data?.error || 'Login failed');
    } finally {
      setLoading(false);
    }
  };

  return (
    <div className="min-h-screen flex items-center justify-center bg-gray-50">
      <div className="max-w-md w-full space-y-8 p-8 bg-white rounded-lg shadow">
        <div>
          <h2 className="text-3xl font-bold text-center">Darb</h2>
          <p className="mt-2 text-center text-gray-600">
            Environment Management System
          </p>
        </div>

        <form onSubmit={handleSubmit} className="mt-8 space-y-6">
          {error && (
            <div className="bg-red-50 text-red-600 p-3 rounded">
              {error}
            </div>
          )}

          <div>
            <label className="block text-sm font-medium text-gray-700">
              Username
            </label>
            <input
              type="text"
              value={username}
              onChange={(e) => setUsername(e.target.value)}
              className="mt-1 block w-full px-3 py-2 border border-gray-300 rounded-md"
              required
            />
          </div>

          <div>
            <label className="block text-sm font-medium text-gray-700">
              Password
            </label>
            <input
              type="password"
              value={password}
              onChange={(e) => setPassword(e.target.value)}
              className="mt-1 block w-full px-3 py-2 border border-gray-300 rounded-md"
              required
            />
          </div>

          <button
            type="submit"
            disabled={loading}
            className="w-full flex justify-center py-2 px-4 border border-transparent rounded-md shadow-sm text-white bg-indigo-600 hover:bg-indigo-700 disabled:opacity-50"
          >
            {loading ? 'Signing in...' : 'Sign in'}
          </button>
        </form>
      </div>
    </div>
  );
};
```

**Dashboard** (`src/pages/Dashboard.tsx`):
- Overview of environments
- Quick actions
- Recent jobs

**Environments Page** (`src/pages/Environments.tsx`):
- List all environments
- Create new environment button
- Environment cards with status badges
- Delete environment action

**Environment Detail Page** (`src/pages/EnvironmentDetail.tsx`):
- Environment information
- List of installed packages
- Install package button
- Remove package action
- View related jobs

**Jobs Page** (`src/pages/Jobs.tsx`):
- List all jobs
- Filter by status/type
- View job logs
- Real-time status updates

### Step 7: Routing

Create `src/App.tsx`:
```tsx
import { BrowserRouter, Routes, Route, Navigate } from 'react-router-dom';
import { QueryClient, QueryClientProvider } from '@tanstack/react-query';
import { useAuthStore } from './store/authStore';
import { Login } from './pages/Login';
import { Dashboard } from './pages/Dashboard';
import { Environments } from './pages/Environments';
import { EnvironmentDetail } from './pages/EnvironmentDetail';
import { Jobs } from './pages/Jobs';
import { Layout } from './components/layout/Layout';

const queryClient = new QueryClient();

const PrivateRoute = ({ children }: { children: React.ReactNode }) => {
  const isAuthenticated = useAuthStore((state) => state.isAuthenticated());
  return isAuthenticated ? <>{children}</> : <Navigate to="/login" />;
};

function App() {
  return (
    <QueryClientProvider client={queryClient}>
      <BrowserRouter>
        <Routes>
          <Route path="/login" element={<Login />} />
          <Route
            path="/"
            element={
              <PrivateRoute>
                <Layout />
              </PrivateRoute>
            }
          >
            <Route index element={<Dashboard />} />
            <Route path="environments" element={<Environments />} />
            <Route path="environments/:id" element={<EnvironmentDetail />} />
            <Route path="jobs" element={<Jobs />} />
          </Route>
        </Routes>
      </BrowserRouter>
    </QueryClientProvider>
  );
}

export default App;
```

### Step 8: Development Environment

Update `vite.config.ts`:
```typescript
import { defineConfig } from 'vite'
import react from '@vitejs/plugin-react'

export default defineConfig({
  plugins: [react()],
  server: {
    port: 3000,
    proxy: {
      '/api': {
        target: 'http://localhost:8080',
        changeOrigin: true,
      },
    },
  },
})
```

Create `.env`:
```
VITE_API_URL=http://localhost:8080/api/v1
```

## Key Features to Implement

### 1. Environment Management
- **List View**: Grid/list of environment cards
- **Status Indicators**: Color-coded badges for status
- **Quick Actions**: Create, delete, view details
- **Empty State**: When no environments exist

### 2. Package Management
- **Search**: Search for packages to install
- **Install**: Modal with package name input
- **List**: Show installed packages with versions
- **Remove**: Confirm before removing

### 3. Job Monitoring
- **Real-time Updates**: Poll for status changes
- **Log Viewer**: Expandable log display
- **Status Badges**: Visual indicators for job status
- **Filtering**: Filter by status/type

### 4. User Experience
- **Loading States**: Spinners and skeletons
- **Error Handling**: Toast notifications
- **Confirmation Dialogs**: For destructive actions
- **Responsive**: Mobile-friendly design

## Testing Phase 4

**Manual Testing Checklist:**

1. **Authentication:**
   - [ ] Login with valid credentials
   - [ ] Login with invalid credentials shows error
   - [ ] Token persists across page refreshes
   - [ ] Logout clears token and redirects to login

2. **Environments:**
   - [ ] List all environments
   - [ ] Create new environment
   - [ ] View environment details
   - [ ] Delete environment
   - [ ] Status updates in real-time

3. **Packages:**
   - [ ] List packages in environment
   - [ ] Install package
   - [ ] Remove package
   - [ ] View installation progress

4. **Jobs:**
   - [ ] List all jobs
   - [ ] View job details
   - [ ] View job logs
   - [ ] Status updates in real-time

## Acceptance Criteria

Phase 4 is complete when:

- [ ] React app runs and connects to backend API
- [ ] User can login and stay authenticated
- [ ] User can view all environments
- [ ] User can create new environment
- [ ] User can delete environment
- [ ] User can install packages in environment
- [ ] User can remove packages from environment
- [ ] User can view job status and logs
- [ ] UI is responsive and works on mobile
- [ ] Error states are handled gracefully

## Development Workflow

```bash
# Terminal 1: Run backend
cd /Users/aktech/dev/darb
make dev

# Terminal 2: Run frontend
cd /Users/aktech/dev/darb/frontend
npm run dev
```

Access the app at http://localhost:3000

## Next Steps After Phase 4

Once the UI is complete:
- **Phase 5**: Add Docker/Kubernetes runtime support
- **Phase 6**: Implement WebSocket for real-time log streaming
- **Phase 7**: Add RBAC and multi-user permissions
- **Phase 8**: Build admin interface
- **Phase 9**: Package for production deployment

## Notes for Implementation

1. **API Polling**: Use React Query's `refetchInterval` for real-time updates
2. **Error Boundaries**: Add error boundaries for graceful error handling
3. **Accessibility**: Use Headless UI for accessible components
4. **Performance**: Lazy load routes with React.lazy
5. **Code Splitting**: Vite handles this automatically
6. **Environment Variables**: Use VITE_ prefix for env vars

## Resources

- **React**: https://react.dev
- **Vite**: https://vitejs.dev
- **TanStack Query**: https://tanstack.com/query
- **Tailwind CSS**: https://tailwindcss.com
- **Headless UI**: https://headlessui.com
- **React Router**: https://reactrouter.com

---

**Ready to start Phase 4?** Begin with project setup and API client configuration!
