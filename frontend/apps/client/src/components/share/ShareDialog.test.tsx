import { Toaster } from '@nebi/ui/components/ui/toast';
import userEvent from '@testing-library/user-event';
import { HttpResponse, http } from 'msw';
import { beforeEach, describe, expect, it } from 'vitest';
import {
  mockServerConnection,
  SERVER_PROJECT_ID,
} from '@/test/fixtures/contracts';
import { server } from '@/test/handlers';
import {
  mockAlex,
  mockJordan,
  mockSam,
  mockTeamJatic,
  resetSharingMock,
  sharingMockState,
} from '@/test/handlers/sharing';
import { renderWithProviders, screen, waitFor, within } from '@/test/utils';
import { ShareDialog } from './ShareDialog';
import { principalDetail, principalInitials } from './shareModel';

const C = '/api/v1/client';

const renderDialog = () =>
  renderWithProviders(
    <Toaster>
      <ShareDialog
        serverProjectId={SERVER_PROJECT_ID}
        projectName="ml-baseline"
        open
        onOpenChange={() => {}}
      />
    </Toaster>,
  );

const dialog = () => screen.getByRole('dialog');

const roleOf = (state: ReturnType<typeof sharingMockState>, id: string) =>
  state.entries.find((e) => e.principal.id === id)?.role;

