// Owner: U5 (remote). Label + input + inline error, as the prototype's
// dialog fields ("Server URL", "Token", "Local name").
import { Input, type InputProps } from '@nebi/ui/components/ui/input';
import { useId } from 'react';

export interface TextFieldProps extends Omit<InputProps, 'id'> {
  label: string;
  error?: string;
}

export const TextField = ({ label, error, ...input }: TextFieldProps) => {
  const id = useId();
  const errorId = `${id}-error`;
  return (
    <div className="grid gap-1">
      <label htmlFor={id} className="font-medium text-foreground text-sm">
        {label}
      </label>
      <Input
        id={id}
        aria-invalid={error ? true : undefined}
        aria-describedby={error ? errorId : undefined}
        {...input}
      />
      {error && (
        <p id={errorId} className="text-destructive-foreground text-xs">
          {error}
        </p>
      )}
    </div>
  );
};
