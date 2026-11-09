// Owner: U3 (newversion). A titled form section (title and intro on the
// left, fields on the right), the prototype's layout for create and new
// version pages.
import type { ReactNode } from 'react';

/** Width-limited column of the create and new version pages. */
export const PAGE_COLUMN = 'mx-auto w-full max-w-[1080px]';

export function FormSection({
  id,
  title,
  intro,
  children,
}: {
  id: string;
  title: ReactNode;
  intro: ReactNode;
  children: ReactNode;
}) {
  return (
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
}
