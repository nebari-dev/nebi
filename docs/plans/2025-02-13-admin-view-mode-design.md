# Admin Pages View Mode Support

**Date:** 2025-02-13
**Status:** Approved

## Summary

Add local/remote view mode toggle support to all Admin pages (Dashboard, Users, Registries, Audit Logs), following the same pattern already used in Workspaces, Registries, and Jobs pages.

## Background

The view mode toggle (Local/Remote) appears in the header when the desktop app is connected to a remote server. Currently, Workspaces, Registries, and Jobs pages respect this toggle and show data from either the local server or the remote server accordingly.

The Admin pages (Dashboard, Users, Registries management, Audit Logs) do not currently respect the view mode toggle. This creates an inconsistent experience where switching to "Remote" mode still shows local admin data.

## Design

### Approach

Follow the existing pattern from Workspaces/Registries/Jobs pages:
1. Each page imports view mode stores and `useRemoteServer`
2. Fetch both local and remote data (remote only when connected)
3. Display data based on `viewMode` when `isRemoteConnected`

No new shared abstractions — keep it explicit and consistent with existing code.

### Changes Required

#### 1. Remote API (`api/remote.ts`)

Add authenticated endpoints for admin data:

```typescript
// Admin endpoints
getUsers: () => remoteClient.get('/api/v1/admin/users')
getAdminRegistries: () => remoteClient.get('/api/v1/admin/registries')
getAuditLogs: (filters?) => remoteClient.get('/api/v1/admin/audit-logs', { params: filters })
getDashboardStats: () => remoteClient.get('/api/v1/admin/dashboard')
```

#### 2. Remote Hooks (`hooks/useRemote.ts`)

Add hooks that mirror the local admin hooks:

```typescript
export function useRemoteUsers(enabled: boolean)
export function useRemoteAdminRegistries(enabled: boolean)
export function useRemoteAuditLogs(enabled: boolean, filters?: AuditLogFilters)
export function useRemoteDashboardStats(enabled: boolean)
```

#### 3. Admin Pages

**AdminDashboard.tsx:**
- Add view mode detection (isLocalMode, viewMode, serverStatus, isRemoteConnected)
- Add `useRemoteWorkspaces`, `useRemoteJobs`, `useRemoteDashboardStats` calls
- Switch displayed stats based on `shouldShowRemote`

**UserManagement.tsx:**
- Add view mode detection
- Add `useRemoteUsers` hook call
- Switch displayed users based on view mode

**RegistryManagement.tsx:**
- Add view mode detection
- Add `useRemoteAdminRegistries` hook call
- Switch displayed registries based on view mode
- Note: CRUD operations should target the correct server based on view mode

**AuditLogs.tsx:**
- Add view mode detection
- Add `useRemoteAuditLogs` hook call
- Switch displayed logs based on view mode

### Pattern Reference

From `Workspaces.tsx`:

```typescript
const isLocal = useModeStore((state) => state.mode === 'local');
const { data: serverStatus } = useRemoteServer();
const isRemoteConnected = isLocal && serverStatus?.status === 'connected';
const { data: remoteWorkspaces, isLoading: remoteLoading } = useRemoteWorkspaces(isRemoteConnected);
const viewMode = useViewModeStore((state) => state.viewMode);

const displayedWorkspaces = useMemo(() => {
  if (!isRemoteConnected) {
    return workspaces || [];
  }
  if (viewMode === 'local') {
    return workspaces || [];
  } else {
    return remoteWorkspaces || [];
  }
}, [workspaces, remoteWorkspaces, isRemoteConnected, viewMode]);
```

## Out of Scope

- No changes to backend API (assuming admin endpoints already exist on remote server)
- No new shared utility hooks or abstractions
- No changes to the view mode toggle UI itself

## Testing

1. Start local server (port 8460)
2. Start remote server (port 8470 with separate database)
3. Connect to remote server via Settings
4. Verify each admin page:
   - In Local mode: shows local server data
   - In Remote mode: shows remote server data
5. Verify CRUD operations target the correct server
