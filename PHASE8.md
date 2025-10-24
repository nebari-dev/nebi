# Phase 8: Admin UI & Environment Sharing UI

## Overview

Phase 8 builds the React frontend for admin functionality and environment sharing. This includes user management, permission management, audit logs, and the ability for environment owners to share their environments with collaborators.

## Current State (Phase 7 Complete ✅)

**Backend Complete:**
- ✅ Full RBAC with Casbin
- ✅ Admin API endpoints for user/role/permission management
- ✅ Audit logging system
- ✅ Environment sharing API (owner can share with viewer/editor roles)
- ✅ Collaborator management endpoints

**Frontend Complete (Phase 4):**
- ✅ Basic React app with authentication
- ✅ Environment list and management
- ✅ Package installation
- ✅ Job tracking

**What's Missing:**
- ❌ Admin dashboard UI
- ❌ User management interface
- ❌ Permission management interface
- ❌ Audit log viewer
- ❌ Environment sharing UI

## Phase 8 Goals

1. **Admin Dashboard** - Overview, stats, quick actions
2. **User Management UI** - List, create, edit, delete users, toggle admin
3. **Audit Log Viewer** - Searchable, filterable log interface
4. **Environment Sharing UI** - Share button, collaborator management
5. **Permission Indicators** - Show user's role on each environment

---

## Architecture

### Component Structure

```
frontend/src/
├── pages/
│   ├── admin/
│   │   ├── AdminDashboard.tsx       # Overview with stats
│   │   ├── UserManagement.tsx       # User CRUD
│   │   ├── AuditLogs.tsx            # Log viewer
│   │   └── PermissionMatrix.tsx     # (Optional) Permission overview
│   └── environments/
│       ├── EnvironmentList.tsx      # (Update) Add share button
│       ├── EnvironmentDetail.tsx    # (Update) Add collaborators tab
│       └── ShareDialog.tsx          # Share modal
├── components/
│   ├── admin/
│   │   ├── UserTable.tsx
│   │   ├── CreateUserDialog.tsx
│   │   ├── AuditLogTable.tsx
│   │   └── UserRoleBadge.tsx
│   └── sharing/
│       ├── CollaboratorList.tsx
│       ├── ShareButton.tsx
│       ├── AddCollaboratorDialog.tsx
│       └── RoleSelector.tsx
└── hooks/
    ├── useUsers.ts
    ├── useAuditLogs.ts
    └── useCollaborators.ts
```

---

## Implementation Plan

### Step 1: Admin Dashboard (/admin)

**Goal:** Provide an overview for admins with key metrics and quick actions.

**Features:**
- Total users count
- Total environments count
- Recent audit log entries (last 10)
- Quick links to user management, audit logs

**API Endpoints Used:**
- `GET /api/v1/admin/users` - Count users
- `GET /api/v1/environments` - Count environments (admin sees all)
- `GET /api/v1/admin/audit-logs?limit=10` - Recent activity

**Implementation:**

```tsx
// src/pages/admin/AdminDashboard.tsx
import { useQuery } from '@tanstack/react-query';
import { api } from '@/lib/api';

export function AdminDashboard() {
  const { data: users } = useQuery({
    queryKey: ['admin', 'users'],
    queryFn: () => api.get('/admin/users'),
  });

  const { data: auditLogs } = useQuery({
    queryKey: ['admin', 'audit-logs'],
    queryFn: () => api.get('/admin/audit-logs'),
  });

  return (
    <div className="p-6">
      <h1 className="text-2xl font-bold mb-6">Admin Dashboard</h1>

      {/* Stats Cards */}
      <div className="grid grid-cols-3 gap-4 mb-8">
        <StatCard title="Total Users" value={users?.length || 0} />
        <StatCard title="Environments" value={environments?.length || 0} />
        <StatCard title="Active Jobs" value={activeJobs || 0} />
      </div>

      {/* Recent Activity */}
      <div className="bg-white rounded-lg shadow p-6">
        <h2 className="text-lg font-semibold mb-4">Recent Activity</h2>
        <AuditLogTable logs={auditLogs?.slice(0, 10)} compact />
      </div>

      {/* Quick Actions */}
      <div className="mt-6 flex gap-4">
        <Link to="/admin/users">
          <Button>Manage Users</Button>
        </Link>
        <Link to="/admin/audit-logs">
          <Button variant="outline">View All Logs</Button>
        </Link>
      </div>
    </div>
  );
}
```

