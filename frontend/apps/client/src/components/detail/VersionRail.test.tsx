import { TooltipProvider } from '@nebi/ui/components/ui/tooltip';
import { render, screen, within } from '@testing-library/react';
import { describe, expect, it, vi } from 'vitest';
import { mockDetailVersions } from '@/test/handlers/detail';
import type { VersionItem } from '@/types/contracts';
import { VersionRail } from './VersionRail';

const ctx = { linked: true, serverLatest: 6 };

const rail = (versions: VersionItem[]) => (
  <TooltipProvider>
    <VersionRail
      versions={versions}
      selected={7}
      installingVersion={null}
      filterContext={ctx}
      onSelect={vi.fn()}
    />
  </TooltipProvider>
);

const row = (n: number) =>
  within(screen.getByRole('navigation', { name: 'Version history' }))
    .getByText(`v${n}`)
    .closest('button') as HTMLElement;

describe('VersionRail', () => {
  it('lights up only versions that appear while it is showing', () => {
    const { rerender } = render(rail(mockDetailVersions));
    expect(row(7)).not.toHaveAttribute('data-just-created');

    const created: VersionItem = {
      ...mockDetailVersions[0],
      number: 8,
      description: 'Added polars',
      tags: [],
      installed: false,
      sync: 'newest_here',
    };
    rerender(rail([created, ...mockDetailVersions]));
    expect(row(8)).toHaveAttribute('data-just-created');
    expect(row(7)).not.toHaveAttribute('data-just-created');
  });

  it('marks the parts of the In sync marker for its animation', () => {
    const synced = mockDetailVersions.map((v) => ({
      ...v,
      sync: v.number === 7 ? ('in_sync' as const) : null,
    }));
    render(rail(synced));
    const marker = row(7).querySelector('[data-sync="in_sync"]');
    expect(marker?.querySelector('[data-sync-part="laptop"]')).not.toBeNull();
    expect(marker?.querySelector('[data-sync-part="server"]')).not.toBeNull();
    expect(marker?.querySelector('[data-sync-part="label"]')).toHaveTextContent(
      'In sync',
    );
  });
});
