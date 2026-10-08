# Nebi Frontend

Modern React frontend for the Nebi environment management system.

## Tech Stack

- **React 19** with TypeScript
- **Vite** - Fast build tool and dev server
- **Tailwind CSS v4** - Utility-first CSS framework
- **shadcn/ui** - Beautiful, accessible component library
- **TanStack Query (React Query)** - Powerful server state management
- **Zustand** - Lightweight client state for authentication
- **React Router v7** - Client-side routing
- **Axios** - HTTP client
- **Lucide React** - Icon library

## Features

- ✅ Team server authentication with JWT; local client login bypass
- ✅ Environment management (create, view, delete)
- ✅ Package installation and removal
- ✅ Real-time job status updates (2-second polling)
- ✅ Job log viewing
- ✅ Responsive design with Tailwind CSS
- ✅ Clean, minimal UI with shadcn components

## Project Structure

```text
frontend/
├── apps/
│   ├── client/           # Local browser / desktop app → dist/client
│   └── server/           # Team server app → dist/server
├── packages/
│   └── ui/               # Presentational components, theme, and styles
├── public/               # Assets shared by both builds
├── e2e/                  # Browser and accessibility tests
├── package.json          # npm workspaces and root commands
└── package-lock.json     # One dependency lockfile
```

Each app owns its routing, API clients, data hooks, stores, and domain types
under `src/`. `@nebi/ui` takes props and emits events; it
must not import either app, fetch application data, or depend on app stores.

App identity is selected by the Go entry point, not a frontend mode check.
The client bypasses login and uses `networkMode: 'always'` for its loopback API;
the server authenticates and uses `'online'`. Client remote-view switching is
still dynamic and independent of this split. `/version` still loads runtime
features and the gateway logout URL before routing.

`make build-frontend` and the Wails build hook use `npm run build:embed` to
build both apps and replace the copies in `internal/frontend/dist`.
`internal/frontend.ClientApp()` and `ServerApp()` expose the appropriate assets;
the same package handles static files and SPA fallback. Wails uses the client
bundle. Runtime branding, CSP nonces, and proxy base paths are preserved.

## Getting Started

### Prerequisites

- Node.js 20 (20.19 or newer), or Node.js 22.12 or newer (as required by Vite)
- npm (the repository uses npm workspaces and a shared lockfile)
- A matching Nebi backend: `nebi-web` for the client or `nebi-server` for the
  server frontend, normally running on `http://localhost:8460`

### Installation

```bash
# From the repository root
cd frontend
npm ci
```

### Development

From `frontend/`, start the UI for the backend you are running:

```bash
# Client UI for the local web / desktop backend (default)
npm run dev

# Server UI for the team backend
npm run dev:server
```

Both commands use http://localhost:8461 and should be run separately.
From the repository root, `make dev` starts the server UI and team backend;
`make dev FRONTEND_APP=client`
starts the client UI and local web backend. Both prepare empty embed files when
needed; Vite serves the UI while Go hot reloads.

The dev server includes:
- Hot Module Replacement (HMR)
- Proxy to backend API at `/api` → `http://localhost:8460`

### Build for Production

```bash
# Build both apps and typecheck the shared UI package
npm run build

# Build both apps and copy their assets into Go
npm run build:embed

# Preview the client production build
npm run preview
```

### Testing

```bash
# Run all client, server, and shared UI unit tests once
npm test

# Watch and rerun unit tests across all three workspaces
npm run test:watch

# Optionally watch only one project (@nebi/client, @nebi/server, or @nebi/ui)
npm run test:watch -- --project @nebi/server

# First-time Playwright browser setup
npx playwright install

# Run Playwright e2e tests, including axe accessibility assertions
npm run test:e2e

# Run only the accessibility-tagged Playwright flow
npm run test:a11y
```

Playwright starts separate client and server Vite instances automatically and
writes an HTML report to `playwright-report/`. After a run, view it with:

```bash
npx playwright show-report
```

## Environment Variables

Create a `.env` file in the frontend directory:

```env
VITE_API_URL=/api/v1
```

## Usage

### Client and Server Access

Open http://localhost:8461 while the corresponding Vite server is running.

