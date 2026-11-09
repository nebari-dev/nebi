import { describe, expect, it } from 'vitest';
import { archiveFileName } from '@/api/client/detail';
import { mockProjectItem } from '@/test/fixtures/contracts';
import {
  mockDetailVersions,
  mockServerOnlyVersion,
} from '@/test/handlers/detail';
import {
  ageGroupLabel,
  authorLabel,
  formatBytes,
  formatVersionDate,
  groupVersionsByAge,
  pickVersion,
  pixiPlatformFor,
  platformLabel,
  unpushedVersions,
  versionsPhrase,
} from './format';
import { availableFilters, filterVersions } from './versionFilters';

describe('detail format helpers', () => {
  it('formats dates like the prototype', () => {
    const now = Date.parse('2026-11-09T12:00:00Z');
    expect(formatVersionDate('2026-11-09T11:59:40Z', now)).toBe('just now');
    expect(formatVersionDate('2026-11-09T11:00:00Z', now)).toBe('9 Nov 2026');
  });

  it('labels age groups', () => {
    expect([0, 3, 12, 200, 400].map(ageGroupLabel)).toEqual([
      'Today',
      'This week',
      'This month',
      'This year',
      'Older',
    ]);
    const groups = groupVersionsByAge(mockDetailVersions);
    expect(groups.map((g) => g.label)).toEqual([
      'Today',
      'This week',
      'This month',
      'This year',
    ]);
  });

  it('words version lists', () => {
    expect(versionsPhrase([5])).toBe('version 5');
    expect(versionsPhrase([5, 6, 7])).toBe('versions 5, 6 and 7');
  });

  it('formats download sizes', () => {
    expect(formatBytes(0)).toBe('');
    expect(formatBytes(512)).toBe('512 B');
    expect(formatBytes(17_500_000)).toBe('17.5 MB');
    expect(formatBytes(1_200_000_000)).toBe('1.2 GB');
  });

  it('maps the host platform', () => {
    expect(pixiPlatformFor('darwin/arm64')).toBe('osx-arm64');
    expect(pixiPlatformFor('linux/amd64')).toBe('linux-64');
    expect(pixiPlatformFor('windows/amd64')).toBe('win-64');
    expect(pixiPlatformFor('plan9/mips')).toBe('');
    expect(platformLabel('osx-arm64', 'osx-arm64')).toBe(
      'osx-arm64 · this machine',
    );
    expect(platformLabel('linux-64', 'osx-arm64')).toBe('linux-64');
  });

  it('marks versions made on this machine', () => {
    expect(authorLabel(mockDetailVersions[0])).toBe('Sam Garcia (you)');
    expect(authorLabel(mockDetailVersions[1])).toBe('Alex Kim');
  });

  it('picks the version to show', () => {
    const all = [mockServerOnlyVersion, ...mockDetailVersions];
    expect(pickVersion(all, 5, 7)?.number).toBe(5);
    expect(pickVersion(all, 0, 6)?.number).toBe(6);
    expect(pickVersion(all, 0, null)?.number).toBe(7);
    expect(pickVersion([], 0, null)).toBeUndefined();
  });

  it('lists local versions the server lacks', () => {
    expect(
      unpushedVersions({ ...mockProjectItem, server_latest_version: 5 }, [
        mockServerOnlyVersion,
        ...mockDetailVersions,
      ]),
    ).toEqual([6, 7]);
  });

  it('reads archive file names', () => {
    expect(archiveFileName('attachment; filename=p-v2.tar.gz')).toBe(
      'p-v2.tar.gz',
    );
    expect(archiveFileName('attachment; filename="my p-v2.tar.gz"')).toBe(
      'my p-v2.tar.gz',
    );
    expect(archiveFileName('attachment')).toBeNull();
  });
});

describe('version filters', () => {
  const ctx = { linked: true, serverLatest: 6 };
  const all = [mockServerOnlyVersion, ...mockDetailVersions];

  it('searches by number, description, author and tag', () => {
    expect(filterVersions(all, 'v5', [], ctx).map((v) => v.number)).toEqual([
      5,
    ]);
    expect(filterVersions(all, 'alex', [], ctx).map((v) => v.number)).toEqual([
      8, 6,
    ]);
    expect(filterVersions(all, 'LATEST', [], ctx).map((v) => v.number)).toEqual(
      [7],
    );
  });

  it('combines filters', () => {
    const n = (f: Parameters<typeof filterVersions>[2]) =>
      filterVersions(all, '', f, ctx).map((v) => v.number);
    expect(n(['installed'])).toEqual([7]);
    expect(n(['tagged'])).toEqual([7, 5]);
    expect(n(['published'])).toEqual([5]);
    expect(n(['not_pushed'])).toEqual([7]);
    expect(n(['server_only'])).toEqual([8]);
    expect(n(['tagged', 'published'])).toEqual([5]);
  });

  it('offers server filters only when they apply', () => {
    expect(
      availableFilters(mockDetailVersions, {
        linked: false,
        serverLatest: null,
      }),
    ).toEqual(['installed', 'tagged', 'published']);
    expect(availableFilters(all, ctx)).toEqual([
      'installed',
      'tagged',
      'published',
      'not_pushed',
      'server_only',
    ]);
  });
});
