// Owner: U1 (projects). A labelled multi-value combobox with chips
// (prototype: Platforms, Tags; also Channels). Typing a value that is not in
// the suggestions offers "Add “value”"; Tab also adds the typed value.
import {
  Combobox,
  ComboboxChip,
  ComboboxChips,
  ComboboxContent,
  ComboboxEmpty,
  ComboboxInput,
  ComboboxItem,
  ComboboxList,
} from '@nebi/ui/components/ui/combobox';
import {
  Field,
  FieldDescription,
  FieldError,
  FieldLabel,
} from '@nebi/ui/components/ui/field';
import { cn } from '@nebi/ui/lib/utils';
import { type ReactNode, useState } from 'react';

export interface ChipsFieldProps {
  label: ReactNode;
  value: string[];
  onChange: (value: string[]) => void;
  /** Offered before anything is typed (selected values are always listed). */
  suggestions?: string[];
  placeholder?: string;
  description?: ReactNode;
  /** Field-level error (replaces the description). */
  error?: string | null;
  /** Per-chip problem: the chip turns red and the message is listed. */
  invalid?: (item: string) => string | null;
  /** Display text of an item (chips and options). */
  itemLabel?: (item: string) => string;
  /** Normalizes typed text before it is offered (default: trim). */
  normalize?: (input: string) => string;
  /** Whether typed text may be added as a new value (default: any). */
  acceptCustom?: (input: string) => boolean;
  /** Shown in the popup when nothing matches. */
  emptyText?: ReactNode;
  disabled?: boolean;
}

export const ChipsField = ({
  label,
  value,
  onChange,
  suggestions = [],
  placeholder,
  description,
  error,
  invalid,
  itemLabel = (item) => item,
  normalize = (input) => input.trim(),
  acceptCustom = () => true,
  emptyText = 'Type a name and press Enter.',
  disabled,
}: ChipsFieldProps) => {
  const [input, setInput] = useState('');
  const typed = normalize(input);
  const query = typed.toLowerCase();
  const known = [...new Set([...suggestions, ...value])];
  const custom =
    typed && !known.includes(typed) && acceptCustom(typed) ? [typed] : [];
  const items = [
    ...known.filter(
      (item) => !query || itemLabel(item).toLowerCase().includes(query),
    ),
    ...custom,
  ];
  const problems = value
    .map((item) => invalid?.(item))
    .filter((m): m is string => !!m);
  const hasError = !!error || problems.length > 0;

  return (
    <Field invalid={hasError} className="w-full min-w-0 gap-1">
      <FieldLabel className="text-foreground">{label}</FieldLabel>
      <Combobox<string, true>
        multiple
        items={items}
        filter={null}
        autoHighlight
        value={value}
        onValueChange={(next) => {
          onChange(next as string[]);
          setInput('');
        }}
        inputValue={input}
        onInputValueChange={setInput}
        itemToStringLabel={itemLabel}
        disabled={disabled}
      >
        <ComboboxChips>
          {value.map((item) => (
            <ComboboxChip
              key={item}
              aria-label={itemLabel(item)}
              className={cn(
                invalid?.(item) &&
                  'border-destructive-foreground text-destructive-foreground',
              )}
            >
              {itemLabel(item)}
            </ComboboxChip>
          ))}
          <ComboboxInput
            aria-invalid={hasError || undefined}
            placeholder={value.length ? '' : placeholder}
            onKeyDown={(event) => {
              if (event.key === 'Tab' && !event.shiftKey && custom.length) {
                event.preventDefault();
                if (!value.includes(typed)) onChange([...value, typed]);
                setInput('');
              }
            }}
          />
        </ComboboxChips>
        <ComboboxContent>
          <ComboboxEmpty>{emptyText}</ComboboxEmpty>
          <ComboboxList>
            {(item: string) => (
              <ComboboxItem key={item} value={item}>
                {custom.includes(item) ? `Add “${item}”` : itemLabel(item)}
              </ComboboxItem>
            )}
          </ComboboxList>
        </ComboboxContent>
      </Combobox>
      {error ? (
        <FieldError match className="text-xs">
          {error}
        </FieldError>
      ) : (
        problems.map((p) => (
          <FieldError key={p} match className="text-xs">
            {p}
          </FieldError>
        ))
      )}
      {!error && description && (
        <FieldDescription>{description}</FieldDescription>
      )}
    </Field>
  );
};
