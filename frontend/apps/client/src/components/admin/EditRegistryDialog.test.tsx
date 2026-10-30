import { screen, waitFor } from '@testing-library/react';
import userEvent from '@testing-library/user-event';
import { HttpResponse, http } from 'msw';
import { describe, expect, it } from 'vitest';
import { server } from '@/test/handlers';
import { renderWithProviders } from '@/test/utils';
import type { OCIRegistry } from '@/types';
import { EditRegistryDialog } from './EditRegistryDialog';

const registry: OCIRegistry = {
  id: 'reg-1',
  name: 'GHCR',
  url: 'ghcr.io',
  username: '',
  has_api_token: false,
  is_default: false,
  namespace: 'nebari',
  config_managed: false,
  restricted: false,
  created_at: '2026-01-01T00:00:00Z',
};

async function submit(user: ReturnType<typeof userEvent.setup>) {
  await user.click(screen.getByRole('button', { name: 'Update Registry' }));
}

describe('EditRegistryDialog', () => {
  it('updates the local registry when isRemote is not set', async () => {
    let hitLocal = false;
    server.use(
      http.put('/api/v1/admin/registries/reg-1', () => {
        hitLocal = true;
        return HttpResponse.json({ id: 'reg-1' }, { status: 200 });
      }),
      http.put('/api/v1/remote/admin/registries/reg-1', () =>
        HttpResponse.json({ error: 'should not be called' }, { status: 500 }),
      ),
    );
    const user = userEvent.setup();
    renderWithProviders(
      <EditRegistryDialog registry={registry} open onOpenChange={() => {}} />,
    );

    await submit(user);

    await waitFor(() => expect(hitLocal).toBe(true));
  });

  it('updates the remote registry when isRemote is true', async () => {
    let hitRemote = false;
    server.use(
      http.put('/api/v1/remote/admin/registries/reg-1', () => {
        hitRemote = true;
        return HttpResponse.json({ id: 'reg-1' }, { status: 200 });
      }),
      http.put('/api/v1/admin/registries/reg-1', () =>
        HttpResponse.json({ error: 'should not be called' }, { status: 500 }),
      ),
    );
    const user = userEvent.setup();
    renderWithProviders(
      <EditRegistryDialog
        registry={registry}
        open
        onOpenChange={() => {}}
        isRemote
      />,
    );

    await submit(user);

    await waitFor(() => expect(hitRemote).toBe(true));
  });
});
