import { describe, it, expect, vi, beforeEach } from 'vitest';
import { screen, waitFor } from '@testing-library/react';
import userEvent from '@testing-library/user-event';
import { http, HttpResponse } from 'msw';
import { server, mockRegistry, mockPublishDefaults } from '@/test/handlers';
import { PublishDialog } from './PublishDialog';
import { renderWithProviders } from '@/test/utils';

const defaultProps = {
  open: true,
  onOpenChange: vi.fn(),
  environmentId: 'ws-1',
  environmentName: 'test-workspace',
};

beforeEach(() => {
  vi.clearAllMocks();
  // Suppress jsdom "Not implemented" noise from window.location.reload
  vi.spyOn(window, 'location', 'get').mockReturnValue({
    ...window.location,
    reload: vi.fn(),
  } as Location);
});

describe('PublishDialog', () => {
  it('shows a loading spinner while fetching data', () => {
    // Spinner is visible on initial render before the query resolves
    renderWithProviders(<PublishDialog {...defaultProps} />);
    expect(document.querySelector('.animate-spin')).toBeTruthy();
  });

  it('shows a warning when no registries are configured', async () => {
    server.use(
      http.get('/api/v1/registries', () => HttpResponse.json([]))
    );
    renderWithProviders(<PublishDialog {...defaultProps} />);
    await waitFor(() =>
      expect(screen.getByText('No registries configured')).toBeInTheDocument()
    );
  });

  it('renders the form with registry options when registries exist', async () => {
    renderWithProviders(<PublishDialog {...defaultProps} />);
    await waitFor(() =>
      expect(screen.getByRole('option', { name: new RegExp(mockRegistry.name) })).toBeInTheDocument()
    );
    expect(screen.getByPlaceholderText(/e\.g\., myenv/)).toBeInTheDocument();
    expect(screen.getByPlaceholderText(/e\.g\., v1/)).toBeInTheDocument();
  });

  it('auto-populates form fields from publish defaults', async () => {
    renderWithProviders(<PublishDialog {...defaultProps} />);
    await waitFor(() =>
      expect((screen.getByPlaceholderText(/e\.g\., myenv/) as HTMLInputElement).value).toBe(
        mockPublishDefaults.repository
      )
    );
    expect((screen.getByPlaceholderText(/e\.g\., v1/) as HTMLInputElement).value).toBe(
      mockPublishDefaults.tag
    );
  });

  it('disables the Publish button when required fields are empty', async () => {
    server.use(
      http.get('/api/v1/workspaces/:id/publish-defaults', () =>
        HttpResponse.json({ registry_id: '', namespace: '', repository: '', tag: '', registry_name: '' })
      )
    );
    renderWithProviders(<PublishDialog {...defaultProps} />);
    await waitFor(() =>
      expect(screen.getByRole('option', { name: new RegExp(mockRegistry.name) })).toBeInTheDocument()
    );

    expect(screen.getByRole('button', { name: /Publish/ })).toBeDisabled();
  });

  it('shows success state after a successful publish', async () => {
    const user = userEvent.setup();
    renderWithProviders(<PublishDialog {...defaultProps} />);

    await waitFor(() =>
      expect(screen.getByRole('option', { name: new RegExp(mockRegistry.name) })).toBeInTheDocument()
    );

    // Select registry
    const select = screen.getByRole('combobox', { hidden: true }) as HTMLSelectElement;
    await user.selectOptions(select, mockRegistry.id);

    await user.click(screen.getByRole('button', { name: /Publish/ }));

    await waitFor(() =>
      expect(screen.getByText('Published successfully!')).toBeInTheDocument()
    );
  });

  it('shows existing publication tags as hints', async () => {
    renderWithProviders(<PublishDialog {...defaultProps} />);
    await waitFor(() =>
      expect(screen.getByText(/v1\.0\.0/)).toBeInTheDocument()
    );
  });

  it('does not render when open is false', () => {
    renderWithProviders(<PublishDialog {...defaultProps} open={false} />);
    expect(screen.queryByText('Publish Workspace to OCI Registry')).not.toBeInTheDocument();
  });
});
