import userEvent from '@testing-library/user-event';
import { HttpResponse, http } from 'msw';
import { Route, Routes, useLocation } from 'react-router-dom';
import { beforeEach, describe, expect, it } from 'vitest';
import { mockProjectItem, PROJECT_ID } from '@/test/fixtures/contracts';
import { server } from '@/test/handlers';
import {
  mockCompareResult,
  mockCompareVersions,
} from '@/test/handlers/compare';
import { renderWithProviders, screen, waitFor, within } from '@/test/utils';
import { ComparePage } from './ComparePage';

const C = '/api/v1/client';

export const DetailStub = () => {
  const location = useLocation();
  return <p>Detail page {location.search}</p>;
};

export const SearchProbe = () => {
  const location = useLocation();
  return <output aria-label="search">{location.search}</output>;
};

function renderCompare(query = '') {
  return renderWithProviders(
    <Routes>
      <Route
        path="/projects/:id/compare"
        element={
          <>
            <ComparePage />
            <SearchProbe />
          </>
        }
      />
      <Route path="/projects/:id" element={<DetailStub />} />
      <Route path="/" element={<p>Projects list</p>} />
    </Routes>,
    { initialEntries: [`/projects/${PROJECT_ID}/compare${query}`] },
  );
}

/** Records the base/target pairs the page asked the API for. */
function recordCompareRequests() {
  const calls: Array<[number, number]> = [];
  server.use(
    http.get(`${C}/projects/:id/compare`, ({ request }) => {
      const q = new URL(request.url).searchParams;
      const base = Number(q.get('base'));
      const target = Number(q.get('target'));
      calls.push([base, target]);
      return HttpResponse.json(mockCompareResult(base, target));
    }),
  );
  return calls;
}