describe('ShareDialog', () => {
  beforeEach(() => {
    resetSharingMock();
    server.use(
      http.get(`${C}/remote/server`, () =>
        HttpResponse.json({ ...mockServerConnection, username: 'Sam Garcia' }),
      ),
    );
  });

  it('shows who has access with the owner, roles and the current user', async () => {
    renderDialog();
    expect(
      await screen.findByRole('heading', { name: 'Share ml-baseline' }),
    ).toBeInTheDocument();
    expect(
      screen.getByText(
        'People and teams you add can open this project on the server.',
      ),
    ).toBeInTheDocument();
    expect(await screen.findByText('Sam Garcia (you)')).toBeInTheDocument();
    expect(screen.getByText('Owner')).toBeInTheDocument();
    expect(screen.getByText('Team, 6 members')).toBeInTheDocument();
    expect(
      screen.getByRole('button', { name: 'Access for team-jatic: Can edit' }),
    ).toBeInTheDocument();
    expect(
      screen.getByRole('button', { name: 'Access for Alex Kim: Can view' }),
    ).toBeInTheDocument();
    expect(
      screen.getByRole('button', { name: 'Role for people you add: Can view' }),
    ).toBeInTheDocument();
    expect(screen.getByRole('button', { name: 'Share' })).toBeDisabled();
  });

  it('adds a person by search with the chosen role, and Undo removes them', async () => {
    const user = userEvent.setup();
    renderDialog();
    await screen.findByText('Alex Kim');

    await user.type(screen.getByPlaceholderText('Name, email or team'), 'jo');
    const option = await screen.findByRole('option', { name: /Jordan Lee/ });
    expect(within(option).getByText('jordan@example.com')).toBeInTheDocument();
    // People who already have access are not offered again.
    expect(screen.queryByRole('option', { name: /Alex Kim/ })).toBeNull();
    await user.click(option);

    await user.click(
      screen.getByRole('button', { name: 'Role for people you add: Can view' }),
    );
    await user.click(
      await screen.findByRole('menuitemcheckbox', { name: 'Can edit' }),
    );
    await user.click(within(dialog()).getByRole('button', { name: 'Share' }));

    expect(await screen.findByText('Shared ml-baseline')).toBeInTheDocument();
    expect(
      screen.getByText('Jordan Lee can now edit this project.'),
    ).toBeInTheDocument();
    expect(roleOf(sharingMockState(), mockJordan.id)).toBe('edit');
    expect(
      await screen.findByRole('button', {
        name: 'Access for Jordan Lee: Can edit',
      }),
    ).toBeInTheDocument();
    expect(screen.getByPlaceholderText('Name, email or team')).toHaveValue('');

    await user.click(screen.getByRole('button', { name: 'Undo' }));
    await waitFor(() =>
      expect(roleOf(sharingMockState(), mockJordan.id)).toBeUndefined(),
    );
    await waitFor(() => expect(screen.queryByText('Jordan Lee')).toBeNull());
  });

  it('shares a team by pressing Enter', async () => {
    const user = userEvent.setup();
    resetSharingMock({
      entries: [{ principal: mockSam, role: 'owner' }],
      can_manage: true,
    });
    renderDialog();
    await screen.findByText('Sam Garcia (you)');
    const input = screen.getByPlaceholderText('Name, email or team');
    await user.type(input, 'team-j');
    await user.click(await screen.findByRole('option', { name: /team-jatic/ }));
    await user.type(input, '{Enter}');
    expect(await screen.findByText('Shared ml-baseline')).toBeInTheDocument();
    expect(
      screen.getByText('team-jatic can now view this project.'),
    ).toBeInTheDocument();
    expect(roleOf(sharingMockState(), mockTeamJatic.id)).toBe('view');
  });

  it('says when nobody matches the search', async () => {
    const user = userEvent.setup();
    renderDialog();
    await screen.findByText('Alex Kim');
    await user.type(screen.getByPlaceholderText('Name, email or team'), 'zzz');
    expect(
      await screen.findByText(
        'No one by that name has signed in to this server.',
      ),
    ).toBeInTheDocument();
  });

  it('changes a role with an Undo that restores it', async () => {
    const user = userEvent.setup();
    renderDialog();
    await user.click(
      await screen.findByRole('button', {
        name: 'Access for Alex Kim: Can view',
      }),
    );
    await user.click(
      await screen.findByRole('menuitemcheckbox', { name: 'Can edit' }),
    );

    expect(await screen.findByText('Access updated')).toBeInTheDocument();
    expect(
      screen.getByText('Alex Kim can now edit ml-baseline.'),
    ).toBeInTheDocument();
    expect(roleOf(sharingMockState(), mockAlex.id)).toBe('edit');

    await user.click(screen.getByRole('button', { name: 'Undo' }));
    await waitFor(() =>
      expect(roleOf(sharingMockState(), mockAlex.id)).toBe('view'),
    );
    expect(
      await screen.findByRole('button', {
        name: 'Access for Alex Kim: Can view',
      }),
    ).toBeInTheDocument();
  });

  it('removes access with an Undo that gives it back', async () => {
    const user = userEvent.setup();
    renderDialog();
    await user.click(
      await screen.findByRole('button', {
        name: 'Access for team-jatic: Can edit',
      }),
    );
    await user.click(
      await screen.findByRole('menuitem', { name: 'Remove access' }),
    );

    expect(await screen.findByText('Access removed')).toBeInTheDocument();
    expect(
      screen.getByText('team-jatic can no longer open ml-baseline.'),
    ).toBeInTheDocument();
    expect(roleOf(sharingMockState(), mockTeamJatic.id)).toBeUndefined();

    await user.click(screen.getByRole('button', { name: 'Undo' }));
    await waitFor(() =>
      expect(roleOf(sharingMockState(), mockTeamJatic.id)).toBe('edit'),
    );
  });

  it('is read-only for people who cannot manage access', async () => {
    resetSharingMock({
      entries: [
        { principal: mockSam, role: 'owner' },
        { principal: mockAlex, role: 'view' },
      ],
      can_manage: false,
    });
    renderDialog();
    expect(await screen.findByText('Can view')).toBeInTheDocument();
    expect(
      screen.getByText(
        'Only the project owner or an admin can change who has access.',
      ),
    ).toBeInTheDocument();
    expect(screen.queryByPlaceholderText('Name, email or team')).toBeNull();
    expect(screen.queryByRole('button', { name: /Access for/ })).toBeNull();
  });

  it('shows the server error when a change is refused', async () => {
    const user = userEvent.setup();
    server.use(
      http.put(`${C}/remote/projects/:id/access/:kind/:pid`, () =>
        HttpResponse.json(
          {
            error: 'Only the project owner or an admin can manage access',
            code: 'server_forbidden',
          },
          { status: 403 },
        ),
      ),
    );
    renderDialog();
    await user.click(
      await screen.findByRole('button', {
        name: 'Access for Alex Kim: Can view',
      }),
    );
    await user.click(
      await screen.findByRole('menuitemcheckbox', { name: 'Can edit' }),
    );
    // Error toasts are also announced, so the text can appear twice.
    expect(
      (await screen.findAllByText('Couldn’t update access'))[0],
    ).toBeInTheDocument();
    expect(
      screen.getAllByText(
        'Only the project owner or an admin can manage access',
      )[0],
    ).toBeInTheDocument();
    expect(roleOf(sharingMockState(), mockAlex.id)).toBe('view');
  });

  it('offers Retry when the list cannot load', async () => {
    const user = userEvent.setup();
    let fail = true;
    server.use(
      http.get(`${C}/remote/projects/:id/access`, () =>
        fail
          ? HttpResponse.json(
              { error: 'server unreachable', code: 'server_unreachable' },
              { status: 502 },
            )
          : HttpResponse.json(sharingMockState()),
      ),
    );
    renderDialog();
    expect(await screen.findByText('server unreachable')).toBeInTheDocument();
    fail = false;
    await user.click(screen.getByRole('button', { name: 'Retry' }));
    expect(await screen.findByText('Alex Kim')).toBeInTheDocument();
  });
});

describe('shareModel', () => {
  it('formats initials and details like the prototype', () => {
    expect(principalInitials('Sam Garcia')).toBe('SG');
    expect(principalInitials('team-jatic')).toBe('TJ');
    expect(principalInitials('sam')).toBe('SA');
    expect(principalDetail(mockTeamJatic)).toBe('Team, 6 members');
    expect(principalDetail({ ...mockTeamJatic, members: 1 })).toBe(
      'Team, 1 member',
    );
    expect(principalDetail(mockJordan)).toBe('jordan@example.com');
  });
});
