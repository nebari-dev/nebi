import userEvent from '@testing-library/user-event';
import { describe, expect, it } from 'vitest';
import { render, screen } from '@/test/utils';
import { PixiTomlEditor } from './PixiTomlEditor';

const customPixiToml = `# hand-written project note
[workspace]
name = "demo"
channels = ["conda-forge", "bioconda"]
platforms = ["linux-64"]

[dependencies]
python = ">=3.11"

[tasks]
hello = "echo hello"
`;

describe('PixiTomlEditor', () => {
  it('preserves hand-edited TOML content when UI mode adds a package', async () => {
    const user = userEvent.setup();
    let latestToml = customPixiToml;

    render(
      <PixiTomlEditor
        tomlValue={customPixiToml}
        onTomlChange={(value) => {
          latestToml = value;
        }}
        workspaceName="demo"
      />,
    );

    await user.click(screen.getByRole('button', { name: /ui mode/i }));
    await user.type(screen.getByPlaceholderText(/package name/i), 'numpy');
    await user.click(screen.getByRole('button', { name: /add package/i }));

    expect(latestToml).toContain('# hand-written project note');
    expect(latestToml).toContain('channels = ["conda-forge", "bioconda"]');
    expect(latestToml).toContain('platforms = ["linux-64"]');
    expect(latestToml).toContain('[tasks]');
    expect(latestToml).toContain('hello = "echo hello"');
    expect(latestToml).toContain('numpy = "*"');
  });
});
