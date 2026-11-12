import { cleanup, render, screen } from '@testing-library/react'
import { afterEach, expect, it, vi } from 'vitest'

import { formatLogQuota } from '@/lib/format'
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

function show(other: object, type = 2, quota = 100) {
  const log = usageLogSchema.parse({
    id: 1,
    user_id: 1,
    created_at: 1000,
    type,
    content: '',
    model_name: 'model',
    quota,
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
  expect(screen.getByText('Input').nextElementSibling).toHaveTextContent(
    /700 token × .*\/M token = /
  )
})

it('shows text and image component amounts without adding informational image tokens again', () => {
  const currency = useSystemConfigStore.getState().config.currency
  try {
    useSystemConfigStore.getState().setConfig({
      currency: { ...currency, quotaDisplayType: 'USD' },
    })
    show({
      billing_mode: 'tiered_expr',
      group_ratio: 1,
      contract_applicable: false,
      billing_explanation: {
        lines: [
          {
            label: 'Text Input',
            quantity: 76,
            unit: 'token',
            unit_price_usd: 5,
            subtotal_usd: 0.00038,
          },
          {
            label: 'Image Out',
            quantity: 1372,
            unit: 'token',
            unit_price_usd: 30,
            subtotal_usd: 0.04116,
          },
        ],
      },
    })
    expect(screen.getByText('= $0.00038')).toHaveClass('whitespace-nowrap')
    expect(screen.getByText('× $5/M token')).toHaveClass('whitespace-nowrap')
    expect(screen.getByText('Text Input').nextElementSibling).toHaveTextContent(
      '76 token × $5/M token = $0.00038'
    )
    expect(
      screen.getByText('Text input price').nextElementSibling
    ).toHaveTextContent('$5/M')
    expect(screen.getByText('Image Out').nextElementSibling).toHaveTextContent(
      '1,372 token × $30/M token = $0.04116'
    )
  } finally {
    useSystemConfigStore.getState().setConfig({ currency })
  }
})

it('explains unavailable historical components while retaining the recorded total', () => {
  show({ billing_mode: 'tiered_expr', billing_explanation: { lines: [] } })
  expect(
    screen.getByText(
      'Itemized costs cannot be verified from the recorded billing evidence. The recorded charge is unchanged.'
    )
  ).toBeVisible()
  expect(screen.getByText('Total Cost')).toBeVisible()
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
    billing_facts: { recorded_original: '9007199254740993' },
  })
  expect(screen.getByText('List price').nextElementSibling).toHaveTextContent(
    'Cannot calculate'
  )
  expect(screen.getByText('Savings').nextElementSibling).toHaveTextContent(
    'Cannot calculate'
  )
})

it('shows the source and inferred meter of a unique replay', () => {
  show(
    {
      billing_facts: {
        recorded_original: '15860',
        original_source: 'unique_settlement_replay',
        derived_output_image_tokens: 1056,
      },
    },
    2,
    11737
  )
  expect(
    screen.getByText('Unique solution verified against the actual charge')
  ).toBeVisible()
  expect(
    screen.getByText('Derived output image tokens').nextElementSibling
  ).toHaveTextContent('1,056')
})

it('keeps task charges and the frozen contract difference separate', () => {
  show(
    {
      billing_facts: {
        recorded_original: '0',
        contract_settlement: {
          expected_quota: 146000,
          actual_quota: 200000,
          delta_quota: 54000,
        },
      },
    },
    2,
    54000
  )
  expect(
    screen.getByText('Expected task charge under frozen contract')
      .nextElementSibling
  ).toHaveTextContent(formatLogQuota(146000))
  expect(
    screen.getByText('Actual task charge').nextElementSibling
  ).toHaveTextContent(formatLogQuota(200000))
  expect(
    screen.getByText('Task charge difference (actual minus expected)')
      .nextElementSibling
  ).toHaveTextContent(formatLogQuota(54000))
})

it('explains a missing list price while keeping the recorded charge visible', () => {
  show({
    billing_facts: { estimate_reasons: ['original_replay_missing_usage'] },
  })
  expect(screen.getByText('List price').nextElementSibling).toHaveTextContent(
    'Historical metered usage is incomplete'
  )
  expect(screen.getByText('Savings').nextElementSibling).toHaveTextContent(
    'Cannot calculate'
  )
  expect(screen.getByText('Total Cost').nextElementSibling).toHaveTextContent(
    formatLogQuota(100)
  )
})

it.each([
  [119774, 89831, 29943],
  [0, 0, 0],
])(
  'shows verified list price %i and savings from the recorded charge',
  (original, charged, savings) => {
    show({ billing_facts: { recorded_original: String(original) } }, 2, charged)
    expect(screen.getByText('List price').nextElementSibling).toHaveTextContent(
      formatLogQuota(original)
    )
    expect(screen.getByText('Savings').nextElementSibling).toHaveTextContent(
      formatLogQuota(savings)
    )
  }
)

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
