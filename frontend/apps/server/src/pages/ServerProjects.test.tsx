import { Toaster } from '@nebi/ui/components/ui/toast';
import userEvent from '@testing-library/user-event';
import { HttpResponse, http } from 'msw';
import { beforeEach, describe, expect, it } from 'vitest';
import { useAuthStore } from '@/store/authStore';
import { mockUser, server } from '@/test/handlers';
import {
  accessOf,
  deletedProjects,
  mockJordanPrincipal,
  resetServerMock,
} from '@/test/sharingHandlers';
import { renderWithProviders, screen, waitFor, within } from '@/test/utils';
import { ServerProjects } from './ServerProjects';

const renderPage = () =>
  renderWithProviders(
    <Toaster>
      <ServerProjects />
    </Toaster>,
  );

const notAdmin = () =>
  server.use(
    http.get('/api/v1/admin/users', () =>
      HttpResponse.json({ error: 'Admin access required' }, { status: 403 }),
    ),
  );

const projectNames = () =>
  screen
    .getAllByRole('row')
    .slice(1)
    .map((r) => within(r).getAllByRole('cell')[0].textContent);

describe('ServerProjects', () => {
  beforeEach(() => {
    resetServerMock();
    useAuthStore.setState({ user: mockUser, token: 'tok' });
  });

  it('lists server projects without pull or install', async () => {
    renderPage();
    expect(
      screen.getByRole('heading', { level: 1, name: 'Server projects' }),
    ).toBeInTheDocument();
    expect(
      screen.getByText(
        'This is a server-only interface. Projects can be managed and shared here, but not pulled or installed.',
      ),
    ).toBeInTheDocument();
    expect(await screen.findByText('ml-baseline')).toBeInTheDocument();
    expect(projectNames()).toEqual([
      'jatic-cosmos',
      'ml-baseline',
      'nebi-chat',
    ]);
    expect(screen.getByText('v4')).toBeInTheDocument();
    expect(screen.getByText('team-jatic')).toBeInTheDocument();
    expect(screen.queryByRole('button', { name: /pull|install/i })).toBeNull();
    expect(screen.getAllByRole('button', { name: 'Share' })).toHaveLength(3);
  });

  it('searches and sorts', async () => {
    const user = userEvent.setup();
    renderPage();
    await screen.findByText('ml-baseline');

    await user.click(screen.getByRole('button', { name: 'Latest version' }));
    expect(projectNames()).toEqual([
      'jatic-cosmos',
      'ml-baseline',
      'nebi-chat',
    ]);
    await user.click(screen.getByRole('button', { name: 'Latest version' }));
    expect(projectNames()).toEqual([
      'nebi-chat',
      'ml-baseline',
      'jatic-cosmos',
    ]);

    await user.type(screen.getByLabelText('Search server projects'), 'chat');
    expect(projectNames()).toEqual(['nebi-chat']);
    await user.clear(screen.getByLabelText('Search server projects'));
    await user.type(screen.getByLabelText('Search server projects'), 'zzz');
    expect(
      screen.getByText('No server projects match “zzz”.'),
    ).toBeInTheDocument();
  });

  it('says when nothing is shared yet', async () => {
    resetServerMock([]);
    renderPage();
    expect(
      await screen.findByText('Nothing is shared with you on this server yet.'),
    ).toBeInTheDocument();
  });

  it('shows a load error with Retry', async () => {
    const user = userEvent.setup();
    let fail = true;
    server.use(
      http.get('/api/v1/catalog/projects', () =>
        fail
          ? HttpResponse.json({ error: 'database is down' }, { status: 500 })
          : HttpResponse.json([]),
      ),
    );
    renderPage();
    expect(await screen.findByText('database is down')).toBeInTheDocument();
    fail = false;
    await user.click(screen.getByRole('button', { name: 'Retry' }));
    expect(
      await screen.findByText('Nothing is shared with you on this server yet.'),
    ).toBeInTheDocument();
  });

  it('shares a project from the Share button, with Undo', async () => {
    const user = userEvent.setup();
    renderPage();
    const row = (await screen.findByText('ml-baseline')).closest('tr');
    if (!row) throw new Error('row not found');
    await user.click(within(row).getByRole('button', { name: 'Share' }));

    const dialog = await screen.findByRole('dialog');
    expect(
      within(dialog).getByRole('heading', { name: 'Share ml-baseline' }),
    ).toBeInTheDocument();
    expect(
      await within(dialog).findByText('testuser (you)'),
    ).toBeInTheDocument();
    await waitFor(() =>
      expect(
        within(dialog).getByPlaceholderText('Name, email or team'),
      ).toHaveFocus(),
    );

    await user.type(
      within(dialog).getByPlaceholderText('Name, email or team'),
      'jor',
    );
    await user.click(await screen.findByRole('option', { name: /jordan/ }));
    await user.click(within(dialog).getByRole('button', { name: 'Share' }));

    expect(await screen.findByText('Shared ml-baseline')).toBeInTheDocument();
    expect(
      screen.getByText('jordan can now view this project.'),
    ).toBeInTheDocument();
    expect(
      accessOf('proj-ml').entries.some(
        (e) => e.principal.id === mockJordanPrincipal.id,
      ),
    ).toBe(true);

    await user.click(screen.getByRole('button', { name: 'Undo' }));
    await waitFor(() =>
      expect(
        accessOf('proj-ml').entries.some(
          (e) => e.principal.id === mockJordanPrincipal.id,
        ),
      ).toBe(false),
    );
  });

  it('opens Manage access from the row menu', async () => {
    const user = userEvent.setup();
    renderPage();
    await screen.findByText('ml-baseline');
    await user.click(
      screen.getByRole('button', { name: 'More actions for nebi-chat' }),
    );
    await user.click(
      await screen.findByRole('menuitem', { name: 'Manage access' }),
    );
    expect(
      await screen.findByRole('heading', { name: 'Share nebi-chat' }),
    ).toBeInTheDocument();
    expect(await screen.findByText('Team, 6 members')).toBeInTheDocument();
  });

  it('deletes a project from the server after confirmation', async () => {
    const user = userEvent.setup();
    renderPage();
    await screen.findByText('ml-baseline');
    await user.click(
      screen.getByRole('button', { name: 'More actions for ml-baseline' }),
    );
    await user.click(
      await screen.findByRole('menuitem', { name: 'Delete from server…' }),
    );
    const confirm = await screen.findByRole('alertdialog');
    expect(
      within(confirm).getByText('Delete ml-baseline from the server?'),
    ).toBeInTheDocument();
    expect(deletedProjects).toEqual([]);
    await user.click(
      within(confirm).getByRole('button', { name: 'Delete from server' }),
    );

    expect(
      await screen.findByText('Deleted ml-baseline from the server'),
    ).toBeInTheDocument();
    expect(deletedProjects).toEqual(['proj-ml']);
    await waitFor(() => expect(screen.queryByText('ml-baseline')).toBeNull());
  });

  it('cancelling the delete confirmation keeps the project', async () => {
    const user = userEvent.setup();
    renderPage();
    await screen.findByText('ml-baseline');
    await user.click(
      screen.getByRole('button', { name: 'More actions for ml-baseline' }),
    );
    await user.click(
      await screen.findByRole('menuitem', { name: 'Delete from server…' }),
    );
    await user.click(
      within(await screen.findByRole('alertdialog')).getByRole('button', {
        name: 'Cancel',
      }),
    );
    expect(deletedProjects).toEqual([]);
    expect(screen.getByText('ml-baseline')).toBeInTheDocument();
  });

  it('only owners and admins can delete; viewers see access read-only', async () => {
    const user = userEvent.setup();
    notAdmin();
    renderPage();
    await screen.findByText('jatic-cosmos');
    await user.click(
      screen.getByRole('button', { name: 'More actions for jatic-cosmos' }),
    );
    expect(
      await screen.findByRole('menuitem', { name: 'Manage access' }),
    ).toBeInTheDocument();
    expect(
      screen.queryByRole('menuitem', { name: 'Delete from server…' }),
    ).toBeNull();
    await user.click(screen.getByRole('menuitem', { name: 'Manage access' }));
    expect(
      await screen.findByText(
        'Only the project owner or an admin can change who has access.',
      ),
    ).toBeInTheDocument();
    expect(screen.queryByPlaceholderText('Name, email or team')).toBeNull();
  });

  it('reports a refused delete', async () => {
    const user = userEvent.setup();
    server.use(
      http.delete('/api/v1/catalog/projects/:id', () =>
        HttpResponse.json(
          {
            error:
              'Only the project owner or an admin can delete it from the server',
            code: 'forbidden',
          },
          { status: 403 },
        ),
      ),
    );
    renderPage();
    await screen.findByText('ml-baseline');
    await user.click(
      screen.getByRole('button', { name: 'More actions for ml-baseline' }),
    );
    await user.click(
      await screen.findByRole('menuitem', { name: 'Delete from server…' }),
    );
    await user.click(
      within(await screen.findByRole('alertdialog')).getByRole('button', {
        name: 'Delete from server',
      }),
    );
    expect(
      (await screen.findAllByText('Couldn’t delete ml-baseline'))[0],
    ).toBeInTheDocument();
    expect(screen.getByText('ml-baseline')).toBeInTheDocument();
  });
});
