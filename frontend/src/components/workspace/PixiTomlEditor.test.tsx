import { fireEvent, screen } from '@testing-library/react';
import userEvent from '@testing-library/user-event';
import { useState } from 'react';
import { describe, expect, it } from 'vitest';
import { renderWithProviders } from '@/test/utils';
import { PixiTomlEditor } from './PixiTomlEditor';

const DEFAULT_PIXI_TOML = `[workspace]
name = ""
channels = ["conda-forge"]
platforms = ["osx-arm64", "linux-64"]

[dependencies]
python = ">=3.11"
`;

const getTomlName = (toml: string): string | null => {
  const match = toml.match(/^name\s*=\s*"([^"]*)"/m);
  return match ? match[1] : null;
};

function renderEditor() {
  const EditorHarness = () => {
    const [toml, setToml] = useState(DEFAULT_PIXI_TOML);

    return (
      <PixiTomlEditor
        tomlValue={toml}
        onTomlChange={setToml}
        workspaceName={getTomlName(toml) || ''}
      />
    );
  };

  renderWithProviders(<EditorHarness />);
}

describe('PixiTomlEditor', () => {
  it('discards TOML changes immediately when switching to UI mode', async () => {
    const user = userEvent.setup();
    renderEditor();

    fireEvent.change(
      screen.getByPlaceholderText('Enter your pixi.toml content'),
      {
        target: {
          value: DEFAULT_PIXI_TOML.replace('name = ""', 'name = "edited"'),
        },
      },
    );

    await user.click(screen.getByRole('button', { name: /ui mode/i }));
    await user.click(screen.getByRole('button', { name: /discard changes/i }));

    expect(screen.getByPlaceholderText('Workspace name')).toHaveValue('');

    await user.click(screen.getByRole('button', { name: /toml mode/i }));

    expect(
      screen.getByPlaceholderText('Enter your pixi.toml content'),
    ).toHaveValue(DEFAULT_PIXI_TOML);
  });
});