---

### Step 2: User Management UI (/admin/users)

**Goal:** Allow admins to manage users - create, list, toggle admin, delete.

**Features:**
- Table showing username, email, admin status
- "Create User" button
- "Toggle Admin" button for each user
- "Delete" button for each user
- Admin badge indicator

**API Endpoints Used:**
- `GET /api/v1/admin/users`
- `POST /api/v1/admin/users`
- `POST /api/v1/admin/users/:id/toggle-admin`
- `DELETE /api/v1/admin/users/:id`

**Implementation:**

```tsx
// src/pages/admin/UserManagement.tsx
import { useQuery, useMutation, useQueryClient } from '@tanstack/react-query';
import { api } from '@/lib/api';
import { CreateUserDialog } from '@/components/admin/CreateUserDialog';

export function UserManagement() {
  const queryClient = useQueryClient();

  const { data: users, isLoading } = useQuery({
    queryKey: ['admin', 'users'],
    queryFn: () => api.get('/admin/users'),
  });

  const toggleAdminMutation = useMutation({
    mutationFn: (userId: string) =>
      api.post(`/admin/users/${userId}/toggle-admin`),
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: ['admin', 'users'] });
    },
  });

  const deleteUserMutation = useMutation({
    mutationFn: (userId: string) =>
      api.delete(`/admin/users/${userId}`),
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: ['admin', 'users'] });
    },
  });

  return (
    <div className="p-6">
      <div className="flex justify-between items-center mb-6">
        <h1 className="text-2xl font-bold">User Management</h1>
        <CreateUserDialog />
      </div>

      {/* User Table */}
      <div className="bg-white rounded-lg shadow overflow-hidden">
        <table className="min-w-full divide-y divide-gray-200">
          <thead className="bg-gray-50">
            <tr>
              <th>Username</th>
              <th>Email</th>
              <th>Role</th>
              <th>Created</th>
              <th>Actions</th>
            </tr>
          </thead>
          <tbody>
            {users?.map((user) => (
              <tr key={user.id}>
                <td>{user.username}</td>
                <td>{user.email}</td>
                <td>
                  <UserRoleBadge isAdmin={user.is_admin} />
                </td>
                <td>{formatDate(user.created_at)}</td>
                <td className="flex gap-2">
                  <Button
                    size="sm"
                    variant="outline"
                    onClick={() => toggleAdminMutation.mutate(user.id)}
                  >
                    {user.is_admin ? 'Revoke Admin' : 'Make Admin'}
                  </Button>
                  <Button
                    size="sm"
                    variant="destructive"
                    onClick={() => {
                      if (confirm('Delete this user?')) {
                        deleteUserMutation.mutate(user.id);
                      }
                    }}
                  >
                    Delete
                  </Button>
                </td>
              </tr>
            ))}
          </tbody>
        </table>
      </div>
    </div>
  );
}

// src/components/admin/CreateUserDialog.tsx
export function CreateUserDialog() {
  const [open, setOpen] = useState(false);
  const queryClient = useQueryClient();

  const createMutation = useMutation({
    mutationFn: (data: CreateUserRequest) =>
      api.post('/admin/users', data),
    onSuccess: () => {
      setOpen(false);
      queryClient.invalidateQueries({ queryKey: ['admin', 'users'] });
    },
  });

  return (
    <Dialog open={open} onOpenChange={setOpen}>
      <DialogTrigger asChild>
        <Button>Create User</Button>
      </DialogTrigger>
      <DialogContent>
        <DialogHeader>
          <DialogTitle>Create New User</DialogTitle>
        </DialogHeader>
        <form onSubmit={handleSubmit}>
          <Input name="username" label="Username" required />
          <Input name="email" type="email" label="Email" required />
          <Input name="password" type="password" label="Password" required />
          <Checkbox name="is_admin" label="Make Admin" />
          <Button type="submit">Create</Button>
        </form>
      </DialogContent>
    </Dialog>
  );
}
```

---

### Step 3: Audit Log Viewer (/admin/audit-logs)

**Goal:** Display audit logs with filtering and search capabilities.

