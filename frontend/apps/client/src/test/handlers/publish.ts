// Owner: U7 (publish).
import { HttpResponse, http } from 'msw';
import { mockRegistryItem } from '@/test/fixtures/contracts';

const C = '/api/v1/client';

export const publishHandlers = [
  http.get(`${C}/registries`, () => HttpResponse.json([mockRegistryItem])),
  http.get(`${C}/projects/:id/versions/:version/publish-defaults`, () =>
    HttpResponse.json([]),
  ),
];
