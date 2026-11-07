import { act, screen } from '@testing-library/react';
import { HttpResponse, http } from 'msw';
import { describe, expect, it, vi } from 'vitest';
import { server } from '@/test/handlers';
import { renderWithProviders } from '@/test/utils';
import { VersionHistory } from './VersionHistory';

describe('VersionHistory', () => {
  it('shows versions saved externally without reopening the tab', async ({
    onTestFinished,
  }) => {
    vi.useFakeTimers({ toFake: ['setInterval', 'clearInterval'] });
    onTestFinished(() => {
      vi.useRealTimers();
    });

    const firstVersion = {
      id: 'version-1',
      project_id: 'project-1',
      version_number: 1,
      created_at: '2026-08-14T07:35:38Z',
    };
    const versions = [firstVersion];
    server.use(
      http.get('/api/v1/projects/:id/versions', () =>
        HttpResponse.json(versions),
      ),
    );

    renderWithProviders(
      <VersionHistory environmentId="project-1" environmentStatus="ready" />,
    );
    expect(await screen.findByText('Snapshot 1')).toBeInTheDocument();

    versions.unshift({ ...firstVersion, id: 'version-2', version_number: 2 });
    await act(() => vi.advanceTimersByTimeAsync(30_000));

    expect(await screen.findByText('Snapshot 2')).toBeInTheDocument();
  });

  it('uses the pixi.toml version as the primary version label', async () => {
    server.use(
      http.get('/api/v1/projects/:id/versions', () =>
        HttpResponse.json([
          {
            id: 'version-1',
            project_id: 'project-1',
            version_number: 1,
            manifest_version: '0.0.3',
            created_at: '2026-08-14T07:35:38Z',
            created_by: 'user-1',
            description: 'Initial project creation',
          },
        ]),
      ),
    );

    renderWithProviders(
      <VersionHistory environmentId="project-1" environmentStatus="ready" />,
    );

    expect(
      await screen.findByText('Project version 0.0.3'),
    ).toBeInTheDocument();
    expect(screen.getByText('Snapshot 1')).toBeInTheDocument();
    expect(
      screen.queryByRole('heading', { name: 'Version 1' }),
    ).not.toBeInTheDocument();
  });
});
