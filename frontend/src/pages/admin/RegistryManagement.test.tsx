import { screen } from '@testing-library/react';
import { describe, expect, it, vi } from 'vitest';
import { renderWithProviders } from '@/test/utils';
import type { OCIRegistry } from '@/types';
import { RegistryManagement } from './RegistryManagement';

const managedRegistry: OCIRegistry = {
  id: '11111111-1111-1111-1111-111111111111',
  name: 'acme-managed',
  url: 'registry.acme.com',
  username: 'svc',
  has_api_token: false,
  is_default: true,
  namespace: 'acme-envs',
  config_managed: true,
  created_at: '2026-01-01T00:00:00Z',
};

const userRegistry: OCIRegistry = {
  id: '22222222-2222-2222-2222-222222222222',
  name: 'personal',
  url: 'ghcr.io',
  username: '',
  has_api_token: false,
  is_default: false,
  namespace: 'me',
  config_managed: false,
  created_at: '2026-01-02T00:00:00Z',
};

vi.mock('@/hooks/useRegistries', () => ({
  useRegistries: () => ({
    data: [managedRegistry, userRegistry],
    isLoading: false,
  }),
  useDeleteRegistry: () => ({ mutateAsync: vi.fn(), isPending: false }),
  useCreateRegistry: () => ({ mutateAsync: vi.fn(), isPending: false }),
  useUpdateRegistry: () => ({ mutateAsync: vi.fn(), isPending: false }),
}));

vi.mock('@/hooks/useRemote', () => ({
  useRemoteServer: () => ({ data: undefined }),
  useRemoteAdminRegistries: () => ({ data: undefined, isLoading: false }),
}));

describe('RegistryManagement', () => {
  it('shows a Managed badge for config-managed registries', () => {
    renderWithProviders(<RegistryManagement />);
    expect(screen.getByText('Managed')).toBeInTheDocument();
  });

  it('disables edit and delete for config-managed registries only', () => {
    renderWithProviders(<RegistryManagement />);

    // Rows render in data order: managed first, user-created second.
    const editButtons = screen.getAllByTitle(/edit registry/i);
    const deleteButtons = screen.getAllByTitle(/delete registry/i);
    expect(editButtons).toHaveLength(2);
    expect(deleteButtons).toHaveLength(2);

    expect(editButtons[0]).toBeDisabled();
    expect(deleteButtons[0]).toBeDisabled();
    expect(editButtons[1]).not.toBeDisabled();
    expect(deleteButtons[1]).not.toBeDisabled();
  });
});