- **Client:** `/login` redirects to `/projects`; local access does not require
  signing in. Use **Settings** to connect to a remote team server, then switch
  between local and remote views.
- **Server:** Sign in at `/login` using an account configured on the team
  backend. When OIDC is configured, use the configured sign-in flow. Successful
  sign-in opens **Projects**. There are no default credentials supplied by the UI.
  Remote connection settings (`/settings`) and remote project details
  (`/remote/projects/:id`) are client-only routes and are not registered in the
  server app.

### Managing Projects

1. Open **Projects** and choose **New Project**.
2. Enter the project name and configuration in the `pixi.toml` editor.
3. Choose **Create & Save**, then follow progress in the project's **Jobs** tab.

Select a project to inspect its overview, packages, configuration, and jobs.
Use its delete action to remove it after confirming the dialog.

### Managing Packages

The **Packages** tab lists installed packages. Edit dependencies in the
project's **Configuration** tab, then use **Save & Install** to apply them.
Track the operation in the project's **Jobs** tab.

### Viewing Jobs

1. Navigate to the "Jobs" tab
2. Click on a job to expand and view:
   - Job status
   - Logs
   - Error messages (if any)
   - Metadata

Jobs auto-refresh every 2 seconds for real-time updates.

## API Integration

Both apps communicate with their backend through REST APIs:

- **Client:** Uses the local backend, which bypasses authentication. It keeps
  queries and mutations running while the OS reports offline. Remote-view
  requests go through that local backend to the connected team server.
- **Server:** Uses authenticated requests to the team backend. Stored JWTs are
  attached to requests; authentication failures normally redirect to `/login`.
  Queries and mutations pause while the browser reports offline.
- **Updates:** Project, package, and job hooks poll for progress. Client remote
  queries use their own retry and polling policies to recover from outages.

## Styling

The app uses Tailwind CSS v4 with a custom theme:

- Shared theme and component styles are defined in `packages/ui/src/index.css`.
- Each app imports the shared styles from its own `src/index.css` and scans
  only its own source tree; the shared stylesheet scans shared UI components.
- shadcn/ui components for consistency
- Responsive design with mobile support
- Dark mode support (theme variables included)

## Key Features

### Real-time Updates

Jobs, environments, and packages automatically refresh every 2 seconds using React Query's `refetchInterval`.

### Protected Routes

The server app protects its main routes and redirects unauthenticated users to
`/login`. The client app bypasses the login gate. Both apps check access before
showing administrator routes; the backend enforces permissions according to its
runtime mode.

### Status Badges

Visual indicators for:
- Environment status: pending, creating, ready, failed, deleting
- Job status: pending, running, completed, failed
- Job type: create, delete, install, remove, update

### Error Handling

- Form validation
- API error messages
- Failed job error display
- Network error handling

## Development Tips

### Adding a New Page

1. Create component in `apps/<client|server>/src/pages/`
2. Add route in `apps/<client|server>/src/App.tsx`
3. Add navigation link in `apps/<client|server>/src/components/layout/Layout.tsx`

### Adding a New API Endpoint

1. Add function to appropriate file in `apps/<client|server>/src/api/`
2. Create custom hook in `apps/<client|server>/src/hooks/` if needed
3. Use the hook in your component

### Adding a shadcn Component

```bash
# Install shadcn CLI (if not using manual approach)
# Or manually create component in packages/ui/src/components/ui/
```

## Troubleshooting

**Build Errors:**
- Ensure all type imports use `import type { ... }`
- Check Tailwind v4 compatibility

**API Connection Issues:**
- Verify backend is running on port 8460
- Check proxy configuration in `apps/<client|server>/vite.config.ts`
- Inspect browser console for CORS errors

**Authentication Issues:**
- For the server UI, check the backend authentication configuration and JWT
  request headers; clear stored authentication and sign in again if needed.
- For the client UI, local access requires no login. Check the remote server
  connection in **Settings** when remote requests fail.

## Future Enhancements

- WebSocket support for real-time log streaming
- Environment templates
- Bulk operations
- Advanced filtering and search
- Environment health checks dashboard

## License

Same as parent Nebi project
