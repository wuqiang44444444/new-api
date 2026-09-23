import { render, screen, waitFor, within } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { describe, expect, it, vi } from 'vitest'

import type { BillingDiscountCombination } from '@/features/billing-reconciliation/types'

import { CustomerUsageTable } from '../customer-usage-table'
import { UsageDiscountCells } from '../discount-details'

vi.mock('react-i18next', () => ({
  useTranslation: () => ({ t: (key: string) => key }),
}))

const combination: BillingDiscountCombination = {
  group_id: 11,
  model_name: 'public-model',
  billing_mode: 'token',
  group_name: 'Historical group',
  group_ratio_source: 'user_exclusive',
  group_ratio: 0.8,
  contract_applicable: 'yes',
  contract_id_known: true,
  contract_id: 3,
  contract_name: 'Historical contract',
  contract_version: 2,
  contract_ratio: 0.5,
  original_known: true,
  original_quota: 100,
  discount_quota: 60,
  usage: {
    requests: 1,
    billable_calls: 0,
    refunded_calls: 0,
    input_tokens: 10,
    output_tokens: 5,
    cache_read_tokens: 0,
    cache_write_tokens: 0,
    gross_quota: 50,
    refund_quota: 10,
    net_quota: 40,
  },
}

function DiscountRow(props: {
  combinations?: BillingDiscountCombination[]
  moneyIncomplete?: boolean
}) {
  return (
    <table>
      <thead>
        <tr>
          <th>Group discount/ratio</th>
          <th>Contract discount</th>
          <th>Final discount</th>
          <th>Estimated savings</th>
        </tr>
      </thead>
      <tbody>
        <tr>
          <UsageDiscountCells {...props} />
        </tr>
      </tbody>
    </table>
  )
}

function definition(label: string) {
  const headers = screen.getAllByRole('columnheader')
  const index = headers.indexOf(
    screen.getByRole('columnheader', { name: label })
  )
  return within(screen.getAllByRole('cell')[index])
}

describe('customer usage discount details', () => {
  it('shows historical group, contract version and combined factor', () => {
    render(<DiscountRow combinations={[combination]} />)
    expect(definition('Group discount/ratio').getByText(/×0.8/)).toBeVisible()
    expect(screen.getByText('User Exclusive Ratio')).toBeVisible()
    expect(screen.getByText('Historical contract · v2')).toBeVisible()
    expect(definition('Contract discount').getByText(/×0.5/)).toBeVisible()
    expect(definition('Final discount').getByText(/×0.4/)).toBeVisible()
    expect(screen.getByText('Estimated savings')).toBeVisible()
  })

  it('opens multiple discount versions without adding rows and closes with Escape', async () => {
    render(
      <DiscountRow
        combinations={[
          combination,
          { ...combination, contract_version: 3, contract_ratio: 0.9 },
        ]}
      />
    )
    expect(
      screen.queryByText('Historical contract · v2')
    ).not.toBeInTheDocument()
    const user = userEvent.setup()
    const trigger = screen.getByRole('button', { name: /Discount details/ })
    await user.click(trigger)
    expect(screen.getByText('Historical contract · v2')).toBeVisible()
    expect(screen.getByText('Historical contract · v3')).toBeVisible()
    expect(screen.getByText(/\(×0.72\)/)).toBeVisible()
    expect(screen.getAllByRole('row')).toHaveLength(2)
    expect(
      screen.getByRole('dialog', { name: /Discount details/ })
    ).toBeVisible()
    await user.keyboard('{Escape}')
    await waitFor(() =>
      expect(screen.queryByRole('dialog')).not.toBeInTheDocument()
    )
    expect(trigger).toHaveFocus()
  })

  it.each([
    { secondSavings: 40, expected: '$0.0002' },
    { secondSavings: undefined, expected: '-' },
  ])(
    'shows savings $expected when one combination has $secondSavings savings',
    ({ secondSavings, expected }) => {
      render(
        <DiscountRow
          combinations={[
            combination,
            {
              ...combination,
              contract_version: 3,
              discount_quota: secondSavings,
            },
          ]}
        />
      )
      expect(definition('Estimated savings').getByText(expected)).toBeVisible()
    }
  )

  it.each(['unrecorded', 'unknown'] as const)(
    'keeps %s contract evidence unknown',
    (state) => {
      render(
        <DiscountRow
          combinations={[
            {
              ...combination,
              contract_applicable: state,
              contract_ratio: undefined,
            },
          ]}
        />
      )
      expect(
        definition('Contract discount').getByText('Not recorded')
      ).toBeVisible()
      expect(
        definition('Final discount').getByText('Not recorded')
      ).toBeVisible()
      expect(
        screen.queryByText('No contract discount applied')
      ).not.toBeInTheDocument()
    }
  )

  it('distinguishes explicit no-contract records from missing history', () => {
    render(
      <DiscountRow
        combinations={[
          {
            ...combination,
            contract_applicable: 'no',
            contract_ratio: undefined,
          },
        ]}
      />
    )
    expect(screen.getByText('No contract discount applied')).toBeVisible()
    expect(definition('Final discount').getByText(/×0.8/)).toBeVisible()
  })

  it('suppresses factors when settlement money is incomplete', () => {
    render(<DiscountRow moneyIncomplete combinations={[combination]} />)
    expect(
      screen.getByText(/Discount details await complete settlement records/)
    ).toBeVisible()
    expect(screen.queryByText(/×0.4/)).not.toBeInTheDocument()
  })

  it('does not apply model discounts to auxiliary charges', () => {
    render(
      <DiscountRow
        combinations={[
          {
            ...combination,
            original_known: false,
            original_quota: undefined,
            discount_quota: undefined,
            estimate_reasons: ['auxiliary_charge'],
          },
        ]}
      />
    )
    expect(
      screen.getByText(
        'Additional fees are not included in the model discount.'
      )
    ).toBeVisible()
    expect(definition('Estimated savings').getByText('-')).toBeVisible()
  })

  it('shows discounts as columns in the same customer model row', () => {
    const metrics = {
      total_calls: 1,
      success_calls: 1,
      failure_calls: 0,
      cancelled_calls: 0,
      other_result_calls: 0,
      input_tokens: 10,
      output_tokens: 5,
      cache_read_tokens: 0,
      cache_write_tokens: 0,
      image_count: 0,
      gross_quota: 50,
      refund_quota: 10,
      net_quota: 40,
    }
    render(
      <CustomerUsageTable
        period={{
          period: 'day',
          date: '2026-09-21',
          start_timestamp: 1,
          end_timestamp: 86401,
          timezone: 'Asia/Shanghai',
          days: [{ date: '2026-09-21', start: 1, end: 86401 }],
        }}
        view={{
          total: metrics,
          day_totals: [metrics],
          keys: [
            {
              token_id: 11,
              token_name: 'Key',
              days: [metrics],
              total: metrics,
              models: [
                {
                  model_name: 'public-model',
                  total: metrics,
                  days: [metrics],
                  discount_combinations: [combination],
                },
              ],
            },
          ],
        }}
      />
    )
    const row = screen.getByRole('row', { name: /public-model/ })
    expect(within(row).getByText('Historical contract · v2')).toBeVisible()
    expect(within(row).getByText(/×0.4/)).toBeVisible()
    expect(within(row).getAllByRole('cell')).toHaveLength(11)
  })
})
