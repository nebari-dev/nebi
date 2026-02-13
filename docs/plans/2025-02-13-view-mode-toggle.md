# View Mode Toggle Implementation Plan

> **For Claude:** REQUIRED SUB-SKILL: Use superpowers:executing-plans to implement this plan task-by-task.

**Goal:** Add a global Local/Remote toggle to the header that controls which workspaces are displayed, only visible when connected to a server.

**Architecture:** New Zustand store (`viewModeStore`) with localStorage persistence manages `viewMode: 'local' | 'remote'`. The Layout component renders a segmented control when server is connected. Workspaces page conditionally fetches/displays based on viewMode instead of merging both lists.

**Tech Stack:** React, Zustand with persist middleware, shadcn/ui components, Tailwind CSS

---

### Task 1: Create viewModeStore

**Files:**
- Create: `frontend/src/store/viewModeStore.ts`

**Step 1: Create the store file**

```typescript
import { create } from 'zustand';
import { persist } from 'zustand/middleware';

type ViewMode = 'local' | 'remote';

interface ViewModeState {
  viewMode: ViewMode;
  setViewMode: (mode: ViewMode) => void;
}

export const useViewModeStore = create<ViewModeState>()(
  persist(
    (set) => ({
      viewMode: 'local',
      setViewMode: (mode) => set({ viewMode: mode }),
    }),
    { name: 'nebi-view-mode' }
  )
);
```

**Step 2: Verify TypeScript compiles**

Run: `cd frontend && npx tsc --noEmit`
Expected: No errors

**Step 3: Commit**

```bash
git add frontend/src/store/viewModeStore.ts
git commit -m "feat: add viewModeStore with localStorage persistence"
```

---

### Task 2: Add segmented control to Layout header

**Files:**
- Modify: `frontend/src/components/layout/Layout.tsx`

**Step 1: Add imports**

Add these imports at the top of Layout.tsx:

```typescript
import { useViewModeStore } from '@/store/viewModeStore';
import { useRemoteServer } from '@/hooks/useRemote';
import { HardDrive, Cloud } from 'lucide-react';
```

**Step 2: Add hooks and state inside Layout component**

After the existing hooks (around line 14), add:

```typescript
const { viewMode, setViewMode } = useViewModeStore();
const { data: serverStatus } = useRemoteServer();
const isRemoteConnected = isLocalMode && serverStatus?.status === 'connected';
```

**Step 3: Add segmented control in header**

Inside the header, after the closing `</nav>` tag (around line 79) and before the right-side user controls, add:

```typescript
{/* View Mode Toggle - only show when remote is connected */}
{isRemoteConnected && (
  <div className="flex items-center gap-1 p-1 bg-muted rounded-lg ml-4">
    <Button
      variant={viewMode === 'local' ? 'default' : 'ghost'}
      size="sm"
      onClick={() => setViewMode('local')}
      className="gap-1.5 h-7 px-3"
    >
      <HardDrive className="h-3.5 w-3.5" />
      Local
    </Button>
    <Button
      variant={viewMode === 'remote' ? 'default' : 'ghost'}
      size="sm"
      onClick={() => setViewMode('remote')}
      className="gap-1.5 h-7 px-3"
    >
      <Cloud className="h-3.5 w-3.5" />
      Remote
    </Button>
  </div>
)}
```

**Step 4: Verify TypeScript compiles**

Run: `cd frontend && npx tsc --noEmit`
Expected: No errors

**Step 5: Commit**

```bash
git add frontend/src/components/layout/Layout.tsx
git commit -m "feat: add view mode toggle to header when server connected"
```

---

### Task 3: Update Workspaces page to use viewMode

**Files:**
- Modify: `frontend/src/pages/Workspaces.tsx`

**Step 1: Add viewModeStore import**

Add this import near the top (around line 5):

```typescript
import { useViewModeStore } from '@/store/viewModeStore';
```

**Step 2: Add viewMode hook in component**

After line 84 (`const { data: remoteWorkspaces, isLoading: remoteLoading } = useRemoteWorkspaces(isRemoteConnected);`), add:

```typescript
const viewMode = useViewModeStore((state) => state.viewMode);
```

**Step 3: Replace unifiedWorkspaces logic**

Replace the entire `unifiedWorkspaces` useMemo (lines 104-141) with:

