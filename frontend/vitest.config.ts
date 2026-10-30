import { defineConfig } from 'vitest/config';

export default defineConfig({
  test: {
    projects: [
      'apps/client/vitest.config.ts',
      'apps/server/vitest.config.ts',
      'packages/ui/vitest.config.ts',
    ],
  },
});
