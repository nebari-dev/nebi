import { Toaster } from '@nebi/ui/components/ui/toast';
import { screen, waitFor } from '@testing-library/react';
import userEvent from '@testing-library/user-event';
import { HttpResponse, http } from 'msw';
import type { ReactElement } from 'react';
import { describe, expect, it, vi } from 'vitest';
import {
  mockRegistryItem,
  PROJECT_ID,
  REGISTRY_ID,
} from '@/test/fixtures/contracts';
import { server } from '@/test/handlers';
import { mockPullResult } from '@/test/handlers/publish';
import { renderWithProviders } from '@/test/utils';
import type {
  AddRegistryRequest,
  RegistryPullRequest,
} from '@/types/contracts';
import { AddRegistryDialog } from './AddRegistryDialog';
import { PullFromRegistryDialog } from './PullFromRegistryDialog';

const C = '/api/v1/client';

const renderWithToaster = (ui: ReactElement) =>
  renderWithProviders(<Toaster>{ui}</Toaster>);

describe('AddRegistryDialog', () => {
  it('validates the form before saving', async () => {
    renderWithToaster(<AddRegistryDialog open onOpenChange={vi.fn()} />);
    await userEvent.click(screen.getByRole('button', { name: 'Add registry' }));
    expect(screen.getByText('Enter a registry name.')).toBeInTheDocument();
    expect(screen.getByText('Enter the registry’s URL.')).toBeInTheDocument();

    await userEvent.type(screen.getByLabelText('Registry URL'), 'not a url');
    await userEvent.click(screen.getByRole('button', { name: 'Add registry' }));
    expect(
      screen.getByText('That doesn’t look like a registry URL. Try ghcr.io.'),
    ).toBeInTheDocument();

    await userEvent.click(
      screen.getByRole('tab', { name: 'Username and password' }),
    );
    await userEvent.click(screen.getByRole('button', { name: 'Add registry' }));
    expect(
      screen.getByText('Enter a username, or choose None.'),
    ).toBeInTheDocument();
    expect(
      screen.getByText('Enter a password or token, or choose None.'),
    ).toBeInTheDocument();
  });

  it('adds a registry with credentials and reports it', async () => {
    let body: AddRegistryRequest | undefined;
    server.use(
      http.post(`${C}/registries`, async ({ request }) => {
        body = (await request.json()) as AddRegistryRequest;
        return HttpResponse.json(
          { ...mockRegistryItem, id: 'new', name: body.name },
          { status: 201 },
        );
      }),
    );
    const onAdded = vi.fn();
    const onOpenChange = vi.fn();
    renderWithToaster(
      <AddRegistryDialog open onOpenChange={onOpenChange} onAdded={onAdded} />,
    );
    await userEvent.type(screen.getByLabelText('Name'), 'Quay');
    await userEvent.type(screen.getByLabelText('Registry URL'), 'quay.io');
    await userEvent.type(
      screen.getByLabelText('Namespace (optional)'),
      'nebari',
    );
    await userEvent.click(
      screen.getByRole('tab', { name: 'Username and password' }),
    );
    await userEvent.type(screen.getByLabelText('Username'), 'sam');
    await userEvent.type(screen.getByLabelText('Password or token'), 'secret');
    await userEvent.click(
      screen.getByRole('switch', { name: 'Default registry' }),
    );
    await userEvent.click(screen.getByRole('button', { name: 'Add registry' }));

    await waitFor(() => expect(onAdded).toHaveBeenCalled());
    expect(body).toEqual({
      name: 'Quay',
      url: 'quay.io',
      namespace: 'nebari',
      username: 'sam',
      password: 'secret',
      is_default: true,
    });
    expect(onOpenChange).toHaveBeenCalledWith(false);
    expect(await screen.findByText('Quay added')).toBeInTheDocument();
  });

  it('shows server field errors next to the field', async () => {
    server.use(
      http.post(`${C}/registries`, () =>
        HttpResponse.json(
          {
            error: 'A registry called Quay already exists.',
            code: 'invalid',
            field: 'name',
          },
          { status: 422 },
        ),
      ),
    );
    renderWithToaster(<AddRegistryDialog open onOpenChange={vi.fn()} />);
    await userEvent.type(screen.getByLabelText('Name'), 'Quay');
    await userEvent.type(screen.getByLabelText('Registry URL'), 'quay.io');
    await userEvent.click(screen.getByRole('button', { name: 'Add registry' }));
    expect(
      await screen.findByText('A registry called Quay already exists.'),
    ).toBeInTheDocument();
  });

  it('edits a registry and keeps the saved password when left blank', async () => {
    let body: Record<string, unknown> | undefined;
    let path = '';
    server.use(
      http.put(`${C}/registries/:registryId`, async ({ request, params }) => {
        body = (await request.json()) as Record<string, unknown>;
        path = params.registryId as string;
        return HttpResponse.json({ ...mockRegistryItem, ...body });
      }),
    );
    renderWithToaster(
      <AddRegistryDialog
        open
        onOpenChange={vi.fn()}
        registry={mockRegistryItem}
      />,
    );
    expect(
      screen.getByRole('heading', { name: 'Edit registry' }),
    ).toBeInTheDocument();
    expect(screen.getByLabelText('Name')).toHaveValue(mockRegistryItem.name);
    expect(
      screen.getByText('Leave blank to keep the saved password.'),
    ).toBeInTheDocument();
    const ns = screen.getByLabelText('Namespace (optional)');
    await userEvent.clear(ns);
    await userEvent.type(ns, 'other-org');
    await userEvent.click(screen.getByRole('button', { name: 'Save changes' }));
    await waitFor(() => expect(body).toBeDefined());
    expect(path).toBe(REGISTRY_ID);
    expect(body).toMatchObject({ namespace: 'other-org', username: 'sam' });
    expect(body).not.toHaveProperty('password');
    expect(await screen.findByText('Registry updated')).toBeInTheDocument();
  });
});

