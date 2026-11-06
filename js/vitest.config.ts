import { defineConfig } from 'vitest/config';

export default defineConfig({
  test: { include: ['kits/*/tests/**/*.test.ts', 'sandbox/tests/**/*.test.ts'] },
});
