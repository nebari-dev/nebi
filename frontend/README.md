# Nebi Frontend

Modern React frontend for the Nebi environment management system.

## Tech Stack

- **React 18** with TypeScript
- **Vite** - Fast build tool and dev server
- **Tailwind CSS v4** - Utility-first CSS framework
- **shadcn/ui** - Beautiful, accessible component library
- **TanStack Query (React Query)** - Powerful server state management
- **Zustand** - Lightweight client state for authentication
- **React Router v6** - Client-side routing
- **Axios** - HTTP client
- **Lucide React** - Icon library

## Features

- ✅ User authentication with JWT
- ✅ Environment management (create, view, delete)
- ✅ Package installation and removal
- ✅ Real-time job status updates (2-second polling)
- ✅ Job log viewing
- ✅ Responsive design with Tailwind CSS
- ✅ Clean, minimal UI with shadcn components

## Project Structure

```
frontend/
├── src/
│   ├── api/              # API client and endpoints
│   │   ├── client.ts     # Axios instance with interceptors
│   │   ├── auth.ts       # Authentication API
│   │   ├── environments.ts
│   │   ├── jobs.ts
│   │   └── packages.ts
│   ├── components/
│   │   ├── ui/           # shadcn/ui components
│   │   └── layout/       # Layout components
│   ├── hooks/            # Custom React hooks
│   │   ├── useEnvironments.ts
│   │   ├── useJobs.ts
│   │   └── usePackages.ts
│   ├── pages/            # Page components
│   │   ├── Login.tsx
│   │   ├── Environments.tsx
│   │   ├── EnvironmentDetail.tsx
│   │   └── Jobs.tsx
│   ├── store/            # Zustand stores
│   │   └── authStore.ts
│   ├── types/            # TypeScript type definitions
│   ├── lib/              # Utility functions
│   ├── App.tsx           # Main app component
│   └── main.tsx          # Entry point
├── public/
├── .env                  # Environment variables
├── vite.config.ts        # Vite configuration
├── tailwind.config.js    # Tailwind configuration
└── tsconfig.json         # TypeScript configuration
```

## Getting Started

### Prerequisites

- Node.js 18+ (or compatible version)
- npm or yarn
- Nebi backend running on `http://localhost:8460`

### Installation

```bash
# Install dependencies
npm install
```

### Development

```bash
# Start development server
npm run dev
```

The app will be available at http://localhost:8461

The dev server includes:
- Hot Module Replacement (HMR)
- Proxy to backend API at `/api` → `http://localhost:8460`

### Build for Production

```bash
# Build the app
npm run build

# Preview the production build
npm run preview
```

## Environment Variables

Create a `.env` file in the frontend directory:

```env
VITE_API_URL=/api/v1
```

## Usage

### Login

1. Navigate to http://localhost:3000/login
2. Enter your credentials (created via the backend)
3. You'll be redirected to the Environments page

Default test user (if created):
- Username: `admin`
- Password: `password123`

### Managing Environments

**Create Environment:**
1. Click "New Environment"
2. Enter a name
3. Click "Create"
4. Watch the status change from `creating` → `ready`

**View Environment Details:**
- Click on an environment card to see details and packages

**Delete Environment:**
- Click the trash icon on an environment card

### Managing Packages

**Install Packages:**
1. Open an environment
2. Click "Install Package"
3. Enter package name(s), e.g., `python=3.11, numpy`
4. Multiple packages can be comma-separated
5. Watch the job status in the Jobs page

**Remove Package:**
- Click the trash icon on a package card

### Viewing Jobs

1. Navigate to the "Jobs" tab
2. Click on a job to expand and view:
   - Job status
   - Logs
   - Error messages (if any)
   - Metadata

Jobs auto-refresh every 2 seconds for real-time updates.

## API Integration

The frontend communicates with the Nebi backend via REST API:

- **Authentication:** JWT tokens stored in localStorage
- **Auto-refresh:** React Query polls every 2 seconds for updates
- **Error Handling:** Automatic redirect to login on 401
- **Request Interceptor:** Adds JWT token to all requests
- **Response Interceptor:** Handles auth errors globally

## Styling

The app uses Tailwind CSS v4 with a custom theme:

- Color scheme defined in `src/index.css`
- shadcn/ui components for consistency
- Responsive design with mobile support
- Dark mode support (theme variables included)

## Key Features

### Real-time Updates

Jobs, environments, and packages automatically refresh every 2 seconds using React Query's `refetchInterval`.

### Protected Routes

All routes except `/login` require authentication. Unauthenticated users are redirected to the login page.

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

1. Create component in `src/pages/`
2. Add route in `src/App.tsx`
3. Add navigation link in `src/components/layout/Layout.tsx`

### Adding a New API Endpoint

1. Add function to appropriate file in `src/api/`
2. Create custom hook in `src/hooks/` if needed
3. Use the hook in your component

### Adding a shadcn Component

```bash
# Install shadcn CLI (if not using manual approach)
# Or manually create component in src/components/ui/
```

## Troubleshooting

**Build Errors:**
- Ensure all type imports use `import type { ... }`
- Check Tailwind v4 compatibility

**API Connection Issues:**
- Verify backend is running on port 8460
- Check proxy configuration in `vite.config.ts`
- Inspect browser console for CORS errors

**Authentication Issues:**
- Clear localStorage and try logging in again
- Verify JWT token is being sent in request headers

## Future Enhancements

- WebSocket support for real-time log streaming
- Dark mode toggle
- Environment templates
- Bulk operations
- Advanced filtering and search
- Environment health checks dashboard

## License

Same as parent Nebi project
