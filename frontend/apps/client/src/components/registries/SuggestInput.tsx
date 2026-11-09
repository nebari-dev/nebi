// Owner: U7 (publish). Free-text input with a suggestion list, built on the
// design-system Combobox. The value is the typed text (controlled through
// inputValue), so a repository or tag the registry does not list, or a
// registry without a catalog API, can still be entered.
import {
  Combobox,
  ComboboxContent,
  ComboboxEmpty,
  ComboboxInput,
  ComboboxItem,
  ComboboxList,
} from '@nebi/ui/components/ui/combobox';
import type { ComponentProps } from 'react';

export interface SuggestInputProps
  extends Omit<ComponentProps<typeof ComboboxInput>, 'value' | 'onChange'> {
  value: string;
  onValueChange: (value: string) => void;
  suggestions: string[];
  emptyText?: string;
}

export const SuggestInput = ({
  value,
  onValueChange,
  suggestions,
  emptyText = 'No match',
  ...inputProps
}: SuggestInputProps) => (
  <Combobox
    items={suggestions}
    inputValue={value}
    onInputValueChange={(v, details) => {
      // Closing the popup re-syncs a single-select combobox's input to its
      // (empty) selection; keep the typed text instead.
      if (details.reason === 'input-clear' || details.reason === 'none') {
        return;
      }
      onValueChange(v);
    }}
    onValueChange={(v) => {
      if (typeof v === 'string') onValueChange(v);
    }}
    openOnInputClick
  >
    <ComboboxInput spellCheck={false} {...inputProps} />
    <ComboboxContent align="start">
      <ComboboxEmpty>{emptyText}</ComboboxEmpty>
      <ComboboxList>
        {(item: string) => (
          <ComboboxItem key={item} value={item}>
            {item}
          </ComboboxItem>
        )}
      </ComboboxList>
    </ComboboxContent>
  </Combobox>
);
