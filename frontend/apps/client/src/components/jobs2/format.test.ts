import { describe, expect, it } from 'vitest';
import { mockJobSummary } from '@/test/fixtures/contracts';
import {
  formatDateTime,
  formatDuration,
  formatRelative,
  jobDuration,
  jobTitle,
  logFileName,
} from './format';
import { MAX_NOTIFICATIONS, toNotifications } from './notifications';

describe('job formatting', () => {
  it('formats durations like the prototype', () => {
    expect(formatDuration(0)).toBe('0s');
    expect(formatDuration(42_000)).toBe('42s');
    expect(formatDuration(125_000)).toBe('2m 05s');
    expect(formatDuration(3_780_000)).toBe('1h 03m');
    expect(formatDuration(-1)).toBe('0s');
  });

  it('formats relative start times', () => {
    const now = new Date(2026, 10, 9, 16, 0).getTime();
    expect(formatRelative(now - 10_000, now)).toBe('Just now');
    expect(formatRelative(now - 5 * 60_000, now)).toBe('5 min ago');
    expect(formatRelative(now - 60 * 60_000, now)).toBe('1 hour ago');
    expect(formatRelative(now - 3 * 60 * 60_000, now)).toBe('3 hours ago');
    expect(formatRelative(now - 20 * 60 * 60_000, now)).toBe('Yesterday');
    expect(formatRelative(new Date(2026, 10, 3, 9).getTime(), now)).toBe(
      '3 Nov',
    );
  });

  it('formats dates and durations of jobs', () => {
    const d = new Date(2026, 10, 9, 16, 5);
    expect(formatDateTime(d.toISOString())).toBe('9 Nov, 16:05');
    expect(jobDuration(mockJobSummary, Date.now())).toBe('30s');
    expect(
      jobDuration({ ...mockJobSummary, started_at: null }, Date.now()),
    ).toBeNull();
    const started = Date.parse('2026-09-29T00:00:01Z');
    expect(
      jobDuration({ ...mockJobSummary, completed_at: null }, started + 65_000),
    ).toBe('1m 05s');
  });

  it('titles jobs with their version', () => {
    expect(jobTitle(mockJobSummary)).toBe('Install version 7');
    expect(
      jobTitle({
        type: 'create',
        type_label: 'Create project',
        version_number: 1,
      }),
    ).toBe('Create project');
    expect(
      jobTitle({
        type: 'pull',
        type_label: 'Pull project',
        version_number: null,
      }),
    ).toBe('Pull project');
    expect(logFileName({ type: 'install_version', id: '12345678-aaaa' })).toBe(
      'nebi-install_version-12345678.log',
    );
  });

  it('turns finished jobs into notifications', () => {
    const base = Date.parse('2026-09-29T00:00:31Z');
    const jobs = [
      {
        ...mockJobSummary,
        id: 'a',
        status: 'running' as const,
        completed_at: null,
      },
      { ...mockJobSummary, id: 'b', status: 'failed' as const },
      {
        ...mockJobSummary,
        id: 'c',
        status: 'cancelled' as const,
        completed_at: new Date(base + 1000).toISOString(),
      },
    ];
    const n = toNotifications(jobs, base);
    expect(n.map((x) => x.id)).toEqual(['c', 'b']);
    expect(n[0]).toMatchObject({
      title: 'Install version 7 cancelled',
      unread: true,
    });
    expect(n[1]).toMatchObject({
      title: 'Install version 7 failed',
      unread: false,
    });
    const many = Array.from({ length: 20 }, (_, i) => ({
      ...mockJobSummary,
      id: String(i),
    }));
    expect(toNotifications(many, 0)).toHaveLength(MAX_NOTIFICATIONS);
  });
});