**Features:**
- Table showing timestamp, user, action, resource, details
- Filter by user
- Filter by action type
- Search by resource

**API Endpoints Used:**
- `GET /api/v1/admin/audit-logs?user_id=...&action=...`

**Implementation:**

```tsx
// src/pages/admin/AuditLogs.tsx
import { useQuery } from '@tanstack/react-query';
import { api } from '@/lib/api';

export function AuditLogs() {
  const [filters, setFilters] = useState({
    user_id: '',
    action: '',
  });

  const { data: logs, isLoading } = useQuery({
    queryKey: ['admin', 'audit-logs', filters],
    queryFn: () => {
      const params = new URLSearchParams();
      if (filters.user_id) params.set('user_id', filters.user_id);
      if (filters.action) params.set('action', filters.action);
      return api.get(`/admin/audit-logs?${params}`);
    },
  });

  return (
    <div className="p-6">
      <h1 className="text-2xl font-bold mb-6">Audit Logs</h1>

      {/* Filters */}
      <div className="mb-4 flex gap-4">
        <Select
          value={filters.action}
          onChange={(action) => setFilters({ ...filters, action })}
          placeholder="Filter by action"
        >
          <option value="">All Actions</option>
          <option value="create_user">Create User</option>
          <option value="delete_user">Delete User</option>
          <option value="grant_permission">Grant Permission</option>
          <option value="revoke_permission">Revoke Permission</option>
          <option value="make_admin">Make Admin</option>
          <option value="revoke_admin">Revoke Admin</option>
        </Select>

        <Input
          placeholder="Filter by user ID"
          value={filters.user_id}
          onChange={(e) => setFilters({ ...filters, user_id: e.target.value })}
        />
      </div>

      {/* Audit Log Table */}
      <div className="bg-white rounded-lg shadow overflow-hidden">
        <table className="min-w-full divide-y divide-gray-200">
          <thead className="bg-gray-50">
            <tr>
              <th>Timestamp</th>
              <th>User</th>
              <th>Action</th>
              <th>Resource</th>
              <th>Details</th>
            </tr>
          </thead>
          <tbody>
            {logs?.map((log) => (
              <tr key={log.id}>
                <td>{formatDateTime(log.timestamp)}</td>
                <td>{log.user?.username || log.user_id}</td>
                <td>
                  <ActionBadge action={log.action} />
                </td>
                <td className="font-mono text-sm">{log.resource}</td>
                <td>
                  <DetailsPopover details={log.details_json} />
                </td>
              </tr>
            ))}
          </tbody>
        </table>
      </div>
    </div>
  );
}

// src/components/admin/ActionBadge.tsx
export function ActionBadge({ action }: { action: string }) {
  const colors = {
    create_user: 'bg-green-100 text-green-800',
    delete_user: 'bg-red-100 text-red-800',
    grant_permission: 'bg-blue-100 text-blue-800',
    revoke_permission: 'bg-orange-100 text-orange-800',
    make_admin: 'bg-purple-100 text-purple-800',
    revoke_admin: 'bg-gray-100 text-gray-800',
  };

  return (
    <span className={`px-2 py-1 rounded text-xs font-medium ${colors[action] || 'bg-gray-100 text-gray-800'}`}>
      {action.replace(/_/g, ' ')}
    </span>
  );
}
```

---

### Step 4: Environment Sharing UI

**Goal:** Allow environment owners to share their environments with other users.

**Features:**
- "Share" button on environment cards (owner only)
- Collaborators tab in environment detail view
- Add collaborator dialog with user search and role selection
- Remove collaborator button
- Role badge indicators (owner, editor, viewer)

**API Endpoints Used:**
- `GET /api/v1/environments/:id/collaborators`
- `POST /api/v1/environments/:id/share`
- `DELETE /api/v1/environments/:id/share/:user_id`

**Implementation:**

#### 4.1 Update Environment Card

```tsx
// src/components/environments/EnvironmentCard.tsx
export function EnvironmentCard({ environment, currentUser }) {
  const isOwner = environment.owner_id === currentUser.id;

  return (
    <div className="bg-white rounded-lg shadow p-4">
      <div className="flex justify-between items-start">
        <div>
          <h3 className="font-semibold">{environment.name}</h3>
          <RoleBadge role={getUserRole(environment, currentUser)} />
        </div>
        <div className="flex gap-2">
          {isOwner && (
            <ShareButton environmentId={environment.id} />
          )}
          <Link to={`/environments/${environment.id}`}>
            <Button variant="outline" size="sm">View</Button>
          </Link>
        </div>
      </div>
    </div>
  );
}
```

