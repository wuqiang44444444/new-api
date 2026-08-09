import path from 'node:path'
import { fileURLToPath } from 'node:url'

import { defineConfig } from 'vitest/config'

const rootDirectory = path.dirname(fileURLToPath(import.meta.url))

export default defineConfig({
  resolve: {
    alias: {
      '@': path.resolve(rootDirectory, './src'),
    },
  },
  test: {
    environment: 'happy-dom',
    include: [
      'src/features/billing-reconciliation/**/*.{test,spec}.{ts,tsx}',
      'src/features/channels/components/**/*.{test,spec}.{ts,tsx}',
      'src/features/channels/lib/__tests__/link-access-plan.test.ts',
      'src/features/docs/**/*.{test,spec}.{ts,tsx}',
      'src/hooks/__tests__/**/*.{test,spec}.{ts,tsx}',
    ],
    setupFiles: ['./src/features/docs/__tests__/setup.ts'],
  },
})
