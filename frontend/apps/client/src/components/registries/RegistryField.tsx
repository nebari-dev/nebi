// Owner: U7 (publish). Labelled form field (label, control, description or
// error) used by the registry dialogs; same layout as the prototype's
// connect-server form fields.
import {
  Field,
  FieldDescription,
  FieldError,
  FieldLabel,
} from '@nebi/ui/components/ui/field';
import type { ReactNode } from 'react';

export interface RegistryFieldProps {
  label: ReactNode;
  description?: ReactNode;
  error?: string | null;
  children: ReactNode;
}

export const RegistryField = ({
  label,
  description,
  error,
  children,
}: RegistryFieldProps) => (
  <Field invalid={!!error} className="w-full min-w-0 gap-1">
    <FieldLabel className="text-foreground">{label}</FieldLabel>
    {children}
    {error ? (
      <FieldError match className="text-xs">
        {error}
      </FieldError>
    ) : (
      description && <FieldDescription>{description}</FieldDescription>
    )}
  </Field>
);
