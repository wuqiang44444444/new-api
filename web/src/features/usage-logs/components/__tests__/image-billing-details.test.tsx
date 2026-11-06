import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import { cleanup, render, screen, within } from '@testing-library/react'
import { afterEach, beforeEach, expect, it } from 'vitest'

import { getPricingQueryKey } from '@/features/pricing/hooks/use-pricing-data'
import type { BillingDisplayProjection } from '@/features/pricing/types'
import {
  DEFAULT_CURRENCY_CONFIG,
  useSystemConfigStore,
} from '@/stores/system-config-store'

import { usageLogSchema } from '../../data/schema'
import { DetailsDialog } from '../dialogs/details-dialog'

const projection: BillingDisplayProjection = {
  expression_hash: 'image-pricing-fixture',
  display_version: 2,
  expression_version: 1,
  status: 'exact',
  unit: 'usd_per_million_tokens',
  tiers: [
    {
      label: 'base',
      unit_prices: { p: 5, c: 30, cr: 1.25, img_cr: 0, img: 8 },
    },
  ],
}
let queryClient: QueryClient
let previousCurrency: typeof DEFAULT_CURRENCY_CONFIG

beforeEach(() => {
  previousCurrency = useSystemConfigStore.getState().config.currency
  useSystemConfigStore
    .getState()
    .setConfig({
      currency: { ...DEFAULT_CURRENCY_CONFIG, quotaDisplayType: 'USD' },
    })
  queryClient = new QueryClient({
    defaultOptions: { queries: { retry: false, staleTime: Infinity } },
  })
  queryClient.setQueryData(getPricingQueryKey(undefined), {
    data: [],
    vendors: [],
  })
})
afterEach(() => {
  cleanup()
  queryClient.clear()
  useSystemConfigStore.getState().setConfig({ currency: previousCurrency })
})

it.each([
  { reported: false, label: 'Upstream did not report cache usage' },
  { reported: true, label: 'Reported by upstream' },
  { reported: undefined, label: 'Not recorded' },
])(
  'keeps all configured cache prices when cache meter is $reported',
  ({ reported, label }) => {
    const other = {
      billing_mode: 'tiered_expr',
      request_path: '/v1/images/generations',
      matched_tier: 'base',
      expr_b64: btoa(
        'tier("base", p * 5 + cr * 1.25 + img * 8 + img_cr * 0 + c * 30)'
      ),
      billing_display: projection,
      cache_tokens: 0,
      image_cache_tokens: 0,
      cache_read_tokens_reported: reported,
      billing_tokens: { p: 76, c: 1372, cr: 0, img: 0, img_cr: 0, img_o: 1372 },
      billing_explanation: { lines: [] },
    }
    const log = usageLogSchema.parse({
      id: 1,
      user_id: 1,
      created_at: 1000,
      type: 2,
      content: '',
      model_name: 'public-image',
      quota: 20770,
      prompt_tokens: 76,
      completion_tokens: 1372,
      other: JSON.stringify(other),
    })
    render(
      <QueryClientProvider client={queryClient}>
        <DetailsDialog
          log={log}
          open
          isAdmin={false}
          isRoot={false}
          onOpenChange={() => {}}
        />
      </QueryClientProvider>
    )
    const table = screen.getByRole('table')
    expect(within(table).getByRole('columnheader', { name: 'Text Input' })).toBeVisible()
    expect(
      within(table).getByRole('columnheader', { name: 'Cached Text Input' })
    ).toBeVisible()
    expect(
      within(table).getByRole('columnheader', { name: 'Cached Image Input' })
    ).toBeVisible()
    expect(within(table).getByText('$0.00')).toBeVisible()
    expect(
      screen.getByText('Cache usage evidence').nextElementSibling
    ).toHaveTextContent(label)
    expect(screen.getByText('$1.25/M')).toBeVisible()
  }
)
