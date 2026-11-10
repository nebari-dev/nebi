// Frozen. Temporary body for pages a unit has not built yet (issue #649).
import type { ReactNode } from 'react';

export const PagePlaceholder = ({
  title,
  subtitle,
  children,
}: {
  title: ReactNode;
  subtitle?: ReactNode;
  children?: ReactNode;
}) => (
  <main className="flex flex-col gap-5 px-12 py-12">
    <header className="flex flex-col gap-1.5">
      <h1 className="font-bold text-3xl text-foreground">{title}</h1>
      {subtitle && (
        <p className="text-base text-muted-foreground">{subtitle}</p>
      )}
    </header>
    {children}
  </main>
);