describe('PullFromRegistryDialog', () => {
  it('pulls a repository:tag into a new project named after the repository', async () => {
    let body: RegistryPullRequest | undefined;
    let regId = '';
    server.use(
      http.post(
        `${C}/registries/:registryId/pull`,
        async ({ request, params }) => {
          body = (await request.json()) as RegistryPullRequest;
          regId = params.registryId as string;
          return HttpResponse.json(mockPullResult, { status: 202 });
        },
      ),
    );
    const onPulled = vi.fn();
    renderWithToaster(
      <PullFromRegistryDialog
        open
        onOpenChange={vi.fn()}
        onPulled={onPulled}
      />,
    );
    // The default registry is preselected.
    expect(
      await screen.findByText(mockRegistryItem.name, {
        selector: '[data-slot=select-value]',
      }),
    ).toBeInTheDocument();
    await userEvent.type(
      screen.getByRole('combobox', { name: 'Repository' }),
      'nebari/ml-baseline',
    );
    await userEvent.tab();
    await userEvent.type(screen.getByRole('combobox', { name: 'Tag' }), '5');
    await userEvent.tab();
    expect(screen.getByLabelText('Local name')).toHaveValue('ml-baseline');
    await userEvent.click(
      screen.getByRole('button', { name: 'Pull and install' }),
    );
    await waitFor(() => expect(onPulled).toHaveBeenCalledWith(PROJECT_ID));
    expect(regId).toBe(REGISTRY_ID);
    expect(body).toEqual({
      repository: 'nebari/ml-baseline',
      tag: '5',
      local_name: 'ml-baseline',
      install: true,
    });
  });

  it('shows a name collision on the local name field', async () => {
    server.use(
      http.post(`${C}/registries/:registryId/pull`, () =>
        HttpResponse.json(
          {
            error:
              'A project called ml-baseline is already on this machine. Pick another name.',
            code: 'name_taken',
          },
          { status: 409 },
        ),
      ),
    );
    renderWithToaster(<PullFromRegistryDialog open onOpenChange={vi.fn()} />);
    await screen.findByText(mockRegistryItem.name, {
      selector: '[data-slot=select-value]',
    });
    await userEvent.type(
      screen.getByRole('combobox', { name: 'Repository' }),
      'nebari/ml-baseline',
    );
    await userEvent.tab();
    await userEvent.type(screen.getByRole('combobox', { name: 'Tag' }), '5');
    await userEvent.tab();
    await userEvent.click(screen.getByRole('button', { name: 'Pull' }));
    expect(
      await screen.findByText(
        'A project called ml-baseline is already on this machine. Pick another name.',
      ),
    ).toBeInTheDocument();
  });

  it('asks for the repository and tag', async () => {
    renderWithToaster(<PullFromRegistryDialog open onOpenChange={vi.fn()} />);
    await screen.findByText(mockRegistryItem.name, {
      selector: '[data-slot=select-value]',
    });
    await userEvent.click(screen.getByRole('button', { name: 'Pull' }));
    expect(screen.getByText('Choose a repository.')).toBeInTheDocument();
    expect(screen.getByText('Choose a tag.')).toBeInTheDocument();
  });

  it('offers to add a registry when none is set up', async () => {
    server.use(http.get(`${C}/registries`, () => HttpResponse.json([])));
    renderWithToaster(<PullFromRegistryDialog open onOpenChange={vi.fn()} />);
    expect(await screen.findByText('No registries yet')).toBeInTheDocument();
    expect(
      screen.getByRole('button', { name: 'Add a registry' }),
    ).toBeInTheDocument();
    expect(
      screen.queryByRole('button', { name: 'Pull' }),
    ).not.toBeInTheDocument();
  });
});
