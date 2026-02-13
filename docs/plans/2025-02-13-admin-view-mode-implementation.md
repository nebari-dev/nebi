# Admin Pages View Mode Implementation Plan

> **For Claude:** REQUIRED SUB-SKILL: Use superpowers:executing-plans to implement this plan task-by-task.

**Goal:** Add local/remote view mode toggle support to all Admin pages, following the existing pattern from Workspaces/Registries/Jobs.

**Architecture:** Each admin page will detect view mode state and conditionally fetch from either local or remote APIs. Remote API calls proxy through the local server to the connected remote server. No new abstractions—just replicate the established pattern.

**Tech Stack:** React, TanStack Query, Zustand stores, TypeScript

---

## Task 1: Add Remote Admin API Endpoints

**Files:**
- Modify: `frontend/src/api/remote.ts`

**Step 1: Add admin endpoint functions to remoteApi**

Add these functions to the `remoteApi` object in `frontend/src/api/remote.ts`:

```typescript
  // Remote admin proxies
  listUsers: async (): Promise<User[]> => {
    const { data } = await apiClient.get('/remote/admin/users');
    return data;
  },

  listAdminRegistries: async (): Promise<OCIRegistry[]> => {
    const { data } = await apiClient.get('/remote/admin/registries');
    return data;
  },

  listAuditLogs: async (params?: { user_id?: string; action?: string }): Promise<AuditLog[]> => {
    const { data } = await apiClient.get('/remote/admin/audit-logs', { params });
    return data;
  },

  getDashboardStats: async (): Promise<DashboardStats> => {
    const { data } = await apiClient.get('/remote/admin/dashboard/stats');
    return data;
  },
```

**Step 2: Add missing type imports**

Update the imports at the top of `frontend/src/api/remote.ts`:

```typescript
import type {
  RemoteServer,
  ConnectServerRequest,
  RemoteWorkspace,
  RemoteWorkspaceVersion,
  RemoteWorkspaceTag,
  CreateRemoteWorkspaceRequest,
  OCIRegistry,
  Job,
  User,
  AuditLog,
  DashboardStats,
} from '@/types';
```

**Step 3: Verify TypeScript compiles**

Run: `cd frontend && npx tsc --noEmit`
Expected: No errors

**Step 4: Commit**

```bash
git add frontend/src/api/remote.ts
git commit -m "feat: add remote admin API endpoints"
```

---

## Task 2: Add Remote Admin Hooks

**Files:**
- Modify: `frontend/src/hooks/useRemote.ts`

**Step 1: Add useRemoteUsers hook**

Add to `frontend/src/hooks/useRemote.ts`:

```typescript
export const useRemoteUsers = (enabled: boolean) => {
  return useQuery({
    queryKey: ['remote', 'admin', 'users'],
    queryFn: remoteApi.listUsers,
    enabled,
  });
};
```

**Step 2: Add useRemoteAdminRegistries hook**

```typescript
export const useRemoteAdminRegistries = (enabled: boolean) => {
  return useQuery({
    queryKey: ['remote', 'admin', 'registries'],
    queryFn: remoteApi.listAdminRegistries,
    enabled,
  });
};
```

**Step 3: Add useRemoteAuditLogs hook**

```typescript
export const useRemoteAuditLogs = (enabled: boolean, filters?: { user_id?: string; action?: string }) => {
  return useQuery({
    queryKey: ['remote', 'admin', 'audit-logs', filters],
    queryFn: () => remoteApi.listAuditLogs(filters),
    enabled,
  });
};
```

**Step 4: Add useRemoteDashboardStats hook**

```typescript
export const useRemoteDashboardStats = (enabled: boolean) => {
  return useQuery({
    queryKey: ['remote', 'admin', 'dashboard', 'stats'],
    queryFn: remoteApi.getDashboardStats,
    enabled,
  });
};
```

**Step 5: Verify TypeScript compiles**

Run: `cd frontend && npx tsc --noEmit`
Expected: No errors

**Step 6: Commit**

```bash
git add frontend/src/hooks/useRemote.ts
git commit -m "feat: add remote admin hooks"
```

---

## Task 3: Update UserManagement Page

