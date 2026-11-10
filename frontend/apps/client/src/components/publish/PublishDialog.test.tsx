import { Toaster } from '@nebi/ui/components/ui/toast';
import { TooltipProvider } from '@nebi/ui/components/ui/tooltip';
import { screen, waitFor, within } from '@testing-library/react';
import userEvent from '@testing-library/user-event';
import { HttpResponse, http } from 'msw';
import { describe, expect, it, vi } from 'vitest';
import {
  mockRegistryItem,
  mockVersionItem,
  PROJECT_ID,
  REGISTRY_ID,
} from '@/test/fixtures/contracts';
import { server } from '@/test/handlers';
import {
  mockQuayPublication,
  PUBLICATION_ID,
  QUAY_REGISTRY_ID,
} from '@/test/handlers/publish';
import { renderWithProviders } from '@/test/utils';
import type { PublishRequest, VersionItem } from '@/types/contracts';
import { PublishDialog } from './PublishDialog';

const C = '/api/v1/client';

function renderDialog(version: VersionItem = mockVersionItem) {
  const onOpenChange = vi.fn();
  renderWithProviders(
    <TooltipProvider>
      <Toaster>
        <PublishDialog
          projectId={PROJECT_ID}
          projectName="Project_1"
          version={version}
          open
          onOpenChange={onOpenChange}
        />
      </Toaster>
    </TooltipProvider>,
  );
  return { onOpenChange };
}

const row = (name: string) =>
  screen.getByText(name, { selector: 'span' }).closest('li') as HTMLElement;

