import { screen } from '@testing-library/react';
import { HttpResponse, http } from 'msw';
import { describe, expect, it } from 'vitest';
import { server } from '@/test/handlers';
import { renderWithProviders } from '@/test/utils';
import { VersionHistory } from './VersionHistory';

describe('VersionHistory', () => {
  it('uses the pixi.toml version as the primary version label', async () => {
    server.use(
      http.get('/api/v1/workspaces/:id/versions', () =>
        HttpResponse.json([
          {
            id: 'version-1',
            workspace_id: 'workspace-1',
            version_number: 1,
            manifest_version: '0.0.3',
            created_at: '2026-08-14T07:35:38Z',
            created_by: 'user-1',
            description: 'Initial workspace creation',
          },
        ]),
      ),
    );

    renderWithProviders(
      <VersionHistory environmentId="workspace-1" environmentStatus="ready" />,
    );

    expect(
      await screen.findByText('Workspace version 0.0.3'),
    ).toBeInTheDocument();
    expect(screen.getByText('Snapshot 1')).toBeInTheDocument();
    expect(
      screen.queryByRole('heading', { name: 'Version 1' }),
    ).not.toBeInTheDocument();
  });
});
