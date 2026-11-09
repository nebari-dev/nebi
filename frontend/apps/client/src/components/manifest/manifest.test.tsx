import { TooltipProvider } from '@nebi/ui/components/ui/tooltip';
import { screen, waitFor } from '@testing-library/react';
import userEvent from '@testing-library/user-event';
import { useState } from 'react';
import { describe, expect, it } from 'vitest';
import { renderWithProviders } from '@/test/utils';
import { ManifestEditor } from './ManifestEditor';
import {
  emptyManifestValue,
  formToToml,
  type ManifestEditorValue,
  setTomlName,
} from './manifestValue';
import {
  joinConstraint,
  machinePlatform,
  platformDisplayName,
  projectNameError,
  splitConstraint,
  tagError,
} from './rules';

describe('constraints', () => {
  it.each([
    ['*', ['none', '']],
    ['', ['none', '']],
    ['>=3.11', ['>=', '3.11']],
    ['>1', ['>', '1']],
    ['<=2', ['<=', '2']],
    ['~=1.4', ['~=', '1.4']],
    ['3.12.*', ['none', '3.12.*']],
  ])('splits %s', (c, want) => {
    expect(splitConstraint(c)).toEqual(want);
  });

  it.each([
    ['>=', '', '*'],
    ['>=', ' 2.0 ', '>=2.0'],
    ['none', '3.12.*', '3.12.*'],
    ['==', '>1', '>1'],
    ['!=', '1.0', '!=1.0'],
  ])('joins %s %s', (op, v, want) => {
    expect(joinConstraint(op, v)).toBe(want);
  });
});

describe('rules', () => {
  it('validates project names', () => {
    expect(projectNameError('')).toBe('Enter a project name.');
    expect(projectNameError('a b')).toBe(
      'Use letters, numbers, hyphens, underscores or dots.',
    );
    expect(projectNameError('My_data-1.0', ['other'])).toBeNull();
    expect(projectNameError('PROJECT_1', ['Project_1'])).toBe(
      'A project with that name already exists.',
    );
  });

  it('validates tags with the prototype copy', () => {
    expect(tagError('v1.0')).toBeNull();
    expect(tagError('1.0')).toBe(
      '“1.0” can’t be used: tags must start with a letter.',
    );
    expect(tagError('v 1')).toBe(
      '“v 1” can’t be used: tags use letters, numbers, dots, hyphens and underscores.',
    );
  });

  it('maps host platforms', () => {
    expect(machinePlatform('darwin/arm64')).toBe('osx-arm64');
    expect(machinePlatform('linux/amd64')).toBe('linux-64');
    expect(machinePlatform('windows/amd64')).toBe('win-64');
    expect(platformDisplayName('osx-arm64', 'osx-arm64')).toBe(
      'osx-arm64 · this machine',
    );
    expect(platformDisplayName('linux-64', 'osx-arm64')).toBe(
      'linux-64 · Linux',
    );
    expect(platformDisplayName('linux-ppc64le')).toBe('linux-ppc64le');
  });
});

describe('manifest value', () => {
  it('renders a fresh pixi.toml', () => {
    expect(
      formToToml({
        name: 'demo',
        channels: ['conda-forge'],
        platforms: ['linux-64'],
        requested: [
          { name: 'python', constraint: '>=3.11' },
          { name: 'ruamel.yaml', constraint: '' },
          { name: 'rich', constraint: '>=13', source: 'pypi' },
        ],
      }),
    ).toBe(
      '[workspace]\nname = "demo"\nchannels = ["conda-forge"]\nplatforms = ["linux-64"]\n\n[dependencies]\npython = ">=3.11"\n"ruamel.yaml" = "*"\n\n[pypi-dependencies]\nrich = ">=13"\n',
    );
  });

  it('renames only the workspace name', () => {
    const toml =
      '# name = "comment"\n[workspace]\nname = "old"\n\n[feature.x]\nname = "keep"\n';
    expect(setTomlName(toml, 'new')).toBe(
      '# name = "comment"\n[workspace]\nname = "new"\n\n[feature.x]\nname = "keep"\n',
    );
  });
});

// biome-ignore lint/style/useComponentExportOnlyModules: test-only harness
const Harness = ({ initial }: { initial: ManifestEditorValue }) => {
  const [value, setValue] = useState(initial);
  return (
    <TooltipProvider>
      <ManifestEditor
        value={value}
        onChange={setValue}
        showName
        machine="linux-64"
        copy={{ packagesEmptyText: 'No packages. Empty environment.' }}
      />
      <output data-testid="value">{JSON.stringify(value)}</output>
    </TooltipProvider>
  );
};

describe('ManifestEditor', () => {
  it('works on its own with the contract props (New version usage)', async () => {
    renderWithProviders(<Harness initial={emptyManifestValue('base')} />);
    expect(screen.getByDisplayValue('base')).toBeInTheDocument();
    expect(
      screen.getByText('No packages. Empty environment.'),
    ).toBeInTheDocument();

    await userEvent.type(screen.getByPlaceholderText('Package name'), 'scipy');
    await userEvent.keyboard('{Enter}');
    expect(screen.getByTestId('value').textContent).toContain(
      '{"name":"scipy","constraint":"*"}',
    );

    await userEvent.click(screen.getByRole('button', { name: 'Remove scipy' }));
    expect(screen.getByTestId('value').textContent).not.toContain('scipy');

    await userEvent.type(
      screen.getByPlaceholderText('Package name'),
      'bad name',
    );
    await userEvent.click(screen.getByRole('button', { name: /Add package/ }));
    expect(
      screen.getByText(
        'Package names use letters, numbers, hyphens, underscores or dots.',
      ),
    ).toBeInTheDocument();

    // The built-in mode switch converts through the client API.
    await userEvent.click(
      screen.getByRole('button', { name: /Open in TOML editor/ }),
    );
    await waitFor(() =>
      expect(screen.getByTestId('value').textContent).toContain(
        '"mode":"toml"',
      ),
    );
    expect(
      screen.getByRole('textbox', { name: 'pixi.toml' }),
    ).toBeInTheDocument();
  });

  it('edits a package constraint in place', async () => {
    renderWithProviders(
      <Harness
        initial={{
          ...emptyManifestValue('p'),
          form: {
            name: 'p',
            channels: ['conda-forge'],
            platforms: ['linux-64'],
            requested: [{ name: 'numpy', constraint: '>=1.0' }],
          },
        }}
      />,
    );
    const version = screen.getByLabelText('Version for numpy');
    await userEvent.clear(version);
    await userEvent.type(version, '2.1{Enter}');
    expect(screen.getByTestId('value').textContent).toContain(
      '{"name":"numpy","constraint":">=2.1"}',
    );
    await userEvent.clear(version);
    await userEvent.tab();
    expect(screen.getByTestId('value').textContent).toContain(
      '{"name":"numpy","constraint":"*"}',
    );
  });
});