describe('ComparePage', () => {
  beforeEach(() => {
    server.use(
      http.get(`${C}/projects/:id/versions`, () =>
        HttpResponse.json(mockCompareVersions),
      ),
    );
  });

  it('compares the installed version against the server’s latest by default', async () => {
    const calls = recordCompareRequests();
    renderCompare();

    expect(
      await screen.findByRole('heading', { level: 1, name: 'Project_1' }),
    ).toBeInTheDocument();
    expect(screen.getByText('/home/user/Project_1')).toBeInTheDocument();
    expect(
      screen.getByRole('heading', { level: 2, name: 'Compare versions' }),
    ).toBeInTheDocument();
    expect(screen.getByRole('combobox', { name: 'Base' })).toHaveTextContent(
      'v7 (installed)',
    );
    expect(
      screen.getByRole('combobox', { name: 'Compare to' }),
    ).toHaveTextContent('v6 (latest on server)');

    const table = await screen.findByRole('table', {
      name: 'Requested packages, v7 (installed) compared to v6 (latest on server)',
    });
    expect(calls).toEqual([[7, 6]]);

    const summary = screen.getByRole('status', { name: 'Summary of changes' });
    expect(within(summary).getByTitle('Added')).toHaveTextContent('+1');
    expect(within(summary).getByTitle('Removed')).toHaveTextContent('−1');
    expect(within(summary).getByTitle('Changed')).toHaveTextContent('~1');

    const rows = within(table).getAllByRole('row');
    // Header + 5 packages.
    expect(rows).toHaveLength(6);
    const scipy = rows.find((r) => r.textContent?.includes('scipy'));
    expect(scipy).toBeDefined();
    expect(
      within(scipy as HTMLElement).getByLabelText('removed'),
    ).toHaveTextContent('−');
    expect(
      within(scipy as HTMLElement).getByText('Not in this version'),
    ).toBeInTheDocument();
    const numpy = rows.find((r) =>
      r.textContent?.includes('numpy'),
    ) as HTMLElement;
    expect(within(numpy).getAllByLabelText('changed')).toHaveLength(2);
    expect(within(numpy).getByText('2.1.3')).toBeInTheDocument();
    expect(within(numpy).getByText('2.0.2')).toBeInTheDocument();
    const rich = rows.find((r) =>
      r.textContent?.includes('rich'),
    ) as HTMLElement;
    expect(within(rich).getByText('PyPI')).toBeInTheDocument();
  });

  it('starts from the version the page was opened from', async () => {
    const calls = recordCompareRequests();
    renderCompare('?from=5');
    expect(
      await screen.findByRole('table', {
        name: 'Requested packages, v5 compared to v7 (installed)',
      }),
    ).toBeInTheDocument();
    expect(calls).toEqual([[5, 7]]);
  });

  it('reads base and to from the URL and updates them from the pickers', async () => {
    const user = userEvent.setup();
    const calls = recordCompareRequests();
    renderCompare('?base=3&to=4');
    await screen.findByRole('table', {
      name: 'Requested packages, v3 compared to v4',
    });

    await user.click(screen.getByRole('combobox', { name: 'Compare to' }));
    await user.click(
      await screen.findByRole('option', { name: 'v6 (latest on server)' }),
    );

    await screen.findByRole('table', {
      name: 'Requested packages, v3 compared to v6 (latest on server)',
    });
    expect(calls).toEqual([
      [3, 4],
      [3, 6],
    ]);
    expect(screen.getByLabelText('search')).toHaveTextContent('base=3&to=6');
  });

  it('shows "No changes" when both sides are the same version', async () => {
    renderCompare('?base=6&to=6');
    const summary = await screen.findByRole('status', {
      name: 'Summary of changes',
    });
    expect(summary).toHaveTextContent('No changes');
    expect(screen.queryByLabelText('removed')).not.toBeInTheDocument();
  });

  it('does not say "No changes" when only channels or platforms changed', async () => {
    server.use(
      http.get(`${C}/projects/:id/compare`, () =>
        HttpResponse.json({
          ...mockCompareResult(6, 6),
          base: mockCompareResult(7, 6).base,
          channels: { added: ['bioconda'], removed: [] },
        }),
      ),
    );
    renderCompare();
    const summary = await screen.findByRole('status', {
      name: 'Summary of changes',
    });
    await waitFor(() => expect(summary).toHaveTextContent('+0−0~0'));
    expect(summary).not.toHaveTextContent('No changes');
  });

  it('shows the requested constraints of a package whose constraint changed', async () => {
    renderCompare();
    const table = await screen.findByRole('table');
    expect(await within(table).findByText('(>=2.1)')).toBeInTheDocument();
    expect(within(table).getByText('(>=2.0)')).toBeInTheDocument();
  });

  it('lists channel and platform changes', async () => {
    server.use(
      http.get(`${C}/projects/:id/compare`, () =>
        HttpResponse.json({
          ...mockCompareResult(7, 6),
          channels: { added: ['bioconda'], removed: [] },
          platforms: { added: [], removed: ['osx-arm64'] },
        }),
      ),
    );
    renderCompare();
    expect(
      await screen.findByText('Channels: added bioconda'),
    ).toBeInTheDocument();
    expect(
      screen.getByText('Platforms: removed osx-arm64'),
    ).toBeInTheDocument();
  });

  it('Exit Compare returns to the version the page was opened from', async () => {
    const user = userEvent.setup();
    renderCompare('?from=5');
    await user.click(
      await screen.findByRole('button', { name: 'Exit Compare' }),
    );
    expect(await screen.findByText('Detail page ?v=5')).toBeInTheDocument();
  });

  it('Exit Compare returns to the detail page', async () => {
    const user = userEvent.setup();
    renderCompare();
    await user.click(
      await screen.findByRole('button', { name: 'Exit Compare' }),
    );
    expect(await screen.findByText(/^Detail page\s*$/)).toBeInTheDocument();
  });

  it('labels a version that is only on the server', async () => {
    const user = userEvent.setup();
    server.use(
      http.get(`${C}/projects/:id/versions`, () =>
        HttpResponse.json([
          {
            ...mockCompareVersions[0],
            number: 9,
            server_only: true,
            installed: false,
            sync: null,
          },
          {
            ...mockCompareVersions[0],
            number: 8,
            server_only: true,
            installed: false,
            sync: 'newest_on_server',
          },
          ...mockCompareVersions,
        ]),
      ),
      http.get(`${C}/projects/:id`, () =>
        HttpResponse.json({
          ...mockProjectItem,
          server_latest_version: 9,
        }),
      ),
    );
    renderCompare();
    const target = await screen.findByRole('combobox', { name: 'Compare to' });
    await waitFor(() =>
      expect(target).toHaveTextContent('v9 (latest on server)'),
    );
    await user.click(target);
    expect(
      await screen.findByRole('option', { name: 'v8 (on server)' }),
    ).toBeInTheDocument();
  });

  it('explains when the server cannot be reached for a server-only side', async () => {
    const user = userEvent.setup();
    let down = true;
    server.use(
      http.get(`${C}/projects/:id/compare`, () =>
        down
          ? HttpResponse.json(
              {
                error: 'server unreachable: connection refused',
                code: 'server_unreachable',
              },
              { status: 502 },
            )
          : HttpResponse.json(mockCompareResult(7, 6)),
      ),
    );
    renderCompare();
    expect(
      await screen.findByText('Can’t reach the server'),
    ).toBeInTheDocument();
    expect(
      screen.getByText(/Nebi keeps trying in the background\./),
    ).toBeInTheDocument();
    expect(
      screen.getByText('server unreachable: connection refused'),
    ).toBeInTheDocument();

    down = false;
    await user.click(screen.getByRole('button', { name: 'Try again' }));
    expect(
      await screen.findByRole('table', {
        name: 'Requested packages, v7 (installed) compared to v6 (latest on server)',
      }),
    ).toBeInTheDocument();
    expect(
      screen.queryByText('Can’t reach the server'),
    ).not.toBeInTheDocument();
  });

  it('shows the API error for a version that is not available', async () => {
    server.use(
      http.get(`${C}/projects/:id/compare`, () =>
        HttpResponse.json(
          { error: 'version 9: not found', code: 'not_found' },
          { status: 404 },
        ),
      ),
    );
    renderCompare();
    expect(
      await screen.findByText('Couldn’t compare these versions'),
    ).toBeInTheDocument();
    expect(screen.getByText('version 9: not found')).toBeInTheDocument();
  });

  it('says when there are no versions yet', async () => {
    server.use(
      http.get(`${C}/projects/:id/versions`, () => HttpResponse.json([])),
    );
    renderCompare();
    expect(
      await screen.findByText('This project has no versions to compare yet.'),
    ).toBeInTheDocument();
  });

  it('shows "Project not found" for an unknown project', async () => {
    const user = userEvent.setup();
    server.use(
      http.get(`${C}/projects/:id`, () =>
        HttpResponse.json(
          { error: 'not found', code: 'not_found' },
          { status: 404 },
        ),
      ),
    );
    renderCompare();
    expect(
      await screen.findByRole('heading', {
        level: 1,
        name: 'Project not found',
      }),
    ).toBeInTheDocument();
    await user.click(screen.getByRole('link', { name: 'Back to projects' }));
    expect(await screen.findByText('Projects list')).toBeInTheDocument();
  });
});