#### 4.2 Share Button & Dialog

```tsx
// src/components/sharing/ShareButton.tsx
export function ShareButton({ environmentId }: { environmentId: string }) {
  const [open, setOpen] = useState(false);

  return (
    <>
      <Button onClick={() => setOpen(true)} size="sm" variant="outline">
        <Users className="w-4 h-4 mr-2" />
        Share
      </Button>
      <ShareDialog
        open={open}
        onOpenChange={setOpen}
        environmentId={environmentId}
      />
    </>
  );
}

// src/components/sharing/ShareDialog.tsx
export function ShareDialog({ open, onOpenChange, environmentId }) {
  const queryClient = useQueryClient();

  const { data: collaborators } = useQuery({
    queryKey: ['collaborators', environmentId],
    queryFn: () => api.get(`/environments/${environmentId}/collaborators`),
    enabled: open,
  });

  const { data: allUsers } = useQuery({
    queryKey: ['admin', 'users'],
    queryFn: () => api.get('/admin/users'),
    enabled: open,
  });

  const shareMutation = useMutation({
    mutationFn: ({ user_id, role }: { user_id: string; role: string }) =>
      api.post(`/environments/${environmentId}/share`, { user_id, role }),
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: ['collaborators', environmentId] });
    },
  });

  const unshareMutation = useMutation({
    mutationFn: (userId: string) =>
      api.delete(`/environments/${environmentId}/share/${userId}`),
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: ['collaborators', environmentId] });
    },
  });

  const availableUsers = allUsers?.filter(
    (user) => !collaborators?.some((c) => c.user_id === user.id)
  );

  return (
    <Dialog open={open} onOpenChange={onOpenChange}>
      <DialogContent className="max-w-2xl">
        <DialogHeader>
          <DialogTitle>Share Environment</DialogTitle>
          <DialogDescription>
            Manage who has access to this environment
          </DialogDescription>
        </DialogHeader>

        {/* Current Collaborators */}
        <div className="space-y-2">
          <h3 className="font-semibold text-sm">Current Access</h3>
          {collaborators?.map((collab) => (
            <div key={collab.user_id} className="flex justify-between items-center p-2 border rounded">
              <div>
                <div className="font-medium">{collab.username}</div>
                <div className="text-sm text-gray-500">{collab.email}</div>
              </div>
              <div className="flex items-center gap-2">
                <RoleBadge role={collab.role} />
                {!collab.is_owner && (
                  <Button
                    size="sm"
                    variant="ghost"
                    onClick={() => {
                      if (confirm(`Remove ${collab.username}?`)) {
                        unshareMutation.mutate(collab.user_id);
                      }
                    }}
                  >
                    <X className="w-4 h-4" />
                  </Button>
                )}
              </div>
            </div>
          ))}
        </div>

        {/* Add Collaborator */}
        <div className="space-y-2">
          <h3 className="font-semibold text-sm">Add Collaborator</h3>
          <AddCollaboratorForm
            users={availableUsers}
            onSubmit={(user_id, role) => {
              shareMutation.mutate({ user_id, role });
            }}
          />
        </div>
      </DialogContent>
    </Dialog>
  );
}

// src/components/sharing/AddCollaboratorForm.tsx
export function AddCollaboratorForm({ users, onSubmit }) {
  const [selectedUser, setSelectedUser] = useState('');
  const [selectedRole, setSelectedRole] = useState('viewer');

  const handleSubmit = (e) => {
    e.preventDefault();
    if (selectedUser) {
      onSubmit(selectedUser, selectedRole);
      setSelectedUser('');
      setSelectedRole('viewer');
    }
  };

  return (
    <form onSubmit={handleSubmit} className="flex gap-2">
      <Select
        value={selectedUser}
        onChange={setSelectedUser}
        placeholder="Select user..."
        className="flex-1"
      >
        {users?.map((user) => (
          <option key={user.id} value={user.id}>
            {user.username} ({user.email})
          </option>
        ))}
      </Select>

      <Select value={selectedRole} onChange={setSelectedRole}>
        <option value="viewer">Viewer (Read-only)</option>
        <option value="editor">Editor (Can modify)</option>
      </Select>

      <Button type="submit" disabled={!selectedUser}>
        Add
      </Button>
    </form>
  );
}
```

