// Owner: U1 (projects). MSW handlers for /api/v1/client project routes.
import { HttpResponse, http } from 'msw';
import {
  JOB_ID,
  mockCapabilities,
  mockJobSummary,
  mockProjectItem,
} from '@/test/fixtures/contracts';
import type {
  CreateProjectRequest,
  ManifestRenderRequest,
  ProjectListItem,
} from '@/types/contracts';

const C = '/api/v1/client';

export const NEW_PROJECT_ID = '55555555-5555-4555-8555-555555555555';

/** A project that is not installed, with a newer version on the server. */
export const mockPulledProject: ProjectListItem = {
  ...mockProjectItem,
  id: '66666666-6666-4666-8666-666666666666',
  name: 'Project_pulled_from_server',
  path: '/home/user/Project_pulled_from_server',
  install_status: 'not_installed',
  installed_version: null,
  last_installed_version: 2,
  latest_version: 2,
  size_bytes: null,
  size: '',
  remotes: [{ kind: 'registry', name: 'ghcr.io/nebari-dev' }],
  update_available: true,
  server_latest_version: 3,
};

/** A project whose last install failed. */
export const mockFailedProject: ProjectListItem = {
  ...mockProjectItem,
  id: '77777777-7777-4777-8777-777777777777',
  name: 'sandbox',
  path: '/home/user/sandbox',
  install_status: 'install_failed',
  installed_version: null,
  last_installed_version: null,
  latest_version: 1,
  size_bytes: null,
  size: '',
  remotes: [],
  update_available: false,
  server_latest_version: null,
  failed_install_job_id: JOB_ID,
};

export const mockProjects: ProjectListItem[] = [
  mockProjectItem,
  mockPulledProject,
  mockFailedProject,
];

/** Unknown tables reported by POST /manifest/parse when present. */
const unknownTables = (toml: string) =>
  [...toml.matchAll(/^\s*\[([^\]]+)\]\s*$/gm)]
    .map((m) => m[1].trim())
    .filter(
      (t) =>
        !['workspace', 'project', 'dependencies', 'pypi-dependencies'].includes(
          t,
        ),
    )
    .map((t) => `[${t}]`);

const listOf = (toml: string, key: string) =>
  (toml.match(new RegExp(`^${key}\\s*=\\s*\\[(.*)\\]`, 'm'))?.[1] ?? '')
    .split(',')
    .map((s) => s.trim().replace(/^"|"$/g, ''))
    .filter(Boolean);

export const projectsHandlers = [
  http.get(`${C}/capabilities`, () => HttpResponse.json(mockCapabilities)),
  http.post(`${C}/desktop/pick-folder`, () =>
    HttpResponse.json(
      { error: 'unavailable', code: 'unavailable' },
      { status: 501 },
    ),
  ),
  // Default list: one installed project (other units' tests rely on it);
  // the projects tests override it with mockProjects.
  http.get(`${C}/projects`, () => HttpResponse.json([mockProjectItem])),
  http.get(`${C}/projects/name-check`, ({ request }) => {
    const name = new URL(request.url).searchParams.get('name') ?? '';
    const taken = mockProjects.some(
      (p) => p.name.toLowerCase() === name.toLowerCase(),
    );
    return HttpResponse.json(
      taken
        ? {
            name,
            available: false,
            error: 'A project with that name already exists.',
          }
        : { name, available: true },
    );
  }),
  http.get(`${C}/projects/:id`, ({ params }) =>
    HttpResponse.json({ ...mockProjectItem, id: params.id as string }),
  ),
  http.post(`${C}/projects`, async ({ request }) => {
    const body = (await request.json()) as CreateProjectRequest;
    if (body.name === mockProjectItem.name) {
      return HttpResponse.json(
        {
          error: 'A project with that name already exists.',
          code: 'name_taken',
        },
        { status: 409 },
      );
    }
    return HttpResponse.json(
      {
        project: {
          ...mockProjectItem,
          id: NEW_PROJECT_ID,
          name: body.name,
          status: 'pending',
          install_status: 'not_installed',
          installed_version: null,
          latest_version: null,
          active_job_id: JOB_ID,
        },
        job: {
          ...mockJobSummary,
          type: 'create',
          type_label: 'Create project',
          status: 'pending',
          project_id: NEW_PROJECT_ID,
          project_name: body.name,
          version_number: null,
        },
      },
      { status: 202 },
    );
  }),
  http.delete(`${C}/projects/:id`, () =>
    HttpResponse.json(
      {
        ...mockJobSummary,
        type: 'delete',
        type_label: 'Delete project',
        status: 'pending',
      },
      { status: 202 },
    ),
  ),
  http.post(`${C}/manifest/parse`, async ({ request }) => {
    const { pixi_toml: toml } = (await request.json()) as {
      pixi_toml: string;
    };
    if (!/^\s*\[(workspace|project)\]/m.test(toml)) {
      return HttpResponse.json(
        {
          error: 'pixi.toml isn’t valid TOML: missing table',
          code: 'invalid',
          field: 'manifest',
        },
        { status: 422 },
      );
    }
    return HttpResponse.json({
      form: {
        name: toml.match(/^name\s*=\s*"([^"]*)"/m)?.[1] ?? '',
        platforms: listOf(toml, 'platforms'),
        channels: listOf(toml, 'channels'),
        requested: [],
      },
      unrepresentable: unknownTables(toml),
    });
  }),
  http.post(`${C}/manifest/render`, async ({ request }) => {
    const { form } = (await request.json()) as ManifestRenderRequest;
    return HttpResponse.json({
      pixi_toml: `[workspace]\nname = "${form.name}"\nplatforms = [${form.platforms
        .map((p) => `"${p}"`)
        .join(', ')}]\n\n[dependencies]\n`,
    });
  }),
];
