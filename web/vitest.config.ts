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
    include: ['src/features/docs/**/*.{test,spec}.{ts,tsx}'],
    setupFiles: ['./src/features/docs/__tests__/setup.ts'],
  },
})
