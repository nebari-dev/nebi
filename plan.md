  ⎿  Darb - Multi-User Environment Management System

     Project Summary

     Darb is a REST API with a web UI for managing pixi (and future uv) environments in multi-user systems. It supports local, Docker, and Kubernetes deployments with a container-first execution model. The entire system ships as a
     single Go binary with embedded React UI, supporting SQLite (dev) or PostgreSQL (prod), Basic Auth or OIDC, and includes RBAC with an admin interface. All operations are queued (Redis/in-memory) with real-time WebSocket log
     streaming. The UI is minimal and clean using IBM Plex Sans/Fira Code fonts with Tailwind CSS. The backend outputs structured JSON logs for production environments and includes embedded Swagger documentation.

     Key Requirements:
     - Single binary deployment with embedded frontend
     - SQLite (local) or PostgreSQL (production)
     - Basic Auth or OIDC authentication (pluggable)
     - RBAC with admin interface for access control management
     - Queue system for concurrent environment operations
     - Container-first execution (Docker/K8s Jobs)
     - Real-time log streaming via WebSockets
     - Structured logging (JSON) for production
     - Embedded Swagger API docs
     - Clean minimal UI (IBM Plex Sans, Fira Code, Tailwind)
     - Support pixi now, extensible for uv later

     ---
     Phase 1: Core Infrastructure

     1. Project structure - darb module: cmd/server, internal/, web/dist, docs/swagger
     2. Structured logging - zerolog with JSON output, log levels, contextual fields
     3. Database abstraction - GORM with SQLite/PostgreSQL drivers, auto-migration
     4. Database schema - users, roles, environments, jobs, permissions, templates, packages, audit_logs
     5. Job queue system - Pluggable: in-memory (dev) or Redis (prod)
     6. Authentication layer - Pluggable: Basic Auth (bcrypt) or OIDC middleware
     7. Swagger generation - swaggo/swag annotations, embedded Swagger UI at /docs

     Phase 2: Package Manager Abstraction

     8. Package manager interface - Abstract pixi/uv operations (create, install, remove, list)
     9. Pixi implementation - Pixi-specific commands and manifest parsing
     10. Future uv support - Stub for uv backend implementation
     11. Container images - Base images with pixi (and later uv) installed

     Phase 3: Backend Execution Layer

     12. Container runtime abstraction - Interface for Docker/K8s operations
     13. Docker backend - Docker SDK: volumes, containers, exec
     14. K8s backend - Client-go: Jobs, PVCs, single namespace with labels
     15. Volume management - Create/mount persistent storage per environment

     Phase 4: Environment Operations (Queued Jobs)

     16. CreateEnvironment job - Container runs package manager init, stores to volume
     17. InstallPackage job - Container mounts volume, adds package
     18. RemovePackage job - Container removes package
     19. DeleteEnvironment job - Cleanup volumes, jobs, records
     20. Job execution - Worker pool with structured logging and real-time log streaming

     Phase 5: RBAC & Access Control

     21. RBAC with casbin - Policies: admin, owner, editor, viewer roles
     22. Permission model - Environment-level and global permissions
     23. Admin API endpoints - /api/v1/admin/{users,roles,permissions,audit}
     24. User management API - Create/update/delete users, assign roles
     25. Permission management API - Grant/revoke environment access, list permissions
     26. Audit logging - Structured logs for all RBAC changes and sensitive operations

     Phase 6: API & Real-time Features

     27. REST API - Gin with Swagger annotations: /api/v1/{environments,packages,templates,jobs}
     28. Auth endpoints - /api/v1/auth/login (basic), /api/v1/auth/callback (OIDC)
     29. Swagger docs - OpenAPI spec at /docs/swagger.json, Swagger UI at /docs
     30. WebSocket server - Real-time job status, queue position, logs at /ws
     31. Template management - CRUD for environment templates
     32. Monitoring API - /api/v1/health, /api/v1/metrics
    
     Phase 7: User Interface

     33. React app - Vite, TypeScript, Tailwind CSS, shadcn/ui
     34. Typography - IBM Plex Sans (UI), Fira Code (code/logs)
     35. Build integration - Frontend embedded in Go binary
     36. Auth flow - Login form (basic) or OIDC redirect, token management
     37. User dashboard - Environment cards, status indicators, accessible environments
     38. Environment wizard - Create form with template selection
     39. Live log viewer - Terminal-style with Fira Code, WebSocket updates

     Phase 8: Admin Interface

     40. Admin dashboard - Restricted to admin role, overview stats
     41. User management UI - List/create/edit users, assign roles
     42. Permission management UI - Grant/revoke environment access per user
     43. Role management UI - Create custom roles, define permissions
     44. Audit log viewer - Searchable/filterable audit trail with structured log display
     45. System settings - Configure backend, queue, auth settings (if allowed)

     Phase 9: Single Binary Deployment

     46. Build script - make build → frontend build → Go binary with embedded assets
     47. Configuration - YAML/ENV with auth type, database, queue, log format
     48. Local mode - ./darb (SQLite + memory + Docker + basic auth + console logs)
     49. Production mode - ./darb -c config.yaml (Postgres + Redis + K8s + OIDC + JSON logs)
     50. Docker image - Alpine with single binary
     51. K8s manifests - Deployment, ConfigMap, Secrets
     52. Release artifacts - Multi-platform binaries via GitHub Actions
