// Owner: U2 (detail).
import { HttpResponse, http } from 'msw';
import {
  mockJobSummary,
  mockVersionDetail,
  mockVersionItem,
} from '@/test/fixtures/contracts';
import type {
  ResolvedPackages,
  VersionDetail,
  VersionItem,
} from '@/types/contracts';

const C = '/api/v1/client';

const daysAgo = (d: number) =>
  new Date(Date.now() - d * 24 * 60 * 60 * 1000).toISOString();

/** A rail like the prototype's Project_1: v7 installed, v6 on the server. */
export const mockDetailVersions: VersionItem[] = [
  {
    ...mockVersionItem,
    number: 7,
    created_at: daysAgo(0),
    author: { name: 'Sam Garcia', username: 'local-user' },
    tags: ['latest'],
    sync: 'newest_here',
    installed: true,
  },
  {
    ...mockVersionItem,
    number: 6,
    description: 'Bumped Python to 3.12',
    author: { name: 'Alex Kim', username: 'alex' },
    created_at: daysAgo(3),
    tags: [],
    sync: 'newest_on_server',
    installed: false,
  },
  {
    ...mockVersionItem,
    number: 5,
    description: 'Swapped tensorflow for scikit-learn',
    created_at: daysAgo(12),
    tags: ['stable'],
    sync: null,
    installed: false,
    published: [
      {
        publication_id: 'pub-5',
        registry_id: 'reg-1',
        registry: 'ghcr.io',
        repository: 'example-org/project_1',
        tag: '5',
        reference: 'ghcr.io/example-org/project_1:5',
        source: 'local',
        can_unpublish: false,
      },
    ],
  },
  {
    ...mockVersionItem,
    number: 4,
    description: 'Added the pytorch channel',
    created_at: daysAgo(40),
    tags: [],
    sync: null,
    installed: false,
  },
];

/** A version only on the linked server. */
export const mockServerOnlyVersion: VersionItem = {
  ...mockVersionItem,
  number: 8,
  description: 'Added polars',
  author: { name: 'Alex Kim', username: 'alex' },
  created_at: daysAgo(0),
  tags: [],
  sync: 'newest_on_server',
  server_only: true,
  installed: false,
};

export const detailVersion = (v: VersionItem): VersionDetail => ({
  ...v,
  spec: mockVersionDetail.spec,
});

export const mockResolvedPackages: ResolvedPackages = {
  platforms: ['linux-64', 'linux-aarch64'],
  packages: {
    'linux-64': [
      {
        name: 'numpy',
        version: '2.1.3',
        build: 'py312h1',
        source: 'conda',
        platform: 'linux-64',
        channel: 'conda-forge',
        size: 8_400_000,
      },
      {
        name: 'python',
        version: '3.12.7',
        build: 'h1',
        source: 'conda',
        platform: 'linux-64',
        channel: 'conda-forge',
        size: 31_000_000,
      },
      {
        name: 'scipy',
        version: '1.14.1',
        build: 'py312h2',
        source: 'conda',
        platform: 'linux-64',
        channel: 'conda-forge',
        size: 17_500_000,
      },
    ],
    'linux-aarch64': [
      {
        name: 'python',
        version: '3.12.6',
        source: 'conda',
        platform: 'linux-aarch64',
        channel: 'conda-forge',
        size: 30_000_000,
      },
    ],
  },
  total_size: { 'linux-64': 56_900_000, 'linux-aarch64': 30_000_000 },
};

export const MOCK_PIXI_TOML =
  '[workspace]\nname = "Project_1"\nchannels = ["conda-forge"]\nplatforms = ["linux-64"]\n';

/** Handlers serving mockDetailVersions (use with server.use in tests). */
export const detailRailHandlers = (
  versions: VersionItem[] = mockDetailVersions,
) => [
  http.get(`${C}/projects/:id/versions`, () => HttpResponse.json(versions)),
  http.get(`${C}/projects/:id/versions/:version`, ({ params }) => {
    const v = versions.find((x) => x.number === Number(params.version));
    return v
      ? HttpResponse.json(detailVersion(v))
      : HttpResponse.json(
          { error: 'version not found', code: 'not_found' },
          { status: 404 },
        );
  }),
  http.get(`${C}/projects/:id/versions/:version/packages`, () =>
    HttpResponse.json(mockResolvedPackages),
  ),
  http.get(`${C}/projects/:id/versions/:version/files/:name`, ({ params }) =>
    HttpResponse.json({
      name: params.name as string,
      content: params.name === 'pixi.toml' ? MOCK_PIXI_TOML : 'version: 6\n',
    }),
  ),
];

export const detailHandlers = [
  http.get(`${C}/projects/:id/versions`, () =>
    HttpResponse.json([mockVersionItem]),
  ),
  http.get(`${C}/projects/:id/versions/:version`, () =>
    HttpResponse.json(mockVersionDetail),
  ),
  http.get(`${C}/projects/:id/versions/:version/files/:name`, ({ params }) =>
    HttpResponse.json({ name: params.name as string, content: '' }),
  ),
  http.get(`${C}/projects/:id/versions/:version/packages`, () =>
    HttpResponse.json({ platforms: [], packages: {}, total_size: {} }),
  ),
  http.get(
    `${C}/projects/:id/versions/:version/archive`,
    ({ params }) =>
      new HttpResponse(new Uint8Array([0x1f, 0x8b, 0x08, 0x00]), {
        headers: {
          'Content-Type': 'application/gzip',
          'Content-Disposition': `attachment; filename=Project_1-v${params.version}.tar.gz`,
        },
      }),
  ),
  http.post(`${C}/projects/:id/install`, () =>
    HttpResponse.json(
      { ...mockJobSummary, status: 'pending' },
      { status: 202 },
    ),
  ),
  http.post(`${C}/projects/:id/uninstall`, () =>
    HttpResponse.json(
      { ...mockJobSummary, type: 'uninstall', status: 'pending' },
      { status: 202 },
    ),
  ),
];