```typescript
// Filter workspaces based on view mode (when remote connected) or show all local (when not)
const displayedWorkspaces = useMemo<UnifiedWorkspace[]>(() => {
  // If not connected to remote, always show local workspaces
  if (!isRemoteConnected) {
    if (!workspaces) return [];
    return workspaces.map((ws) => ({
      id: ws.id,
      name: ws.name,
      status: ws.status,
      package_manager: ws.package_manager,
      created_at: ws.created_at,
      location: 'local' as const,
      source: ws.source,
      path: ws.path,
      owner_id: ws.owner_id,
      owner: ws.owner,
      size_formatted: ws.size_formatted,
    }));
  }

  // When connected, show based on viewMode
  if (viewMode === 'local') {
    if (!workspaces) return [];
    return workspaces.map((ws) => ({
      id: ws.id,
      name: ws.name,
      status: ws.status,
      package_manager: ws.package_manager,
      created_at: ws.created_at,
      location: 'local' as const,
      source: ws.source,
      path: ws.path,
      owner_id: ws.owner_id,
      owner: ws.owner,
      size_formatted: ws.size_formatted,
    }));
  } else {
    if (!remoteWorkspaces) return [];
    return remoteWorkspaces.map((ws) => ({
      id: ws.id,
      name: ws.name,
      status: ws.status,
      package_manager: ws.package_manager,
      created_at: ws.created_at,
      location: 'remote' as const,
      owner: ws.owner,
    }));
  }
}, [workspaces, remoteWorkspaces, isRemoteConnected, viewMode]);
```

**Step 4: Update all references from unifiedWorkspaces to displayedWorkspaces**

Find and replace `unifiedWorkspaces` with `displayedWorkspaces` in these locations:
- Line 270: loading condition
- Line 593: table map
- Line 680: empty state check

**Step 5: Remove redundant Local/Remote badges**

Remove lines 606-617 (the badge rendering for local/remote). The badges are no longer needed since we're filtering by mode. Delete this block:

```typescript
{isLocal && ws.location === 'local' && (
  <Badge variant="outline" className="bg-cyan-500/10 text-cyan-500 border-cyan-500/20 gap-1 text-xs">
    <HardDrive className="h-3 w-3" />
    Local
  </Badge>
)}
{isLocal && ws.location === 'remote' && (
  <Badge className="bg-purple-500/10 text-purple-500 border-purple-500/20 gap-1 text-xs">
    <Cloud className="h-3 w-3" />
    Remote
  </Badge>
)}
```

**Step 6: Update create form to respect viewMode**

In the create form, the `createTarget` state should default to the current `viewMode` when the form opens. Update the "New Workspace" button onClick (around line 410-414):

```typescript
<Button onClick={() => {
  setShowCreate(!showCreate);
  setCreateTarget(isRemoteConnected ? viewMode : 'local');
  setError('');
}}>
```

**Step 7: Verify TypeScript compiles**

Run: `cd frontend && npx tsc --noEmit`
Expected: No errors

**Step 8: Commit**

```bash
git add frontend/src/pages/Workspaces.tsx
git commit -m "feat: filter workspaces by view mode instead of merging"
```

---

### Task 4: Clean up unused imports

**Files:**
- Modify: `frontend/src/pages/Workspaces.tsx`

**Step 1: Remove unused HardDrive and Cloud imports**

Since we removed the badges, check if `HardDrive` and `Cloud` are still used elsewhere in the file (they are used in the create form toggle). If `HardDrive` is not used anywhere else, remove it from the import. `Cloud` is still used for the Server button in create form.

After review: Keep both imports as they're used in the Local/Server target tabs in the create form.

**Step 2: Verify TypeScript compiles**

Run: `cd frontend && npx tsc --noEmit`
Expected: No errors

**Step 3: Commit (if changes made)**

```bash
git add frontend/src/pages/Workspaces.tsx
git commit -m "refactor: clean up unused imports"
```

---

### Task 5: Manual testing verification

**Step 1: Start development server**

Run: `cd /home/balast/eph/nebi/.worktrees/view-mode-toggle && ADMIN_USERNAME=admin ADMIN_PASSWORD=testpass make dev`

**Step 2: Test scenarios**

1. **Without server connection:**
   - Navigate to Workspaces page
   - Verify toggle is NOT visible in header
   - Verify local workspaces display normally

2. **With server connection:**
   - Go to Settings, connect to a remote server
   - Verify toggle appears in header with "Local" and "Remote" buttons
   - Click "Local" - verify only local workspaces show
   - Click "Remote" - verify only remote workspaces show

3. **Persistence:**
   - Select "Remote" mode
   - Refresh the page
   - Verify "Remote" is still selected

4. **Create form:**
   - In Local mode, click "New Workspace"
   - Verify "Local" tab is selected by default
   - Switch to Remote mode in header
   - Click "New Workspace" again
   - Verify "Server" tab is selected by default

**Step 3: Verify build succeeds**

Run: `cd frontend && npm run build`
Expected: Build completes without errors

---

### Task 6: Final commit and branch summary

**Step 1: Verify all changes are committed**

Run: `git status`
Expected: Clean working tree

**Step 2: Review commit history**

Run: `git log --oneline main..HEAD`
Expected: 3-4 commits showing the feature implementation

**Step 3: Push branch (if ready for review)**

Run: `git push -u origin feature/view-mode-toggle`
