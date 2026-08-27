import { screen } from '@testing-library/react';
import userEvent from '@testing-library/user-event';
import { Route, Routes } from 'react-router-dom';
import { beforeEach, describe, expect, it, vi } from 'vitest';
import { renderWithProviders } from '@/test/utils';
import type { OCIRegistry, RegistryTag } from '@/types';
import { RegistryRepositories } from './Registries';

const registry: OCIRegistry = {
  id: 'reg-1',
  name: 'Test Registry',
  url: 'registry.example.com',
  username: '',
  has_api_token: false,
  is_default: true,
  namespace: 'envs',
  config_managed: false,
  restricted: false,
  created_at: '2026-01-01T00:00:00Z',
};

let mockTags: RegistryTag[] = [];

vi.mock('@/hooks/useRegistries', () => ({
  usePublicRegistries: () => ({ data: [registry], isLoading: false }),
  useRegistryRepositories: () => ({
    data: { repositories: [{ name: 'team/app', is_public: true }] },
    isLoading: false,
  }),
  useRepositoryTags: () => ({
    data: { tags: mockTags },
    isLoading: false,
  }),
  useImportEnvironment: () => ({ mutateAsync: vi.fn(), isPending: false }),
}));

const renderPage = () =>
  renderWithProviders(
    <Routes>
      <Route
        path="/registries/:registryId"
        element={<RegistryRepositories />}
      />
    </Routes>,
    { initialEntries: ['/registries/reg-1'] },
  );

describe('RegistryRepositories', () => {
  beforeEach(() => {
    mockTags = [];
  });

  it('defaults the tag select to latest when the repository has one', () => {
    mockTags = [{ name: 'v1' }, { name: 'latest' }, { name: 'v2' }];
    renderPage();

    const select = screen.getByLabelText<HTMLSelectElement>(
      'Select tag for team/app',
    );
    expect(select.value).toBe('latest');
  });

  it('falls back to the first tag when there is no latest tag', () => {
    mockTags = [{ name: 'v1' }, { name: 'v2' }];
    renderPage();

    const select = screen.getByLabelText<HTMLSelectElement>(
      'Select tag for team/app',
    );
    expect(select.value).toBe('v1');
  });

  it('uses the default tag in the command preview and prefilled workspace name', async () => {
    mockTags = [{ name: 'v1' }, { name: 'latest' }];
    const user = userEvent.setup();
    renderPage();

    expect(screen.getByText(/team\/app:latest/)).toBeInTheDocument();

    await user.click(screen.getByRole('button', { name: /import/i }));
    expect(screen.getByLabelText('Workspace Name')).toHaveValue('app-latest');
  });

  it('toggles the import panel from the row trigger', async () => {
    mockTags = [{ name: 'latest' }];
    const user = userEvent.setup();
    renderPage();

    const trigger = screen.getByRole('button', { name: /import/i });
    expect(trigger).toHaveAttribute('aria-expanded', 'false');

    await user.click(trigger);
    expect(screen.getByLabelText('Workspace Name')).toBeInTheDocument();
    const closeTrigger = screen.getByRole('button', { name: /close/i });
    expect(closeTrigger).toHaveAttribute('aria-expanded', 'true');

    await user.click(closeTrigger);
    expect(screen.queryByLabelText('Workspace Name')).not.toBeInTheDocument();
    expect(screen.getByRole('button', { name: /import/i })).toHaveAttribute(
      'aria-expanded',
      'false',
    );
  });

  it('renders only one Import button while the panel is expanded', async () => {
    mockTags = [{ name: 'latest' }];
    const user = userEvent.setup();
    renderPage();

    await user.click(screen.getByRole('button', { name: /import/i }));

    const importButtons = screen.getAllByRole('button', { name: /import/i });
    expect(importButtons).toHaveLength(1);
  });
});
