// Owner: U1 (projects). Layout pieces of the Create project / New version
// pages (prototype): a two-column section with a heading and intro, and a
// labelled field row with description or error.
import {
  Field,
  FieldDescription,
  FieldError,
  FieldLabel,
} from '@nebi/ui/components/ui/field';
import type { ReactNode } from 'react';

/** Centered page column of the create/new-version pages. */
export const FORM_COLUMN = 'mx-auto w-full max-w-[1080px]';

export const FormSection = ({
  id,
  title,
  intro,
  children,
}: {
  id: string;
  title: ReactNode;
  intro: ReactNode;
  children: ReactNode;
}) => (
  <section
    aria-labelledby={`${id}-heading`}
    className="grid grid-cols-[280px_minmax(0,1fr)] gap-12 border-border border-t py-6 first:border-t-0 max-[900px]:grid-cols-1 max-[900px]:gap-4"
  >
    <div className="flex flex-col gap-1">
      <h2
        id={`${id}-heading`}
        className="font-semibold text-base text-foreground"
      >
        {title}
      </h2>
      <p className="text-muted-foreground text-sm">{intro}</p>
    </div>
    <div className="flex min-w-0 flex-col gap-6">{children}</div>
  </section>
);

export const FormRow = ({
  label,
  description,
  error,
  children,
}: {
  label: ReactNode;
  description?: ReactNode;
  error?: string | null;
  children: ReactNode;
}) => (
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
