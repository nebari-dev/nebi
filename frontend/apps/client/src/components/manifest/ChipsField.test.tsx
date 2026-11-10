import { screen } from '@testing-library/react';
import userEvent from '@testing-library/user-event';
import { useState } from 'react';
import { describe, expect, it } from 'vitest';
import { renderWithProviders } from '@/test/utils';
import { ChipsField } from './ChipsField';

// biome-ignore lint/style/useComponentExportOnlyModules: test-only harness
const Harness = ({ keepPlaceholder }: { keepPlaceholder?: boolean }) => {
  const [value, setValue] = useState(['latest']);
  return (
    <>
      <ChipsField
        label="Tags (optional)"
        value={value}
        onChange={setValue}
        suggestions={['latest']}
        placeholder="Add a tag"
        keepPlaceholder={keepPlaceholder}
      />
      <output data-testid="value">{value.join(',')}</output>
    </>
  );
};

describe('ChipsField', () => {
  it('adds a typed value and keeps the chips when Escape is pressed', async () => {
    renderWithProviders(<Harness keepPlaceholder />);
    const input = screen.getByRole('combobox');
    await userEvent.click(input);
    await userEvent.type(input, 'stable');
    await userEvent.keyboard('{Enter}');
    expect(screen.getByTestId('value').textContent).toBe('latest,stable');

    // Escape closes the popup, then would clear the whole selection in
    // Base UI; the chips must survive both presses.
    await userEvent.keyboard('{Escape}');
    await userEvent.keyboard('{Escape}');
    expect(screen.getByTestId('value').textContent).toBe('latest,stable');
  });

  it('keeps the placeholder next to chips only when asked', () => {
    const { unmount } = renderWithProviders(<Harness keepPlaceholder />);
    expect(screen.getByPlaceholderText('Add a tag')).toBeInTheDocument();
    unmount();
    renderWithProviders(<Harness />);
    expect(screen.queryByPlaceholderText('Add a tag')).not.toBeInTheDocument();
  });
});
