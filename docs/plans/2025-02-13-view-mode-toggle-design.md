# Local/Remote View Mode Toggle Design

**Date:** 2025-02-13
**Status:** Approved

## Problem

The Workspaces page currently displays local and remote workspaces together in a unified list, distinguished by colored badges. This becomes confusing as users work primarily in one context or the other. Additionally, registries will eventually need similar local/remote separation.

## Solution

Add a global view mode toggle to the header that controls whether the user sees local or remote resources across all pages. The toggle only appears when a server connection exists.

## Requirements

1. **Global toggle in header** - Single toggle controls view mode across all pages (workspaces now, registries later)
2. **Segmented control UI** - Two pill buttons `[Local | Remote]` with active state highlighted
3. **Only visible when connected** - Hide toggle entirely when no server connection exists
4. **Persist selection** - Remember user's choice in localStorage across sessions
5. **Default to Local** - First-time users start in Local mode

## Architecture

### New Store: `viewModeStore.ts`

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

### Header Component Changes (`Layout.tsx`)

- Add segmented control between nav links and right-side user controls
- Only render when `serverStatus?.status === 'connected'`
- Read/write from `useViewModeStore`

### Workspaces Page Changes (`Workspaces.tsx`)

- Read `viewMode` from store
- When `viewMode === 'local'`: only call `useWorkspaces()`, display local workspaces
- When `viewMode === 'remote'`: only call `useRemoteWorkspaces()`, display remote workspaces
- Remove the unified list merging logic (`unifiedWorkspaces` useMemo)
- Remove Local/Remote badges (now redundant)

## Data Flow

```
User clicks toggle in header
       ↓
viewModeStore.setViewMode() called
       ↓
State persists to localStorage
       ↓
Workspaces.tsx re-renders, reads new viewMode
       ↓
Conditionally fetches from useWorkspaces() OR useRemoteWorkspaces()
       ↓
Renders single-source workspace list (no merging)
```

## UI Behavior Matrix

| Condition | Toggle Visible | Behavior |
|-----------|---------------|----------|
| No server configured | No | Always shows local workspaces |
| Server configured but disconnected | No | Always shows local workspaces |
| Server connected | Yes | Shows workspaces based on toggle selection |

## Visual Design

The segmented control should:
- Sit in the header, visually separated from nav links
- Use the existing design system colors (likely primary color for active segment)
- Be compact but clearly readable
- Match the overall header aesthetic

## Future Considerations

- Registries page will use the same `viewModeStore` to filter local vs remote registries
- Other pages may adopt this pattern as needed
- The store is intentionally simple to allow easy extension
