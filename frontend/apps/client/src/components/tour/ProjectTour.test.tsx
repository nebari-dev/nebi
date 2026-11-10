import { TooltipProvider } from '@nebi/ui/components/ui/tooltip';
import { render, screen, waitFor } from '@testing-library/react';
import userEvent from '@testing-library/user-event';
import { MemoryRouter } from 'react-router-dom';
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';
import { HelpMenu } from '@/components/layout/HelpMenu';
import { ProjectTour } from './ProjectTour';
import { TOUR_SEEN_KEY, useTourStore } from './tourStore';

// biome-ignore lint/style/useComponentExportOnlyModules: test-only helper
const Page = ({ withPush = true }: { withPush?: boolean }) => (
  <TooltipProvider>
    <MemoryRouter initialEntries={['/projects/p1']}>
      <HelpMenu />
      <aside data-tour="rail">rail</aside>
      <span data-tour="installed">Installed</span>
      <button type="button" data-tour="create-version">
        Create new version
      </button>
      {withPush && (
        <button type="button" data-tour="push">
          Push to server
        </button>
      )}
      <button type="button" data-tour="share">
        Share
      </button>
      <button type="button" data-tour="publish">
        Publish
      </button>
      <ProjectTour />
    </MemoryRouter>
  </TooltipProvider>
);

describe('ProjectTour', () => {
  beforeEach(() => {
    window.localStorage.clear();
    useTourStore.getState().reload();
  });
  afterEach(() => {
    vi.restoreAllMocks();
  });

  it('prompts new viewers and remembers Skip', async () => {
    const user = userEvent.setup();
    const { unmount } = render(<Page />);
    const prompt = await screen.findByRole('dialog', {
      name: 'New to Nebi projects?',
    });
    expect(prompt).toHaveTextContent(
      'Take a 1-minute tour of this page: versions, installing, sharing and publishing.',
    );
    expect(prompt).toHaveTextContent(
      'Not now? You can start it any time from Help.',
    );
    await user.click(screen.getByRole('button', { name: 'Skip' }));
    await waitFor(() =>
      expect(
        screen.queryByRole('dialog', { name: 'New to Nebi projects?' }),
      ).toBeNull(),
    );
    expect(window.localStorage.getItem(TOUR_SEEN_KEY)).toBe('1');
    unmount();

    useTourStore.getState().reload();
    render(<Page />);
    expect(
      screen.queryByRole('dialog', { name: 'New to Nebi projects?' }),
    ).toBeNull();
  });

  it('walks through every step with Next, Back and End tour', async () => {
    const user = userEvent.setup();
    render(<Page />);
    await user.click(await screen.findByRole('button', { name: 'Start tour' }));

    const titles = [
      'Every change is a new version',
      'One version is installed at a time',
      'Change packages with a new version',
      'Push sends your versions to the server',
      'Share gives teammates access',
      'Publish sends a version to a registry',
    ];
    for (const [i, title] of titles.entries()) {
      const card = await screen.findByRole('dialog', { name: title });
      expect(card).toHaveTextContent(`${i + 1} of ${titles.length}`);
      if (i === 1) {
        await user.click(screen.getByRole('button', { name: 'Back' }));
        await screen.findByRole('dialog', { name: titles[0] });
        await user.click(screen.getByRole('button', { name: 'Next' }));
        await screen.findByRole('dialog', { name: title });
      }
      await user.click(screen.getByRole('button', { name: 'Next' }));
    }
    const finish = await screen.findByRole('dialog', {
      name: 'Come back anytime',
    });
    expect(finish).toHaveTextContent(
      'Restart this tour from Help whenever you need it.',
    );
    await user.click(screen.getByRole('button', { name: 'Done' }));
    await waitFor(() =>
      expect(
        screen.queryByRole('dialog', { name: 'Come back anytime' }),
      ).toBeNull(),
    );
    expect(useTourStore.getState().active).toBe(false);
  });

  it('skips steps whose targets are missing and ends with End tour', async () => {
    const user = userEvent.setup();
    window.localStorage.setItem(TOUR_SEEN_KEY, '1');
    useTourStore.getState().reload();
    render(<Page withPush={false} />);

    await user.click(screen.getByRole('button', { name: /Help/ }));
    await user.click(
      await screen.findByRole('menuitem', { name: 'Take the page tour' }),
    );
    const first = await screen.findByRole('dialog', {
      name: 'Every change is a new version',
    });
    expect(first).toHaveTextContent('1 of 5');
    await user.click(screen.getByRole('button', { name: 'End tour' }));
    await waitFor(() =>
      expect(
        screen.queryByRole('dialog', { name: 'Every change is a new version' }),
      ).toBeNull(),
    );
  });

  it('works when localStorage is blocked', () => {
    vi.spyOn(window.localStorage, 'getItem').mockImplementation(() => {
      throw new Error('blocked');
    });
    vi.spyOn(window.localStorage, 'setItem').mockImplementation(() => {
      throw new Error('blocked');
    });
    useTourStore.getState().reload();
    expect(useTourStore.getState().seen).toBe(false);
    useTourStore.getState().dismissPrompt();
    expect(useTourStore.getState().seen).toBe(true);
  });
});

describe('HelpMenu', () => {
  it('offers the tour only on a project page', async () => {
    const user = userEvent.setup();
    render(
      <TooltipProvider>
        <MemoryRouter initialEntries={['/jobs']}>
          <HelpMenu />
        </MemoryRouter>
      </TooltipProvider>,
    );
    await user.click(screen.getByRole('button', { name: /Help/ }));
    const item = await screen.findByRole('menuitem', {
      name: 'No tour for this page yet',
    });
    expect(item).toHaveAttribute('aria-disabled', 'true');
    expect(screen.getByRole('menuitem', { name: /Docs/ })).toBeInTheDocument();
  });
});