**Files:**
- Modify: `frontend/src/pages/admin/UserManagement.tsx`

**Step 1: Add imports for view mode**

Add these imports to the top of the file:

```typescript
import { useModeStore } from '@/store/modeStore';
import { useViewModeStore } from '@/store/viewModeStore';
import { useRemoteServer, useRemoteUsers } from '@/hooks/useRemote';
```

**Step 2: Add view mode detection inside component**

Add after existing hooks (after `const currentUser = ...`):

```typescript
  // View mode support
  const isLocalMode = useModeStore((s) => s.isLocalMode());
  const viewMode = useViewModeStore((state) => state.viewMode);
  const { data: serverStatus } = useRemoteServer();
  const isRemoteConnected = isLocalMode && serverStatus?.status === 'connected';
  const { data: remoteUsers, isLoading: remoteLoading } = useRemoteUsers(isRemoteConnected && viewMode === 'remote');
```

**Step 3: Add useMemo for displayed users**

Add import at top:
```typescript
import { useState, useMemo } from 'react';
```

Add after the view mode detection:

```typescript
  // Show users based on view mode
  const displayedUsers = useMemo(() => {
    if (!isRemoteConnected) {
      return users || [];
    }
    if (viewMode === 'local') {
      return users || [];
    } else {
      return remoteUsers || [];
    }
  }, [users, remoteUsers, isRemoteConnected, viewMode]);
```

**Step 4: Update loading state**

Change the `isLoading` check:

```typescript
  const isLoading = usersLoading || (isRemoteConnected && viewMode === 'remote' && remoteLoading);
```

Note: Need to rename the existing `isLoading` from `useUsers()` to `usersLoading`:

```typescript
  const { data: users, isLoading: usersLoading } = useUsers();
```

**Step 5: Replace `users` with `displayedUsers` in render**

Change:
```typescript
{users?.map((user) => (
```
to:
```typescript
{displayedUsers.map((user) => (
```

And change:
```typescript
{users?.length === 0 && (
```
to:
```typescript
{displayedUsers.length === 0 && (
```

**Step 6: Verify TypeScript compiles**

Run: `cd frontend && npx tsc --noEmit`
Expected: No errors

**Step 7: Commit**

```bash
git add frontend/src/pages/admin/UserManagement.tsx
git commit -m "feat: add view mode support to UserManagement"
```

---

## Task 4: Update RegistryManagement Page

**Files:**
- Modify: `frontend/src/pages/admin/RegistryManagement.tsx`

**Step 1: Add imports for view mode**

Add these imports:

```typescript
import { useMemo } from 'react';
import { useModeStore } from '@/store/modeStore';
import { useViewModeStore } from '@/store/viewModeStore';
import { useRemoteServer, useRemoteAdminRegistries } from '@/hooks/useRemote';
```

**Step 2: Add view mode detection inside component**

Add after existing hooks:

```typescript
  // View mode support
  const isLocalMode = useModeStore((s) => s.isLocalMode());
  const viewMode = useViewModeStore((state) => state.viewMode);
  const { data: serverStatus } = useRemoteServer();
  const isRemoteConnected = isLocalMode && serverStatus?.status === 'connected';
  const shouldShowRemote = isRemoteConnected && viewMode === 'remote';
  const { data: remoteRegistries, isLoading: remoteLoading } = useRemoteAdminRegistries(shouldShowRemote);
```

**Step 3: Add useMemo for displayed registries**

```typescript
  // Show registries based on view mode
  const displayedRegistries = useMemo(() => {
    if (!isRemoteConnected) {
      return registries || [];
    }
    if (viewMode === 'local') {
      return registries || [];
    } else {
      return remoteRegistries || [];
    }
  }, [registries, remoteRegistries, isRemoteConnected, viewMode]);
```

**Step 4: Update loading state**

Rename existing `isLoading` to `registriesLoading`:

```typescript
  const { data: registries, isLoading: registriesLoading } = useRegistries();
```

Update loading check:

```typescript
  const isLoading = registriesLoading || (shouldShowRemote && remoteLoading);
```

**Step 5: Replace `registries` with `displayedRegistries` in render**

Change:
```typescript
{registries?.map((registry) => (
```
to:
```typescript
{displayedRegistries.map((registry) => (
```