#### 4.3 Role Badge Component

```tsx
// src/components/sharing/RoleBadge.tsx
export function RoleBadge({ role }: { role: string }) {
  const styles = {
    owner: 'bg-purple-100 text-purple-800 border-purple-200',
    editor: 'bg-blue-100 text-blue-800 border-blue-200',
    viewer: 'bg-gray-100 text-gray-800 border-gray-200',
  };

  const icons = {
    owner: '👑',
    editor: '✏️',
    viewer: '👁️',
  };

  return (
    <span className={`inline-flex items-center gap-1 px-2 py-1 rounded text-xs font-medium border ${styles[role]}`}>
      <span>{icons[role]}</span>
      {role}
    </span>
  );
}
```

---

### Step 5: Update Environment Detail Page

Add a "Collaborators" tab to show who has access:

```tsx
// src/pages/environments/EnvironmentDetail.tsx
export function EnvironmentDetail() {
  const { id } = useParams();
  const [activeTab, setActiveTab] = useState('overview');

  const { data: environment } = useQuery({
    queryKey: ['environments', id],
    queryFn: () => api.get(`/environments/${id}`),
  });

  const { data: collaborators } = useQuery({
    queryKey: ['collaborators', id],
    queryFn: () => api.get(`/environments/${id}/collaborators`),
  });

  return (
    <div className="p-6">
      <div className="flex justify-between items-center mb-6">
        <h1 className="text-2xl font-bold">{environment?.name}</h1>
        {isOwner && <ShareButton environmentId={id} />}
      </div>

      {/* Tabs */}
      <Tabs value={activeTab} onValueChange={setActiveTab}>
        <TabsList>
          <TabsTrigger value="overview">Overview</TabsTrigger>
          <TabsTrigger value="packages">Packages</TabsTrigger>
          <TabsTrigger value="collaborators">
            Collaborators ({collaborators?.length || 0})
          </TabsTrigger>
          <TabsTrigger value="settings">Settings</TabsTrigger>
        </TabsList>

        <TabsContent value="overview">
          {/* Existing overview content */}
        </TabsContent>

        <TabsContent value="packages">
          {/* Existing packages content */}
        </TabsContent>

        <TabsContent value="collaborators">
          <CollaboratorList collaborators={collaborators} />
        </TabsContent>

        <TabsContent value="settings">
          {/* Settings content */}
        </TabsContent>
      </Tabs>
    </div>
  );
}
```

---

## Routing Updates

Add admin routes to your router:

```tsx
// src/App.tsx or src/router.tsx
import { AdminDashboard } from './pages/admin/AdminDashboard';
import { UserManagement } from './pages/admin/UserManagement';
import { AuditLogs } from './pages/admin/AuditLogs';

// Protected Routes
<Route element={<ProtectedRoute />}>
  <Route path="/environments" element={<EnvironmentList />} />
  <Route path="/environments/:id" element={<EnvironmentDetail />} />

  {/* Admin Routes */}
  <Route element={<AdminRoute />}> {/* Check if user is admin */}
    <Route path="/admin" element={<AdminDashboard />} />
    <Route path="/admin/users" element={<UserManagement />} />
    <Route path="/admin/audit-logs" element={<AuditLogs />} />
  </Route>
</Route>

// AdminRoute component
function AdminRoute() {
  const { user } = useAuth();

  const { data: isAdmin } = useQuery({
    queryKey: ['user', 'is_admin'],
    queryFn: async () => {
      // Check if user can access admin endpoint
      try {
        await api.get('/admin/users');
        return true;
      } catch {
        return false;
      }
    },
  });

  if (!isAdmin) {
    return <Navigate to="/environments" replace />;
  }

  return <Outlet />;
}
```

---

## Navigation Updates

Add admin links to the navigation:

