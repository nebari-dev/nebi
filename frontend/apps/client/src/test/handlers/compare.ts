// Owner: U4 (compare).
import { HttpResponse, http } from 'msw';
import { mockVersionItem } from '@/test/fixtures/contracts';
import type {
  CompareResult,
  CompareVersionRef,
  DiffCounts,
  PackageDiffEntry,
  VersionItem,
} from '@/types/contracts';

const C = '/api/v1/client';

/** Versions v7 (installed, newest here) down to v1; v6 is the server's latest. */
export const mockCompareVersions: VersionItem[] = [7, 6, 5, 4, 3, 2, 1].map(
  (number) => ({
    ...mockVersionItem,
    number,
    description: `Version ${number}`,
    tags: number === 7 ? ['latest'] : [],
    base_version: number > 1 ? number - 1 : null,
    sync:
      number === 7 ? 'newest_here' : number === 6 ? 'newest_on_server' : null,
    installed: number === 7,
    content_hash: `sha-00000000000${number}`,
  }),
);

const same = (name: string, version: string): PackageDiffEntry => ({
  name,
  source: 'conda',
  change: 'same',
  base: { constraint: '*', version },
  compare: { constraint: '*', version },
});

/** v7 against v6: scipy was added in v7, numpy moved from 2.0.2 to 2.1.3. */
export const mockCompareRequested: PackageDiffEntry[] = [
  same('ipykernel', '6.29.5'),
  {
    name: 'numpy',
    source: 'conda',
    change: 'changed',
    base: { constraint: '>=2.1', version: '2.1.3' },
    compare: { constraint: '>=2.0', version: '2.0.2' },
  },
  same('python', '3.12.7'),
  {
    name: 'rich',
    source: 'pypi',
    change: 'added',
    base: null,
    compare: { constraint: '>=13', version: '13.9.4' },
  },
  {
    name: 'scipy',
    source: 'conda',
    change: 'removed',
    base: { constraint: '>=1.11', version: '1.14.1' },
    compare: null,
  },
];

const counts = (rows: PackageDiffEntry[]): DiffCounts => ({
  added: rows.filter((r) => r.change === 'added').length,
  removed: rows.filter((r) => r.change === 'removed').length,
  changed: rows.filter((r) => r.change === 'changed').length,
  same: rows.filter((r) => r.change === 'same').length,
});

const ref = (n: number): CompareVersionRef => ({
  number: n,
  server_only: false,
  installed: n === 7,
  server_latest: n === 6,
});

/** A CompareResult for base/target; identical versions have no changes. */
export const mockCompareResult = (
  base: number,
  target: number,
  rows: PackageDiffEntry[] = mockCompareRequested,
): CompareResult => {
  const requested =
    base === target
      ? rows
          .filter((r) => r.base)
          .map((r) => ({
            ...r,
            change: 'same' as const,
            compare: r.base,
          }))
      : rows;
  return {
    base: ref(base),
    target: ref(target),
    platform: 'linux-64',
    requested,
    requested_counts: counts(requested),
    resolved: [],
    resolved_counts: { added: 0, removed: 0, changed: 0, same: 0 },
    channels: { added: [], removed: [] },
    platforms: { added: [], removed: [] },
  };
};

export const compareHandlers = [
  http.get(`${C}/projects/:id/compare`, ({ request }) => {
    const q = new URL(request.url).searchParams;
    return HttpResponse.json(
      mockCompareResult(Number(q.get('base')), Number(q.get('target'))),
    );
  }),
];
