import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import { fireEvent, render, screen } from '@testing-library/react'
import { createInstance } from 'i18next'
import { I18nextProvider } from 'react-i18next'
import { describe, expect, it, vi } from 'vitest'

import en from '@/i18n/locales/en.json'
import zh from '@/i18n/locales/zh.json'

import type { TaskCalculation } from '../../lib/billing-calculation'
import {
  BillingCalculationView,
  TaskBillingCalculation,
} from '../task-billing-calculation'

const { get } = vi.hoisted(() => ({ get: vi.fn() }))
vi.mock('@/lib/api', () => ({ api: { get } }))

async function setup(language = 'en') {
  const i18n = createInstance()
  await i18n.init({ lng: language, fallbackLng: 'en', resources: { en, zh } })
  return i18n
}
const recorded: TaskCalculation = {
  quota: 80,
  state: 'settled',
  source: 'settlement',
  evidence: 'complete',
  initial_evidence: 'historical',
  settlement: {
    version: 1,
    quota: 80,
    steps: [
      {
        op: 'retired_host_operation',
        formula: '20 × 4',
        inputs: ['20', '4'],
        result: '80',
        unit: 'quota',
      },
    ],
  },
}

describe('recorded charge calculation', () => {
  it.each(['en', 'zh'])(
    'explains an uncharged batch line without inventing a calculation in %s',
    async (language) => {
      const i18n = await setup(language)
      render(
        <I18nextProvider i18n={i18n}>
          <BillingCalculationView
            data={{
              quota: 0,
              state: 'settled',
              source: 'not_charged',
              evidence: 'complete',
              initial_evidence: 'complete',
              initial: { version: 1, quota: 100 },
              refunded_quota: 100,
            }}
          />
        </I18nextProvider>
      )
      expect(
        screen.getByText(
          i18n.t(
            'Complete batch results confirm no charge for this request. Any precharge is released when settlement completes.'
          )
        )
      ).toBeVisible()
      expect(screen.getByText(/100 − 100 = 0/)).toBeVisible()
      expect(
        screen.queryByRole('button', {
          name: i18n.t('Original calculation records'),
        })
      ).not.toBeInTheDocument()
      expect(screen.queryByRole('status')).not.toBeInTheDocument()
      fireEvent.click(
        screen.getByRole('button', { name: i18n.t('Precharge calculation') })
      )
      expect(
        screen.getByText(`${i18n.t('Calculated quota')}: 100`)
      ).toBeVisible()
    }
  )

  it('explains a pending precharge before exposing technical records', async () => {
    const i18n = await setup()
    render(
      <I18nextProvider i18n={i18n}>
        <BillingCalculationView
          data={{
            quota: 147929,
            state: 'pending',
            source: 'initial',
            evidence: 'complete',
            initial_evidence: 'complete',
            initial: {
              version: 1,
              quota: 147929,
              nodes: [
                { id: 1, op: 'usage:duration_seconds' },
                { id: 2, op: 'literal', literal: '0.5' },
                { id: 3, op: '*', args: [1, 2] },
                { id: 4, op: 'usd_exchange_rate' },
                { id: 5, op: '/', args: [3, 4] },
                { id: 6, op: 'usage:resolution' },
              ],
              values: [
                { node: 1, value: '4' },
                { node: 3, value: '2' },
                { node: 4, value: '6.76' },
                { node: 5, value: '0.2958579881656805' },
                { node: 6, value: 'protected' },
              ],
            },
          }}
        />
      </I18nextProvider>
    )
    expect(screen.getByText('Precharged amount')).toBeVisible()
    expect(
      screen.getByText(
        'The amount has been reserved. The final charge will be confirmed when the task finishes.'
      )
    ).toBeVisible()
    expect(screen.getByText('Base charge (CNY)')).toBeVisible()
    expect(screen.getByText('4 × 0.5 = 2')).toBeVisible()
    expect(screen.getByText('2 ÷ 6.76 ≈ 0.295858')).toBeVisible()
    expect(screen.queryByText(/#3:/)).not.toBeInTheDocument()
    expect(
      screen.queryByText(/Protected condition value/)
    ).not.toBeInTheDocument()
    const toggle = screen.getByRole('button', {
      name: 'Original calculation records',
    })
    expect(toggle).toHaveAttribute('aria-expanded', 'false')
    fireEvent.click(toggle)
    expect(toggle).toHaveAttribute('aria-expanded', 'true')
    expect(screen.getByText(/#3:/)).toBeVisible()
    expect(
      screen
        .getAllByText(/0.2958579881656805/)
        .some((element) => element.textContent?.includes('0.2958579881656805'))
    ).toBe(true)
  })

  it('does not claim historical missing records are a recorded precharge', async () => {
    const i18n = await setup()
    render(
      <I18nextProvider i18n={i18n}>
        <BillingCalculationView
          data={{
            quota: 70,
            state: 'settled',
            source: 'initial',
            evidence: 'historical',
            initial_evidence: 'historical',
          }}
        />
      </I18nextProvider>
    )
    expect(
      screen.queryByText(/Uses the recorded precharge/)
    ).not.toBeInTheDocument()
    expect(
      screen.queryByRole('button', { name: 'Original calculation records' })
    ).not.toBeInTheDocument()
    expect(screen.getByRole('status')).toBeVisible()
  })

  it('shows recorded operands and preserves the actual amount when historical precharge is unavailable', async () => {
    const i18n = await setup()
    render(
      <I18nextProvider i18n={i18n}>
        <BillingCalculationView data={recorded} />
      </I18nextProvider>
    )
    expect(screen.getByText('20 × 4 = 80')).toBeInTheDocument()
    fireEvent.click(
      screen.getByRole('button', { name: 'Precharge calculation' })
    )
    expect(
      await screen.findByText(
        'Billing details were not fully recorded. The calculation cannot be reconstructed.'
      )
    ).toBeVisible()
    expect(screen.getByText('80 Quota')).toBeVisible()
  })

  it('distinguishes a missing new record from unavailable history', async () => {
    const i18n = await setup('zh')
    render(
      <I18nextProvider i18n={i18n}>
        <BillingCalculationView
          data={{ ...recorded, settlement: undefined, evidence: 'missing' }}
        />
      </I18nextProvider>
    )
    expect(screen.getByRole('status')).toHaveTextContent(
      '计费过程记录缺失或与金额不一致'
    )
    expect(screen.getByRole('heading', { name: '扣费计算过程' })).toBeVisible()
  })

  it('shows a loading error and retries without hiding the request failure as absent evidence', async () => {
    get
      .mockRejectedValueOnce(new Error('network'))
      .mockResolvedValueOnce({ data: { success: true, data: recorded } })
    const i18n = await setup()
    const client = new QueryClient({
      defaultOptions: { queries: { retry: false } },
    })
    render(
      <I18nextProvider i18n={i18n}>
        <QueryClientProvider client={client}>
          <TaskBillingCalculation taskId='task-1' />
        </QueryClientProvider>
      </I18nextProvider>
    )
    fireEvent.click(await screen.findByRole('button', { name: 'Retry' }))
    expect(await screen.findByText('20 × 4 = 80')).toBeInTheDocument()
    expect(get).toHaveBeenLastCalledWith('/api/task/task-1/billing', {
      params: undefined,
    })
  })
  it('shows the actual refund separately from the retained price calculation', async () => {
    const i18n = await setup('zh')
    render(
      <I18nextProvider i18n={i18n}>
        <BillingCalculationView
          data={{
            ...recorded,
            quota: 0,
            state: 'refunded',
            refunded_quota: 50,
            waived_quota: 30,
          }}
        />
      </I18nextProvider>
    )
    expect(screen.getByText('20 × 4 = 80')).toBeVisible()
    expect(screen.getByText(/已退额度/)).toHaveTextContent('50 − 50 = 0')
    expect(screen.getByText(/已免收差额/)).toHaveTextContent('30')
  })

  it('loads only the selected batch line through the protected detail endpoint', async () => {
    get
      .mockReset()
      .mockResolvedValue({ data: { success: true, data: recorded } })
    const i18n = await setup()
    const client = new QueryClient({
      defaultOptions: { queries: { retry: false } },
    })
    render(
      <I18nextProvider i18n={i18n}>
        <QueryClientProvider client={client}>
          <TaskBillingCalculation taskId='batch-1' batchLine='request/2' />
        </QueryClientProvider>
      </I18nextProvider>
    )
    expect(await screen.findByText('20 × 4 = 80')).toBeVisible()
    expect(get).toHaveBeenCalledExactlyOnceWith(
      '/api/batch/batch-1/billing/calculation',
      { params: { custom_id: 'request/2' } }
    )
  })
})