And change:
```typescript
{registries?.length === 0 && (
```
to:
```typescript
{displayedRegistries.length === 0 && (
```

**Step 6: Verify TypeScript compiles**

Run: `cd frontend && npx tsc --noEmit`
Expected: No errors

**Step 7: Commit**

```bash
git add frontend/src/pages/admin/RegistryManagement.tsx
git commit -m "feat: add view mode support to RegistryManagement"
```

---

## Task 5: Update AuditLogs Page

**Files:**
- Modify: `frontend/src/pages/admin/AuditLogs.tsx`

**Step 1: Add imports for view mode**

Add these imports:

```typescript
import { useMemo } from 'react';
import { useModeStore } from '@/store/modeStore';
import { useViewModeStore } from '@/store/viewModeStore';
import { useRemoteServer, useRemoteAuditLogs } from '@/hooks/useRemote';
```

**Step 2: Add view mode detection inside component**

Add after the filters state:

```typescript
  // View mode support
  const isLocalMode = useModeStore((s) => s.isLocalMode());
  const viewMode = useViewModeStore((state) => state.viewMode);
  const { data: serverStatus } = useRemoteServer();
  const isRemoteConnected = isLocalMode && serverStatus?.status === 'connected';
  const shouldShowRemote = isRemoteConnected && viewMode === 'remote';
```

**Step 3: Update useAuditLogs and add remote hook**

Rename existing hook:

```typescript
  const { data: logs, isLoading: logsLoading } = useAuditLogs(
    filters.user_id || filters.action ? filters : undefined
  );

  const { data: remoteLogs, isLoading: remoteLoading } = useRemoteAuditLogs(
    shouldShowRemote,
    filters.user_id || filters.action ? filters : undefined
  );
```

**Step 4: Add useMemo for displayed logs**

```typescript
  // Show logs based on view mode
  const displayedLogs = useMemo(() => {
    if (!isRemoteConnected) {
      return logs || [];
    }
    if (viewMode === 'local') {
      return logs || [];
    } else {
      return remoteLogs || [];
    }
  }, [logs, remoteLogs, isRemoteConnected, viewMode]);
```

**Step 5: Update loading state**

```typescript
  const isLoading = logsLoading || (shouldShowRemote && remoteLoading);
```

**Step 6: Replace `logs` with `displayedLogs` in render**

Change:
```typescript
{logs?.map((log) => (
```
to:
```typescript
{displayedLogs.map((log) => (
```

And change:
```typescript
{logs?.length === 0 && (
```
to:
```typescript
{displayedLogs.length === 0 && (
```

**Step 7: Verify TypeScript compiles**

Run: `cd frontend && npx tsc --noEmit`
Expected: No errors

**Step 8: Commit**

```bash
git add frontend/src/pages/admin/AuditLogs.tsx
git commit -m "feat: add view mode support to AuditLogs"
```

---

## Task 6: Update AdminDashboard Page

**Files:**
- Modify: `frontend/src/pages/admin/AdminDashboard.tsx`

**Step 1: Add imports for view mode**

Add these imports:

```typescript
import { useMemo } from 'react';
import { useModeStore } from '@/store/modeStore';
import { useViewModeStore } from '@/store/viewModeStore';
import { useRemoteServer, useRemoteWorkspaces, useRemoteJobs, useRemoteDashboardStats } from '@/hooks/useRemote';
```

**Step 2: Add view mode detection inside component**

Add after existing hooks:

```typescript
  // View mode support
  const isLocalMode = useModeStore((s) => s.isLocalMode());
  const viewMode = useViewModeStore((state) => state.viewMode);
  const { data: serverStatus } = useRemoteServer();
  const isRemoteConnected = isLocalMode && serverStatus?.status === 'connected';
  const shouldShowRemote = isRemoteConnected && viewMode === 'remote';

  // Remote data
  const { data: remoteWorkspaces, isLoading: remoteWsLoading } = useRemoteWorkspaces(shouldShowRemote);
  const { data: remoteJobs, isLoading: remoteJobsLoading } = useRemoteJobs(shouldShowRemote);
  const { data: remoteDashboardStats, isLoading: remoteStatsLoading } = useRemoteDashboardStats(shouldShowRemote);
```

