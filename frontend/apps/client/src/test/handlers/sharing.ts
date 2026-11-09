// Owner: U6 (sharing).
import { HttpResponse, http } from 'msw';
import { mockAccessList } from '@/test/fixtures/contracts';

const C = '/api/v1/client';

export const sharingHandlers = [
  http.get(`${C}/remote/principals`, () => HttpResponse.json([])),
  http.get(`${C}/remote/projects/:serverProjectId/access`, () =>
    HttpResponse.json(mockAccessList),
  ),
];
