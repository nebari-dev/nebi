import { Toaster } from '@nebi/ui/components/ui/toast';
import { TooltipProvider } from '@nebi/ui/components/ui/tooltip';
import { screen, waitFor, within } from '@testing-library/react';
import userEvent from '@testing-library/user-event';
import { HttpResponse, http } from 'msw';
import { describe, expect, it } from 'vitest';
import { mockRegistryItem } from '@/test/fixtures/contracts';
import { server } from '@/test/handlers';
import { QUAY_REGISTRY_ID } from '@/test/handlers/publish';
import { renderWithProviders } from '@/test/utils';
import { RegistriesPage } from './RegistriesPage';

const C = '/api/v1/client';

const renderPage = () =>
  renderWithProviders(
    <TooltipProvider>
      <Toaster>
        <RegistriesPage />
      </Toaster>
    </TooltipProvider>,
    { initialEntries: ['/registries'] },
  );

describe('RegistriesPage', () => {
  it('lists registries with their default, credentials and unpublish support', async () => {
    renderPage();
    expect(
      screen.getByRole('heading', { level: 1, name: 'Registries' }),
    ).toBeInTheDocument();
    const table = await screen.findByRole('table', { name: 'Registries' });
    const ghcr = within(table)
      .getByText(mockRegistryItem.name)
      .closest('tr') as HTMLElement;
    expect(within(ghcr).getByText('Default')).toBeInTheDocument();
    expect(within(ghcr).getByText('Signed in as sam')).toBeInTheDocument();
    expect(within(ghcr).getByText('Not supported')).toBeInTheDocument();
    const quay = within(table).getByText('Quay').closest('tr') as HTMLElement;
    expect(within(quay).getByText('Supported')).toBeInTheDocument();
    expect(
      within(quay).getByText('None', { selector: 'span' }),
    ).toBeInTheDocument();
  });

  it('filters by search', async () => {
    renderPage();
    await screen.findByRole('table', { name: 'Registries' });
    await userEvent.type(
      screen.getByRole('searchbox', { name: 'Search registries' }),
      'quay',
    );
    expect(screen.queryByText(mockRegistryItem.name)).not.toBeInTheDocument();
    await userEvent.clear(
      screen.getByRole('searchbox', { name: 'Search registries' }),
    );
    await userEvent.type(
      screen.getByRole('searchbox', { name: 'Search registries' }),
      'zzz',
    );
    expect(screen.getByText('No registries match “zzz”.')).toBeInTheDocument();
  });

  it('removes a registry after confirmation', async () => {
    let deleted = '';
    server.use(
      http.delete(`${C}/registries/:registryId`, ({ params }) => {
        deleted = params.registryId as string;
        return new HttpResponse(null, { status: 204 });
      }),
    );
    renderPage();
    await screen.findByRole('table', { name: 'Registries' });
    await userEvent.click(
      screen.getByRole('button', { name: 'Actions for Quay' }),
    );
    await userEvent.click(
      await screen.findByRole('menuitem', { name: 'Remove…' }),
    );
    const dialog = await screen.findByRole('alertdialog');
    expect(within(dialog).getByText('Remove Quay?')).toBeInTheDocument();
    await userEvent.click(
      within(dialog).getByRole('button', { name: 'Remove' }),
    );
    await waitFor(() => expect(deleted).toBe(QUAY_REGISTRY_ID));
    expect(await screen.findByText('Quay removed')).toBeInTheDocument();
  });

  it('makes a registry the default', async () => {
    let body: Record<string, unknown> | undefined;
    server.use(
      http.put(`${C}/registries/:registryId`, async ({ request }) => {
        body = (await request.json()) as Record<string, unknown>;
        return HttpResponse.json({ ...mockRegistryItem, ...body });
      }),
    );
    renderPage();
    await screen.findByRole('table', { name: 'Registries' });
    await userEvent.click(
      screen.getByRole('button', { name: 'Actions for Quay' }),
    );
    await userEvent.click(
      await screen.findByRole('menuitem', { name: 'Make default' }),
    );
    await waitFor(() => expect(body).toEqual({ is_default: true }));
  });

  it('opens edit and pull dialogs from the row menu', async () => {
    renderPage();
    await screen.findByRole('table', { name: 'Registries' });
    await userEvent.click(
      screen.getByRole('button', { name: 'Actions for Quay' }),
    );
    await userEvent.click(
      await screen.findByRole('menuitem', { name: 'Edit' }),
    );
    expect(
      await screen.findByRole('heading', { name: 'Edit registry' }),
    ).toBeInTheDocument();
    expect(screen.getByLabelText('Name')).toHaveValue('Quay');
    await userEvent.click(screen.getByRole('button', { name: 'Cancel' }));

    await userEvent.click(
      screen.getByRole('button', { name: 'Pull from registry' }),
    );
    expect(
      await screen.findByRole('heading', { name: 'Pull from registry' }),
    ).toBeInTheDocument();
  });

  it('shows the empty state with Add a registry', async () => {
    server.use(http.get(`${C}/registries`, () => HttpResponse.json([])));
    renderPage();
    expect(await screen.findByText('No registries yet')).toBeInTheDocument();
    await userEvent.click(
      screen.getByRole('button', { name: 'Add a registry' }),
    );
    expect(
      await screen.findByRole('heading', { name: 'Add a registry' }),
    ).toBeInTheDocument();
  });
});
