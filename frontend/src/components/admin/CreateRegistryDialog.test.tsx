import { screen, waitFor } from '@testing-library/react';
import userEvent from '@testing-library/user-event';
import { HttpResponse, http } from 'msw';
import { describe, expect, it } from 'vitest';
import { server } from '@/test/handlers';
import { renderWithProviders } from '@/test/utils';
import { CreateRegistryDialog } from './CreateRegistryDialog';

async function fillAndSubmit(user: ReturnType<typeof userEvent.setup>) {
  await user.click(screen.getByRole('button', { name: 'Add Registry' }));
  await user.type(screen.getByLabelText('Name'), 'GHCR');
  await user.type(screen.getByLabelText('Registry URL'), 'ghcr.io');
  await user.type(screen.getByLabelText('Namespace'), 'nebari');
  const submitButtons = screen.getAllByRole('button', { name: 'Add Registry' });
  await user.click(submitButtons[submitButtons.length - 1]);
}

describe('CreateRegistryDialog', () => {
  it('creates a local registry when isRemote is not set', async () => {
    let hitLocal = false;
    server.use(
      http.post('/api/v1/admin/registries', () => {
        hitLocal = true;
        return HttpResponse.json({ id: 'reg-1' }, { status: 201 });
      }),
      http.post('/api/v1/remote/admin/registries', () =>
        HttpResponse.json({ error: 'should not be called' }, { status: 500 }),
      ),
    );
    const user = userEvent.setup();
    renderWithProviders(<CreateRegistryDialog />);

    await fillAndSubmit(user);

    await waitFor(() => expect(hitLocal).toBe(true));
  });

  it('creates a remote registry when isRemote is true', async () => {
    let hitRemote = false;
    server.use(
      http.post('/api/v1/remote/admin/registries', () => {
        hitRemote = true;
        return HttpResponse.json({ id: 'reg-1' }, { status: 201 });
      }),
      http.post('/api/v1/admin/registries', () =>
        HttpResponse.json({ error: 'should not be called' }, { status: 500 }),
      ),
    );
    const user = userEvent.setup();
    renderWithProviders(<CreateRegistryDialog isRemote />);

    await fillAndSubmit(user);

    await waitFor(() => expect(hitRemote).toBe(true));
  });
});
