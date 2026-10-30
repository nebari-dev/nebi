import { fireEvent, screen, waitFor, within } from '@testing-library/react';
import userEvent from '@testing-library/user-event';
import { HttpResponse, http } from 'msw';
import { Route, Routes } from 'react-router-dom';
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';
import { projectsApi } from '@/api/projects';
import { useAuthStore } from '@/store/authStore';
import { useModeStore } from '@/store/modeStore';
import { mockJob, mockProject, mockUser, server } from '@/test/handlers';
import { renderWithProviders } from '@/test/utils';
import { ProjectDetail } from './ProjectDetail';

const manifest = '[workspace]\nname = "test-workspace"\n';

function renderProject(canWrite?: boolean) {
  server.use(
    http.get('/api/v1/projects/ws-1', () =>
      HttpResponse.json({
        ...mockProject,
        owner_id: 'another-user',
        can_write: canWrite,
      }),
    ),
  );
  return renderWithProviders(
    <Routes>
      <Route path="/projects/:id" element={<ProjectDetail />} />
    </Routes>,
    { initialEntries: ['/projects/ws-1'] },
  );
}

beforeEach(() => {
  useAuthStore.setState({ user: mockUser });
  useModeStore.setState({ mode: 'team' });
  server.use(
    http.get('/api/v1/projects/ws-1/packages', () => HttpResponse.json([])),
    http.get('/api/v1/projects/ws-1/pixi-toml', () =>
      HttpResponse.json({ content: manifest }),
    ),
  );
});

afterEach(() => {
  vi.restoreAllMocks();
  useAuthStore.setState({ user: null });
  useModeStore.setState({ mode: null });
});

describe('Project configuration permissions', () => {
  it.each([false, undefined])(
    'keeps configuration readable and both Edit buttons disabled when can_write is %s',
    async (canWrite) => {
      const save = vi.spyOn(projectsApi, 'savePixiToml');
      const solve = vi.spyOn(projectsApi, 'solveProject');
      const user = userEvent.setup();
      renderProject(canWrite);

      const headerEdit = await screen.findByRole('button', { name: 'Edit' });
      expect(headerEdit).toBeDisabled();
      expect(headerEdit).toHaveAccessibleDescription(
        'You have read-only access. Editing the configuration requires write access.',
      );
      fireEvent.click(headerEdit);
      expect(
        screen.queryByRole('button', { name: 'Save & Install' }),
      ).toBeNull();

      await user.click(screen.getByRole('tab', { name: 'Configuration' }));
      expect(
        await screen.findByLabelText('pixi.toml contents'),
      ).toHaveTextContent('[workspace]');
      const editButtons = screen.getAllByRole('button', { name: 'Edit' });
      expect(editButtons).toHaveLength(2);
      for (const button of editButtons) {
        expect(button).toBeDisabled();
        expect(button).toHaveAccessibleDescription(/read-only access/);
        fireEvent.click(button);
      }
      expect(screen.queryByRole('textbox')).toBeNull();
      expect(
        screen.queryByRole('button', { name: 'Save & Install' }),
      ).toBeNull();
      expect(save).not.toHaveBeenCalled();
      expect(solve).not.toHaveBeenCalled();
    },
  );

  it.each(['header', 'configuration tab'])(
    'allows a user with write access to edit and save from the %s',
    async (entryPoint) => {
      const save = vi.spyOn(projectsApi, 'savePixiToml').mockResolvedValue();
      const solve = vi
        .spyOn(projectsApi, 'solveProject')
        .mockResolvedValue(mockJob);
      const user = userEvent.setup();
      renderProject(true);

      let edit = await screen.findByRole('button', { name: 'Edit' });
      if (entryPoint === 'configuration tab') {
        await user.click(screen.getByRole('tab', { name: 'Configuration' }));
        edit = await within(screen.getByRole('tabpanel')).findByRole('button', {
          name: 'Edit',
        });
      }
      expect(edit).toBeEnabled();
      await user.click(edit);
      await user.click(
        await screen.findByRole('button', { name: 'TOML Mode' }),
      );
      const editor = screen.getByRole('textbox', {
        name: 'pixi.toml Configuration',
      });
      const updated = `${manifest}\n[dependencies]\npython = "3.12.*"\n`;
      fireEvent.change(editor, { target: { value: updated } });
      await user.click(screen.getByRole('button', { name: 'Save & Install' }));

      await waitFor(() => expect(save).toHaveBeenCalledWith('ws-1', updated));
      expect(solve).toHaveBeenCalledWith('ws-1');
      expect(
        await screen.findByLabelText('pixi.toml contents'),
      ).toHaveTextContent('3.12.*');
      expect(screen.queryByText(/You have read-only access/)).toBeNull();
    },
  );
});
