// Owner: U3 (newversion). Tags field with chips and suggestions (the
// prototype's tag combobox). A local copy until the create project page's
// tag field is shared.
import {
  Combobox,
  ComboboxChip,
  ComboboxChips,
  ComboboxContent,
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
import { useState } from 'react';

export interface TagsInputProps {
  label: string;
  value: string[];
  onChange: (tags: string[]) => void;
  suggestions?: string[];
  placeholder?: string;
  description?: string;
  /** Returns an error message for an invalid tag. */
  invalid?: (tag: string) => string | null;
  /** Server-side error for the field. */
  error?: string | null;
}

export function TagsInput({
  label,
  value,
  onChange,
  suggestions = [],
  placeholder,
  description,
  invalid,
  error,
}: TagsInputProps) {
  const [input, setInput] = useState('');
  const typed = input.trim();
  const lower = typed.toLowerCase();
  const known = [...new Set([...suggestions, ...value])];
  const fresh = typed && !known.includes(typed) ? [typed] : [];
  const items = [
    ...known.filter((t) => !lower || t.toLowerCase().includes(lower)),
    ...fresh,
  ];
  const errors = value.map((t) => invalid?.(t)).filter((e): e is string => !!e);
  if (error && !errors.includes(error)) errors.push(error);

  return (
    <Field invalid={errors.length > 0} className="w-full min-w-0 gap-1">
      <FieldLabel className="text-foreground">{label}</FieldLabel>
      <Combobox
        multiple
        items={items}
        filter={null}
        autoHighlight
        value={value}
        onValueChange={(next: string[]) => {
          onChange(next);
          setInput('');
        }}
        inputValue={input}
        onInputValueChange={setInput}
      >
        <ComboboxChips>
          {value.map((tag) => (
            <ComboboxChip
              key={tag}
              className={cn(
                invalid?.(tag) &&
                  'border-destructive-foreground text-destructive-foreground',
              )}
            >
              {tag}
            </ComboboxChip>
          ))}
          <ComboboxInput
            placeholder={placeholder}
            aria-label={label}
            aria-invalid={errors.length > 0 || undefined}
            onKeyDown={(e) => {
              if (e.key === 'Tab' && !e.shiftKey && typed) {
                e.preventDefault();
                if (!value.includes(typed)) onChange([...value, typed]);
                setInput('');
              }
            }}
          />
        </ComboboxChips>
        {items.length > 0 && (
          <ComboboxContent>
            <p className="px-1.5 py-1 text-muted-foreground text-xs">
              Type a name and press Enter.
            </p>
            <ComboboxList>
              {(tag: string) => (
                <ComboboxItem key={tag} value={tag}>
                  {fresh.includes(tag) ? `Add “${tag}”` : tag}
                </ComboboxItem>
              )}
            </ComboboxList>
          </ComboboxContent>
        )}
      </Combobox>
      {errors.map((e) => (
        <FieldError key={e} match className="text-xs">
          {e}
        </FieldError>
      ))}
      {description && <FieldDescription>{description}</FieldDescription>}
    </Field>
  );
}
