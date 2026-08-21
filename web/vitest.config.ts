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
      'src/components/__tests__/**/*.{test,spec}.{ts,tsx}',
      'src/features/billing-reconciliation/**/*.{test,spec}.{ts,tsx}',
      'src/features/channels/components/**/*.{test,spec}.{ts,tsx}',
      'src/features/channels/lib/__tests__/channel-asset-credential-transform.test.ts',
      'src/features/channels/lib/__tests__/moxing-image-channel.test.ts',
      'src/features/channels/lib/__tests__/official-channel-connectivity.test.ts',
      'src/features/channels/lib/__tests__/seedance-protocol-validation.test.ts',
      'src/features/customer-contracts/**/*.{test,spec}.{ts,tsx}',
      'src/features/docs/**/*.{test,spec}.{ts,tsx}',
      'src/hooks/__tests__/**/*.{test,spec}.{ts,tsx}',
      'src/features/keys/components/**/customer-contract*.{test,spec}.{ts,tsx}',
      'src/features/system-settings/models/**/model-pricing-switch*.{test,spec}.{ts,tsx}',
      'src/features/users/components/**/user-contract*.{test,spec}.{ts,tsx}',
    ],
    setupFiles: ['./src/features/docs/__tests__/setup.ts'],
  },
})
