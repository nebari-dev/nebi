// Owner: U7 (publish).
import { HttpResponse, http } from 'msw';
import {
  JOB_ID,
  mockJobSummary,
  mockRegistryItem,
  PROJECT_ID,
  REGISTRY_ID,
} from '@/test/fixtures/contracts';
import type {
  AddRegistryRequest,
  JobSummary,
  PublishDefault,
  PublishedRef,
  PublishRequest,
  PublishResult,
  PullResult,
  RegistryItem,
  RegistryPullRequest,
} from '@/types/contracts';

const C = '/api/v1/client';

export const QUAY_REGISTRY_ID = '55555555-5555-4555-8555-555555555555';
export const PUBLICATION_ID = '66666666-6666-4666-8666-666666666666';

/** A registry that supports unpublish (mockRegistryItem is GHCR, which doesn't). */
export const mockQuayRegistry: RegistryItem = {
  id: QUAY_REGISTRY_ID,
  name: 'Quay',
  url: 'quay.io',
  namespace: 'nebari',
  username: '',
  has_credentials: false,
  is_default: false,
  can_unpublish: true,
};

export const mockRegistries: RegistryItem[] = [
  mockRegistryItem,
  mockQuayRegistry,
];

export const mockPublishDefaults: PublishDefault[] = [
  {
    registry_id: REGISTRY_ID,
    registry_name: mockRegistryItem.name,
    repository: 'example-org/project_1',
    tag: '7',
  },
  {
    registry_id: QUAY_REGISTRY_ID,
    registry_name: 'Quay',
    repository: 'nebari/project_1',
    tag: '7',
  },
];

/** Version 7 published to Quay from this machine. */
export const mockQuayPublication: PublishedRef = {
  publication_id: PUBLICATION_ID,
  registry_id: QUAY_REGISTRY_ID,
  registry: 'quay.io',
  repository: 'nebari/project_1',
  tag: '7',
  reference: 'quay.io/nebari/project_1:7',
  source: 'local',
  can_unpublish: true,
};

export const mockPullJob: JobSummary = {
  ...mockJobSummary,
  id: JOB_ID,
  type: 'pull_registry',
  type_label: 'Pull from registry',
  status: 'pending',
  project_name: 'project_1',
  version_number: null,
};

export const mockPullResult: PullResult = {
  project_id: PROJECT_ID,
  job: mockPullJob,
};

export const publishHandlers = [
  http.get(`${C}/registries`, () => HttpResponse.json(mockRegistries)),
  http.post(`${C}/registries`, async ({ request }) => {
    const body = (await request.json()) as AddRegistryRequest;
    const item: RegistryItem = {
      id: '77777777-7777-4777-8777-777777777777',
      name: body.name,
      url: body.url,
      namespace: body.namespace ?? '',
      username: body.username ?? '',
      has_credentials: !!body.username,
      is_default: !!body.is_default,
      can_unpublish: true,
    };
    return HttpResponse.json(item, { status: 201 });
  }),
  http.put(`${C}/registries/:registryId`, async ({ params, request }) => {
    const body = (await request.json()) as Partial<RegistryItem>;
    const base =
      mockRegistries.find((r) => r.id === params.registryId) ??
      mockRegistryItem;
    return HttpResponse.json({ ...base, ...body });
  }),
  http.delete(
    `${C}/registries/:registryId`,
    () => new HttpResponse(null, { status: 204 }),
  ),
  http.get(`${C}/registries/:registryId/repositories`, () =>
    HttpResponse.json(['nebari/ml-baseline', 'nebari/project_1']),
  ),
  http.get(`${C}/registries/:registryId/tags`, () =>
    HttpResponse.json(['7', '5', 'stable']),
  ),
  http.post(`${C}/registries/:registryId/pull`, async ({ request }) => {
    const body = (await request.json()) as RegistryPullRequest;
    return HttpResponse.json(
      {
        ...mockPullResult,
        job: { ...mockPullJob, project_name: body.local_name },
      },
      { status: 202 },
    );
  }),
  http.get(`${C}/projects/:id/versions/:version/publish-defaults`, () =>
    HttpResponse.json(mockPublishDefaults),
  ),
  http.post(
    `${C}/projects/:id/versions/:version/publish`,
    async ({ request }) => {
      const body = (await request.json()) as PublishRequest;
      const res: PublishResult = {
        results: body.targets.map((t, i) => {
          const reg = mockRegistries.find((r) => r.id === t.registry_id);
          return {
            registry_id: t.registry_id,
            registry_name: reg?.name ?? 'Registry',
            reference: `${reg?.url ?? 'registry'}/${t.repository}:${t.tag}`,
            status: 'published',
            digest: 'sha256:0123',
            publication_id: `88888888-8888-4888-8888-88888888888${i}`,
          };
        }),
      };
      return HttpResponse.json(res);
    },
  ),
  http.delete(
    `${C}/projects/:id/publications/:publicationId`,
    () => new HttpResponse(null, { status: 204 }),
  ),
];
