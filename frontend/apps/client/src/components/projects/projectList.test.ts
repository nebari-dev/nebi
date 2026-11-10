import { describe, expect, it } from 'vitest';
import {
  mockFailedProject,
  mockProjects,
  mockPulledProject,
} from '@/test/handlers/projects';
import { installTarget, nextSort, searchAndSort } from './projectList';

const names = (xs: { name: string }[]) => xs.map((x) => x.name);

describe('projects list helpers', () => {
  it('installs the last installed version, else the latest', () => {
    expect(installTarget(mockPulledProject)).toBe(2);
    expect(installTarget(mockFailedProject)).toBe(1);
  });

  it('sorts by each column', () => {
    expect(
      names(searchAndSort(mockProjects, '', { key: 'name', dir: 'desc' })),
    ).toEqual(['sandbox', 'Project_pulled_from_server', 'Project_1']);
    expect(
      names(searchAndSort(mockProjects, '', { key: 'version', dir: 'asc' })),
    ).toEqual(['sandbox', 'Project_pulled_from_server', 'Project_1']);
    expect(
      names(
        searchAndSort(mockProjects, '', { key: 'environment', dir: 'asc' }),
      )[0],
    ).toBe('Project_1');
    expect(
      names(searchAndSort(mockProjects, '', { key: 'size', dir: 'desc' }))[0],
    ).toBe('Project_1');
    expect(
      names(searchAndSort(mockProjects, '', { key: 'remotes', dir: 'asc' }))[0],
    ).toBe('sandbox');
  });

  it('filters by name, ignoring case', () => {
    expect(
      names(searchAndSort(mockProjects, ' SAND ', { key: 'name', dir: 'asc' })),
    ).toEqual(['sandbox']);
  });

  it('flips direction on the active column', () => {
    expect(nextSort({ key: 'name', dir: 'asc' }, 'name')).toEqual({
      key: 'name',
      dir: 'desc',
    });
    expect(nextSort({ key: 'name', dir: 'desc' }, 'size')).toEqual({
      key: 'size',
      dir: 'asc',
    });
  });
});