**Step 3: Add useMemo for displayed data**

```typescript
  // Select data based on view mode
  const displayedWorkspaces = useMemo(() => {
    if (!isRemoteConnected || viewMode === 'local') {
      return workspaces || [];
    }
    return remoteWorkspaces || [];
  }, [workspaces, remoteWorkspaces, isRemoteConnected, viewMode]);

  const displayedJobs = useMemo(() => {
    if (!isRemoteConnected || viewMode === 'local') {
      return jobs || [];
    }
    return remoteJobs || [];
  }, [jobs, remoteJobs, isRemoteConnected, viewMode]);

  const displayedStats = useMemo(() => {
    if (!isRemoteConnected || viewMode === 'local') {
      return dashboardStats;
    }
    return remoteDashboardStats;
  }, [dashboardStats, remoteDashboardStats, isRemoteConnected, viewMode]);
```

**Step 4: Update computed values**

Change:
```typescript
  const activeJobs =
    jobs?.filter(
      (job) => job.status === 'running' || job.status === 'pending',
    ).length || 0;

  const failedJobs =
    jobs?.filter((job) => job.status === 'failed').length || 0;
```
to:
```typescript
  const activeJobs =
    displayedJobs.filter(
      (job) => job.status === 'running' || job.status === 'pending',
    ).length;

  const failedJobs =
    displayedJobs.filter((job) => job.status === 'failed').length;
```

**Step 5: Update loading state**

Change:
```typescript
  if (usersLoading || wsLoading || jobsLoading || statsLoading) {
```
to:
```typescript
  const isLoading = usersLoading || wsLoading || jobsLoading || statsLoading ||
    (shouldShowRemote && (remoteWsLoading || remoteJobsLoading || remoteStatsLoading));

  if (isLoading) {
```

**Step 6: Update stat card values**

Change:
```typescript
        <StatCard
          title="Environments"
          value={workspaces?.length || 0}
          icon={Boxes}
        />
```
to:
```typescript
        <StatCard
          title="Environments"
          value={displayedWorkspaces.length}
          icon={Boxes}
        />
```

And change:
```typescript
        <StatCard
          title="Disk Usage"
          value={dashboardStats?.total_disk_usage_formatted || 'N/A'}
          icon={HardDrive}
        />
```
to:
```typescript
        <StatCard
          title="Disk Usage"
          value={displayedStats?.total_disk_usage_formatted || 'N/A'}
          icon={HardDrive}
        />
```

**Step 7: Verify TypeScript compiles**

Run: `cd frontend && npx tsc --noEmit`
Expected: No errors

**Step 8: Commit**

```bash
git add frontend/src/pages/admin/AdminDashboard.tsx
git commit -m "feat: add view mode support to AdminDashboard"
```

---

## Task 7: Final Verification

**Step 1: Run full TypeScript check**

Run: `cd frontend && npx tsc --noEmit`
Expected: No errors

**Step 2: Run build**

Run: `cd frontend && npm run build`
Expected: Build succeeds

**Step 3: Manual testing checklist**

Test with two servers running (see CLAUDE.md for setup):

1. Start local server: `NEBI_MODE=local go run ./cmd/nebi serve`
2. Start remote server: `NEBI_MODE=team NEBI_SERVER_PORT=8470 NEBI_DATABASE_DSN=./nebi-team.db ADMIN_USERNAME=admin ADMIN_PASSWORD=admin123 go run ./cmd/nebi serve`
3. Start frontend: `cd frontend && npm run dev`
4. Connect to remote server via Settings

Verify each admin page:
- [ ] AdminDashboard: stats change when switching Local/Remote
- [ ] UserManagement: user list changes when switching
- [ ] RegistryManagement: registry list changes when switching
- [ ] AuditLogs: logs change when switching

**Step 4: Commit any fixes if needed**

---

## Summary

This plan adds view mode support to all 4 admin pages by:
1. Adding remote API endpoints for admin data
2. Adding React Query hooks for remote admin data
3. Updating each page to detect view mode and switch data sources

The pattern matches what's already done in Workspaces, Registries, and Jobs pages.
