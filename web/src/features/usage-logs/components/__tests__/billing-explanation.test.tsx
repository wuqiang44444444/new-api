import { cleanup, render, screen } from '@testing-library/react'
import { afterEach, expect, it, vi } from 'vitest'

import { useSystemConfigStore } from '@/stores/system-config-store'

import { usageLogSchema } from '../../data/schema'
import { DetailsDialog } from '../dialogs/details-dialog'

vi.mock('react-i18next', () => ({
  useTranslation: () => ({ t: (key: string) => key }),
}))
vi.mock('@/features/pricing/hooks/use-pricing-data', () => ({
  usePricingData: () => ({ models: [] }),
}))
vi.mock('@/features/pricing/components/dynamic-pricing-breakdown', () => ({
  DynamicPricingBreakdown: () => null,
}))
afterEach(cleanup)

function show(other: object, type = 2) {
  const log = usageLogSchema.parse({
    id: 1,
    user_id: 1,
    created_at: 1000,
    type,
    content: '',
    model_name: 'model',
    quota: 100,
    other: JSON.stringify(other),
  })
  render(
    <DetailsDialog
      log={log}
      open
      isAdmin={false}
      isRoot={false}
      onOpenChange={() => {}}
    />
  )
}

it('shows the frozen usage times price equation supplied by the server', () => {
  show({
    model_ratio: 1,
    group_ratio: 0.5,
    contract_discount: 0.3,
    billing_explanation: {
      lines: [
        {
          label: 'Input',
          quantity: 700,
          unit: 'token',
          unit_price_usd: 2,
          subtotal_usd: 0.0014,
        },
      ],
      has_auxiliary_charge: false,
    },
  })
  expect(screen.getByText(/700 token × .*\/M token = /)).toBeTruthy()
})

it('does not reverse auxiliary charges by the model discount', () => {
  show({
    model_ratio: 1,
    group_ratio: 0.5,
    contract_discount: 0.3,
    fee_quota: 70,
  })
  expect(screen.queryByText('Estimated savings')).toBeNull()
  expect(screen.queryByText('Pre-discount subtotal (estimated)')).toBeNull()
  expect(screen.getByText('Auxiliary charges')).toBeTruthy()
})

it('uses the group factor as the final discount when no contract applies', () => {
  show({ model_ratio: 1, group_ratio: 0.5 })
  expect(screen.getByText('No contract')).toBeTruthy()
  expect(screen.getByText('Final discount')).toBeTruthy()
  expect(
    screen.getByText('Final discount').nextElementSibling
  ).toHaveTextContent('0.5000x')
})

it('keeps contradictory contract facts unknown', () => {
  show({
    model_ratio: 1,
    group_ratio: 0.5,
    contract_applicable: false,
    contract_discount: 0.3,
  })
  expect(screen.queryByText('Final discount')).toBeNull()
  expect(screen.queryByText('Estimated savings')).toBeNull()
})

it('identifies native prices converted with the current quota rate', () => {
  show({
    billing_explanation: {
      lines: [
        {
          label: 'Input',
          quantity: 1,
          unit: 'token',
          unit_price_usd: 1,
          subtotal_usd: 0.000001,
        },
      ],
      current_quota_conversion: true,
    },
  })
  expect(
    screen.getByText('Amounts use the current quota conversion rate.')
  ).toBeTruthy()
})

it('uses the configured quota conversion for fallback unit prices', () => {
  const currency = useSystemConfigStore.getState().config.currency
  try {
    useSystemConfigStore.getState().setConfig({
      currency: {
        ...currency,
        quotaPerUnit: 1000000,
        quotaDisplayType: 'USD',
      },
    })
    show({ model_ratio: 1, group_ratio: 1, contract_applicable: false })
    expect(screen.getByText('$1/M')).toBeTruthy()
    expect(screen.queryByText('$2/M')).toBeNull()
  } finally {
    useSystemConfigStore.getState().setConfig({ currency })
  }
})

it('does not render an unsafe original amount from a decimal string', () => {
  show({
    group_ratio: 0.5,
    contract_applicable: false,
    billing_explanation: {
      lines: [],
      original_quota_estimated: '9007199254740993',
    },
  })
  expect(screen.queryByText('Pre-discount subtotal (estimated)')).toBeNull()
  expect(screen.queryByText('Estimated savings')).toBeNull()
})

const recoveredFacts = {
  billing_mode: 'per_second',
  group_name: 'historical',
  group_ratio: 0.87,
  group_ratio_source: 'user_exclusive',
  contract_applicable: 'no',
  contract_ratio: null,
  contract_name: '',
  contract_version: 0,
  input_tokens: 1000,
  input_tokens_unavailable: false,
  output_tokens: 100,
  cache_read_tokens: 300,
  cache_write_tokens: 0,
}

it('shows recovered refund discounts without presenting a new metered charge', () => {
  show({ billing_facts: recoveredFacts }, 6)
  expect(screen.getByText('Duration billing')).toBeTruthy()
  expect(screen.getByText('No contract')).toBeTruthy()
  expect(screen.getAllByText('×0.87')).toHaveLength(2)
  expect(screen.queryByText('Pre-discount subtotal (estimated)')).toBeNull()
})

it('uses the recovered statement input semantic after admin evidence is removed', () => {
  show({ billing_facts: { ...recoveredFacts, billing_mode: 'token' } })
  expect(screen.getByText('1,000')).toBeTruthy()
  expect(screen.getByText('300')).toBeTruthy()
})

it('keeps an unproven statement input total unknown', () => {
  show({
    billing_facts: {
      ...recoveredFacts,
      input_tokens: 0,
      input_tokens_unavailable: true,
    },
  })
  expect(screen.getByText('Not recorded')).toBeTruthy()
  expect(screen.getByText('300')).toBeTruthy()
})

it('preserves explicitly unknown recovered contract facts instead of inventing a final factor', () => {
  show(
    { billing_facts: { ...recoveredFacts, contract_applicable: 'unknown' } },
    6
  )
  expect(screen.queryByText('No contract')).toBeNull()
  expect(
    screen.getByText('Final discount').nextElementSibling
  ).toHaveTextContent('—')
})