```tsx
// src/components/Layout/Navigation.tsx
export function Navigation() {
  const { user } = useAuth();
  const { data: isAdmin } = useIsAdmin();

  return (
    <nav>
      <NavLink to="/environments">Environments</NavLink>
      <NavLink to="/jobs">Jobs</NavLink>

      {isAdmin && (
        <NavLink to="/admin">
          <Shield className="w-4 h-4 mr-2" />
          Admin
        </NavLink>
      )}

      <UserMenu user={user} />
    </nav>
  );
}
```

---

## Testing Checklist

### Admin UI Tests

- [ ] Admin dashboard displays correct stats
- [ ] Non-admin users cannot access `/admin` routes
- [ ] Can create new user with username, email, password
- [ ] Can toggle user to/from admin
- [ ] Can delete user (with confirmation)
- [ ] Cannot delete yourself
- [ ] Audit logs display correctly
- [ ] Can filter audit logs by action
- [ ] Can filter audit logs by user

### Sharing UI Tests

- [ ] "Share" button only visible to environment owner
- [ ] Share dialog shows current collaborators
- [ ] Owner is marked with "owner" badge
- [ ] Can add collaborator with viewer role
- [ ] Can add collaborator with editor role
- [ ] Viewer can read but not write
- [ ] Editor can read and write
- [ ] Can remove collaborator
- [ ] Cannot remove owner
- [ ] Collaborators tab shows correct count
- [ ] Role badges display correctly

---

## API Integration Summary

### Admin Endpoints

```typescript
// User Management
GET    /api/v1/admin/users                 // List all users
POST   /api/v1/admin/users                 // Create user
GET    /api/v1/admin/users/:id             // Get user details
POST   /api/v1/admin/users/:id/toggle-admin // Toggle admin status
DELETE /api/v1/admin/users/:id             // Delete user

// Role Management
GET    /api/v1/admin/roles                 // List all roles

// Permission Management (Admin-level)
GET    /api/v1/admin/permissions           // List all permissions
POST   /api/v1/admin/permissions           // Grant permission (admin)
DELETE /api/v1/admin/permissions/:id       // Revoke permission

// Audit Logs
GET    /api/v1/admin/audit-logs?user_id=...&action=...
```

### Sharing Endpoints (Owner-level)

```typescript
// Environment Sharing
GET    /api/v1/environments/:id/collaborators     // List collaborators
POST   /api/v1/environments/:id/share             // Share with user
       Body: { user_id: string, role: "viewer" | "editor" }
DELETE /api/v1/environments/:id/share/:user_id    // Unshare

// Note: Only environment owner can use these endpoints
```

---

## UI/UX Guidelines

### Design Principles

1. **Clear Role Indicators**: Use distinct badges for owner/editor/viewer
2. **Confirm Destructive Actions**: Always confirm delete/revoke actions
3. **Real-time Updates**: Use React Query to auto-refresh after mutations
4. **Permission-aware UI**: Hide buttons for actions user cannot perform
5. **Audit Trail Transparency**: Show all actions in audit logs

### Color Scheme (Tailwind)

```
Owner:  purple-100/purple-800 👑
Editor: blue-100/blue-800     ✏️
Viewer: gray-100/gray-800     👁️
Admin:  red-100/red-800       🛡️

Actions:
Create:  green-100/green-800
Delete:  red-100/red-800
Modify:  blue-100/blue-800
Revoke:  orange-100/orange-800
```

---

## Acceptance Criteria

Phase 8 is complete when:

- [ ] Admin dashboard displays with stats and recent activity
- [ ] Admin can create, list, and delete users
- [ ] Admin can toggle admin status for users
- [ ] Audit log viewer displays all logs with filtering
- [ ] Environment owner can share via "Share" button
- [ ] Share dialog shows current collaborators
- [ ] Can add collaborator with role selection (viewer/editor)
- [ ] Can remove collaborators
- [ ] Collaborators tab shows all users with access
- [ ] Role badges display correctly everywhere
- [ ] Non-admins cannot access admin routes
- [ ] Non-owners cannot share environments
- [ ] All actions are audited and visible in audit logs

---

## Next Steps: Phase 9

Once Phase 8 is complete, Phase 9 will focus on:
- Embedding the React frontend in the Go binary
- Single binary deployment
- Docker images and Kubernetes manifests
- Multi-platform releases

The backend is fully ready to support Phase 8! 🚀
