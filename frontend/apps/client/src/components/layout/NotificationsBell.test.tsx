import { screen, waitFor } from '@testing-library/react';
import userEvent from '@testing-library/user-event';
import { HttpResponse, http } from 'msw';
import { beforeEach, describe, expect, it } from 'vitest';
import { renderJobsRoute } from '@/pages/jobs/testUtils';
import { useModeStore } from '@/store/modeStore';
import { server } from '@/test/handlers';
import { FAILED_JOB_ID } from '@/test/handlers/jobs';
import { NotificationsBell } from './NotificationsBell';

const SEEN_KEY = 'nebi.notifications.seenAt';

beforeEach(() => {
  localStorage.clear();
  useModeStore.setState({ mode: 'local', loading: false });
});

describe('NotificationsBell', () => {
  it('lists recent local job outcomes with an unread count', async () => {
    const user = userEvent.setup();
    // Last opened an hour ago: the failed job (30 min ago) is unread, the
    // install that finished 3 hours ago is not.
    localStorage.setItem(SEEN_KEY, String(Date.now() - 60 * 60_000));
    renderJobsRoute('/jobs', <NotificationsBell />);

    const bell = await screen.findByRole('button', {
      name: 'Notifications, 1 unread',
    });
    await user.click(bell);
    const failed = await screen.findByRole('menuitem', {
      name: /Install version 2 failed/,
    });
    expect(failed).toHaveTextContent('sandbox');
    expect(failed).toHaveTextContent('min ago');
    expect(
      screen.getByRole('menuitem', { name: /Install version 7 succeeded/ }),
    ).toBeInTheDocument();
    // Active jobs are not notifications.
    expect(
      screen.queryByRole('menuitem', { name: /Create version/ }),
    ).not.toBeInTheDocument();
    expect(screen.getAllByRole('img', { name: 'Unread' })).toHaveLength(1);

    await user.click(failed);
    await waitFor(() =>
      expect(screen.getByTestId('location')).toHaveTextContent(
        `/jobs/${FAILED_JOB_ID}`,
      ),
    );
    // Closing the menu marks everything read.
    expect(
      await screen.findByRole('button', { name: 'Notifications' }),
    ).toBeInTheDocument();
    expect(Number(localStorage.getItem(SEEN_KEY))).toBeGreaterThan(
      Date.now() - 60_000,
    );
  });

  it('starts with nothing unread on first use and links to all jobs', async () => {
    const user = userEvent.setup();
    renderJobsRoute(`/jobs/${FAILED_JOB_ID}`, <NotificationsBell />);
    const bell = await screen.findByRole('button', { name: 'Notifications' });
    await user.click(bell);
    await user.click(
      await screen.findByRole('menuitem', { name: 'View all jobs' }),
    );
    await waitFor(() =>
      expect(screen.getByTestId('location')).toHaveTextContent(/^\/jobs$/),
    );
  });

  it('shows an empty state', async () => {
    const user = userEvent.setup();
    server.use(http.get('/api/v1/client/jobs', () => HttpResponse.json([])));
    renderJobsRoute('/jobs', <NotificationsBell />);
    await user.click(
      await screen.findByRole('button', { name: 'Notifications' }),
    );
    expect(await screen.findByText('No notifications yet')).toBeInTheDocument();
    expect(
      screen.getByText('Finished jobs on this machine show up here.'),
    ).toBeInTheDocument();
  });
});