describe('PublishDialog', () => {
  it('lists every registry with its default repository:tag and enables Save only on change', async () => {
    renderDialog();
    expect(
      await screen.findByText('ghcr.io/example-org/project_1:7'),
    ).toBeInTheDocument();
    expect(screen.getByText('quay.io/nebari/project_1:7')).toBeInTheDocument();
    expect(
      screen.getByRole('heading', { name: 'Publish version 7' }),
    ).toBeInTheDocument();
    const save = screen.getByRole('button', { name: 'Save' });
    expect(save).toBeDisabled();

    await userEvent.click(
      within(row('Quay')).getByRole('switch', { name: 'Quay' }),
    );
    expect(save).toBeEnabled();
  });

  it('edits repository:tag with validation, then publishes and shows per-registry results', async () => {
    let body: PublishRequest | undefined;
    server.use(
      http.post(
        `${C}/projects/:id/versions/:version/publish`,
        async ({ request }) => {
          body = (await request.json()) as PublishRequest;
          return HttpResponse.json({
            results: [
              {
                registry_id: QUAY_REGISTRY_ID,
                registry_name: 'Quay',
                reference: 'quay.io/nebari/project_1:stable',
                status: 'published',
                digest: 'sha256:1',
                publication_id: PUBLICATION_ID,
              },
            ],
          });
        },
      ),
    );
    renderDialog();
    await screen.findByText('quay.io/nebari/project_1:7');
    const quay = row('Quay');
    await userEvent.click(within(quay).getByRole('switch', { name: 'Quay' }));
    await userEvent.click(within(quay).getByRole('button', { name: 'Edit' }));

    const input = screen.getByRole('textbox', { name: 'Repository and tag' });
    await userEvent.clear(input);
    await userEvent.type(input, 'nebari/project_1 x');
    expect(
      screen.getByText('Use repository:tag, e.g. nebari/project_1:5.'),
    ).toBeInTheDocument();
    expect(screen.getByRole('button', { name: 'Save' })).toBeDisabled();

    await userEvent.clear(input);
    await userEvent.type(input, 'nebari/project_1:stable');
    // A valid edit keeps Save available (prototype); it commits on blur.
    expect(screen.getByRole('button', { name: 'Save' })).toBeEnabled();
    await userEvent.type(input, '{Enter}');
    expect(
      screen.getByText('quay.io/nebari/project_1:stable'),
    ).toBeInTheDocument();

    await userEvent.click(screen.getByRole('button', { name: 'Save' }));
    const results = await screen.findByRole('list', {
      name: 'Publish results',
    });
    expect(within(results).getByText('Published')).toBeInTheDocument();
    expect(body).toEqual({
      targets: [
        {
          registry_id: QUAY_REGISTRY_ID,
          repository: 'nebari/project_1',
          tag: 'stable',
        },
      ],
    });
    expect(
      await screen.findByText('Version 7 published to quay.io'),
    ).toBeInTheDocument();
  });

  it('warns before unpublishing and reports the unpublish', async () => {
    let deleted = '';
    server.use(
      http.delete(
        `${C}/projects/:id/publications/:publicationId`,
        ({ params }) => {
          deleted = params.publicationId as string;
          return new HttpResponse(null, { status: 204 });
        },
      ),
    );
    renderDialog({ ...mockVersionItem, published: [mockQuayPublication] });
    await screen.findByText('quay.io/nebari/project_1:7');
    const quay = row('Quay');
    const toggle = within(quay).getByRole('switch', { name: 'Quay' });
    expect(toggle).toBeChecked();
    expect(
      screen.queryByText(
        'Will be unpublished. Pulls of this tag will stop working.',
      ),
    ).not.toBeInTheDocument();

    await userEvent.click(toggle);
    expect(
      screen.getByText(
        'Will be unpublished. Pulls of this tag will stop working.',
      ),
    ).toBeInTheDocument();
    await userEvent.click(screen.getByRole('button', { name: 'Save' }));
    expect(
      await screen.findByText('Version 7 unpublished from quay.io'),
    ).toBeInTheDocument();
    expect(deleted).toBe(PUBLICATION_ID);
  });

  it('locks unpublish on registries that cannot delete tags', async () => {
    renderDialog({
      ...mockVersionItem,
      published: [
        {
          publication_id: PUBLICATION_ID,
          registry_id: REGISTRY_ID,
          registry: 'ghcr.io',
          repository: 'example-org/project_1',
          tag: '7',
          reference: 'ghcr.io/example-org/project_1:7',
          source: 'local',
          can_unpublish: false,
        },
      ],
    });
    await screen.findByText('ghcr.io/example-org/project_1:7');
    const ghcr = row(mockRegistryItem.name);
    expect(
      within(ghcr).getByRole('switch', { name: mockRegistryItem.name }),
    ).toHaveAttribute('aria-disabled', 'true');
    expect(
      within(ghcr).getByText(mockRegistryItem.unpublish_reason as string),
    ).toBeInTheDocument();
  });

  it('locks publications made on another machine, named after the matching registry', async () => {
    renderDialog({
      ...mockVersionItem,
      published: [
        {
          publication_id: '99999999-9999-4999-8999-999999999999',
          registry_id: null,
          registry: 'quay.io',
          repository: 'nebari/project_1',
          tag: 'stable',
          reference: 'quay.io/nebari/project_1:stable',
          source: 'server',
          can_unpublish: false,
        },
      ],
    });
    const ref = await screen.findByText('quay.io/nebari/project_1:stable');
    const li = ref.closest('li') as HTMLElement;
    expect(within(li).getByText('Quay', { selector: 'span' })).toBeVisible();
    expect(within(li).getByRole('switch')).toHaveAttribute(
      'aria-disabled',
      'true',
    );
    expect(
      within(li).getByText(
        'Published from another machine. Unpublish it there.',
      ),
    ).toBeInTheDocument();
  });

  it('shows failures per registry and lets you go back', async () => {
    server.use(
      http.post(`${C}/projects/:id/versions/:version/publish`, () =>
        HttpResponse.json({
          results: [
            {
              registry_id: QUAY_REGISTRY_ID,
              registry_name: 'Quay',
              reference: 'quay.io/nebari/project_1:7',
              status: 'failed',
              error: 'unauthorized',
              publication_id: null,
            },
          ],
        }),
      ),
    );
    renderDialog();
    await screen.findByText('quay.io/nebari/project_1:7');
    await userEvent.click(
      within(row('Quay')).getByRole('switch', { name: 'Quay' }),
    );
    await userEvent.click(screen.getByRole('button', { name: 'Save' }));
    expect(
      await screen.findByText('Couldn’t publish: unauthorized'),
    ).toBeInTheDocument();
    expect(
      (await screen.findAllByText('Couldn’t publish version 7 to quay.io'))
        .length,
    ).toBeGreaterThan(0);
    await userEvent.click(screen.getByRole('button', { name: 'Back' }));
    expect(screen.getByRole('button', { name: 'Save' })).toBeDisabled();
  });

  it('offers to add a registry when none is set up', async () => {
    server.use(http.get(`${C}/registries`, () => HttpResponse.json([])));
    renderDialog();
    expect(await screen.findByText('No registries yet')).toBeInTheDocument();
    await userEvent.click(
      screen.getByRole('button', { name: 'Add a registry' }),
    );
    await waitFor(() =>
      expect(
        screen.getByRole('heading', { name: 'Add a registry' }),
      ).toBeInTheDocument(),
    );
  });
});
