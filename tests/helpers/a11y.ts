import { expect, type Page } from '@playwright/test';
import AxeBuilder from '@axe-core/playwright';

export async function checkA11y(page: Page, context?: string) {
  const results = await new AxeBuilder({ page })
    .withTags(['wcag2a', 'wcag2aa'])
    .analyze();

  const violations = results.violations;
  if (violations.length === 0) return;

  const summary = violations
    .map((v) => {
      const nodes = v.nodes.map((n) => `    - ${n.target.join(', ')}`).join('\n');
      return `[${v.impact}] ${v.id}: ${v.description}\n${nodes}`;
    })
    .join('\n\n');

  const label = context ? ` (${context})` : '';
  expect.soft(violations, `Accessibility violations${label}:\n\n${summary}`).toHaveLength(0);
}
