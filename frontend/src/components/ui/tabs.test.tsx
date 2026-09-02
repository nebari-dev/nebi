import { describe, expect, it } from 'vitest';
import { tabsTabVariants } from './tabs';

function classesFor(variant: 'pill' | 'toggle') {
  return tabsTabVariants({ variant }).split(' ');
}

describe('tabsTabVariants', () => {
  it.each(['pill', 'toggle'] as const)(
    'keeps inactive %s tabs transparent and styles only the active tab',
    (variant) => {
      const classes = classesFor(variant);

      expect(classes).toContain('border-transparent');
      expect(classes).not.toContain('border-border');
      expect(classes).not.toContain('bg-card');
      expect(classes).toContain('data-[active]:border-border-strong');
      expect(classes).toContain('data-[active]:bg-card');
      expect(classes).toContain(
        'data-[active]:shadow-[0_1px_1.5px_rgb(0_0_0_/_0.1)]',
      );
    },
  );
});
