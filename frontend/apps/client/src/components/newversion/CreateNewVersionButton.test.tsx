import { TooltipProvider } from '@nebi/ui/components/ui/tooltip';
import userEvent from '@testing-library/user-event';
import { Route, Routes, useLocation } from 'react-router-dom';
import { describe, expect, it } from 'vitest';
import { mockVersionItem, PROJECT_ID } from '@/test/fixtures/contracts';
import { renderWithProviders, screen, waitFor, within } from '@/test/utils';
import type { VersionItem } from '@/types/contracts';
import { CreateNewVersionButton } from './CreateNewVersionButton';

const v = (number: number, extra: Partial<VersionItem> = {}): VersionItem => ({
  ...mockVersionItem,
  number,
  ...extra,
});

const versions = [v(8, { server_only: true }), v(7), v(6), v(5)];

export function Where() {
  const loc = useLocation();
  return <p data-testid="where">{`${loc.pathname}${loc.search}`}</p>;
}

function renderButton(version: VersionItem) {
  return renderWithProviders(
    <TooltipProvider>
      <Routes>
        <Route
          path="*"
          element={
            <CreateNewVersionButton
              projectId={PROJECT_ID}
              version={version}
              versions={versions}
            />
          }
        />
      </Routes>
      <Where />
    </TooltipProvider>,
    { initialEntries: [`/projects/${PROJECT_ID}`] },
  );
}

const where = () => screen.getByTestId('where').textContent;

describe('CreateNewVersionButton', () => {
  it('goes straight to the page from the latest local version', async () => {
    const user = userEvent.setup();
    renderButton(v(7));
    await user.click(
      screen.getByRole('button', { name: 'Create new version' }),
    );
    expect(screen.queryByRole('dialog')).not.toBeInTheDocument();
    expect(where()).toBe(`/projects/${PROJECT_ID}/new-version?from=7`);
  });

  it('asks before starting from an older version', async () => {
    const user = userEvent.setup();
    renderButton(v(5));
    await user.click(
      screen.getByRole('button', { name: 'Create new version' }),
    );
    const dialog = await screen.findByRole('dialog');
    expect(
      within(dialog).getByText('Start a new version from version 5?'),
    ).toBeInTheDocument();
    expect(
      within(dialog).getByText(
        'Version 5 is older than the latest version, v7.',
      ),
    ).toBeInTheDocument();
    expect(
      within(dialog).getByText(
        'The new version starts with version 5’s packages and settings. Changes made in versions 6 and 7 won’t carry over, but those versions stay in the history.',
      ),
    ).toBeInTheDocument();

    await user.click(within(dialog).getByRole('button', { name: 'Cancel' }));
    await waitFor(() =>
      expect(screen.queryByRole('dialog')).not.toBeInTheDocument(),
    );
    expect(where()).toBe(`/projects/${PROJECT_ID}`);

    await user.click(
      screen.getByRole('button', { name: 'Create new version' }),
    );
    await user.click(
      within(await screen.findByRole('dialog')).getByRole('button', {
        name: 'Start from v7',
      }),
    );
    expect(where()).toBe(`/projects/${PROJECT_ID}/new-version?from=7`);
  });

  it('starts from the older version when confirmed', async () => {
    const user = userEvent.setup();
    renderButton(v(6));
    await user.click(
      screen.getByRole('button', { name: 'Create new version' }),
    );
    const dialog = await screen.findByRole('dialog');
    expect(
      within(dialog).getByText(/Changes made in version 7 won’t carry over/),
    ).toBeInTheDocument();
    await user.click(
      within(dialog).getByRole('button', { name: 'Start from version 6' }),
    );
    expect(where()).toBe(`/projects/${PROJECT_ID}/new-version?from=6`);
  });

  it('is disabled for a version only on the server', () => {
    renderButton(v(8, { server_only: true }));
    expect(
      screen.getByRole('button', { name: 'Create new version' }),
    ).toBeDisabled();
  });
});
