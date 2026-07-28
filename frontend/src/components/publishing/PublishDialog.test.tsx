import { screen, waitFor } from '@testing-library/react';
import userEvent from '@testing-library/user-event';
import { HttpResponse, http } from 'msw';
import { beforeEach, describe, expect, it, vi } from 'vitest';
import { mockPublishDefaults, mockRegistry, server } from '@/test/handlers';
import { renderWithProviders } from '@/test/utils';
import { PublishDialog } from './PublishDialog';

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
    server.use(http.get('/api/v1/registries', () => HttpResponse.json([])));
    renderWithProviders(<PublishDialog {...defaultProps} />);
    await waitFor(() =>
      expect(screen.getByText('No registries configured')).toBeInTheDocument(),
    );
  });

  it('renders the form with registry options when registries exist', async () => {
    const user = userEvent.setup();
    renderWithProviders(<PublishDialog {...defaultProps} />);
    const registrySelect = await screen.findByRole('combobox', {
      name: 'Registry',
    });
    await user.click(registrySelect);
    expect(
      await screen.findByRole('option', {
        name: new RegExp(mockRegistry.name),
      }),
    ).toBeInTheDocument();
    expect(screen.getByPlaceholderText(/e\.g\., myenv/)).toBeInTheDocument();
    expect(screen.getByPlaceholderText(/e\.g\., v1/)).toBeInTheDocument();
  });

  it('auto-populates form fields from publish defaults', async () => {
    renderWithProviders(<PublishDialog {...defaultProps} />);
    await waitFor(() =>
      expect(
        (screen.getByPlaceholderText(/e\.g\., myenv/) as HTMLInputElement)
          .value,
      ).toBe(mockPublishDefaults.repository),
    );
    expect(
      (screen.getByPlaceholderText(/e\.g\., v1/) as HTMLInputElement).value,
    ).toBe(mockPublishDefaults.tag);
  });

  it('disables the Publish button when required fields are empty', async () => {
    server.use(
      http.get('/api/v1/workspaces/:id/publish-defaults', () =>
        HttpResponse.json({
          registry_id: '',
          namespace: '',
          repository: '',
          tag: '',
          registry_name: '',
        }),
      ),
    );
    renderWithProviders(<PublishDialog {...defaultProps} />);
    await screen.findByRole('combobox', { name: 'Registry' });

    expect(screen.getByRole('button', { name: /Publish/ })).toBeDisabled();
  });

  it('shows success state after a successful publish', async () => {
    const user = userEvent.setup();
    renderWithProviders(<PublishDialog {...defaultProps} />);

    const registrySelect = await screen.findByRole('combobox', {
      name: 'Registry',
    });
    await user.click(registrySelect);
    await user.click(
      await screen.findByRole('option', {
        name: new RegExp(mockRegistry.name),
      }),
    );

    await user.click(screen.getByRole('button', { name: /Publish/ }));

    await waitFor(() =>
      expect(screen.getByText('Published successfully!')).toBeInTheDocument(),
    );
  });

  it('shows existing publication tags as hints', async () => {
    renderWithProviders(<PublishDialog {...defaultProps} />);
    await waitFor(() =>
      expect(screen.getByText(/v1\.0\.0/)).toBeInTheDocument(),
    );
  });

  it('does not render when open is false', () => {
    renderWithProviders(<PublishDialog {...defaultProps} open={false} />);
    expect(
      screen.queryByText('Publish Workspace to OCI Registry'),
    ).not.toBeInTheDocument();
  });
});
