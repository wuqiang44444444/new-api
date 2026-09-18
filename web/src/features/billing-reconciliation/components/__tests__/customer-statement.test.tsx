import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import { fireEvent, render, screen } from '@testing-library/react'
import { createInstance } from 'i18next'
import { I18nextProvider, initReactI18next } from 'react-i18next'
import { expect, it, vi } from 'vitest'

import zh from '@/i18n/locales/zh.json'

import type { BillingDataQuality } from '../../types'
import { CustomerStatementView } from '../customer-statement'

it('shows token usage and explains savings against the net settled amount', async () => {
  const i18n = createInstance().use(initReactI18next)
  await i18n.init({ lng: 'en', resources: { en: { translation: {} } } })
  const client = new QueryClient({
    defaultOptions: { queries: { retry: false } },
  })
  const usage = {
    requests: 293,
    billable_calls: 0,
    refunded_calls: 0,
    input_tokens: 0,
    output_tokens: 48750864,
    cache_read_tokens: 0,
    cache_write_tokens: 0,
    gross_quota: 820810200,
    refund_quota: 672363677,
    net_quota: 148446523,
  }
  client.setQueryData(
    [
      'billing-customer-reconciliation',
      false,
      undefined,
      'api_key',
      1000,
      1200,
      null,
    ],
    {
      result: {
        user_id: 91,
        username: 'customer',
        current_balance: 0,
        summary: usage,
        original_quota: 170628187,
        discount_quota: 22181664,
        groups: [
          {
            id: 0,
            name: 'key',
            usage,
            original_quota: 170628187,
            discount_quota: 22181664,
            models: [
              {
                model_name: 'customer-video',
                billing_mode: 'token',
                usage,
                original_quota: 170628187,
                discount_ratio: 0.87,
                detail_filter: {
                  start_timestamp: 1000,
                  end_timestamp: 1200,
                  token_id: 0,
                },
              },
            ],
          },
        ],
      },
    }
  )
  const view = render(
    <I18nextProvider i18n={i18n}>
      <QueryClientProvider client={client}>
        <CustomerStatementView
          isAdmin={false}
          dimension='api_key'
          period={{ start_timestamp: 1000, end_timestamp: 1200 }}
          onDimensionChange={vi.fn()}
        />
      </QueryClientProvider>
    </I18nextProvider>
  )
  expect(
    screen.getByText('Estimated list price minus net settled amount')
  ).toBeTruthy()
  expect(
    screen.getByRole('button', { name: 'Export and download' })
  ).toBeTruthy()
  fireEvent.click(screen.getByRole('button', { name: 'Expand models' }))
  expect(screen.getByText('Token billing')).toBeTruthy()
  expect(screen.queryByText('Per-call billing')).toBeNull()
  expect(screen.getByText(/Output 48,750,864/)).toBeTruthy()
  const detail = new URL(
    screen.getByRole('button', { name: 'View details' }).getAttribute('href') ??
      '',
    'http://localhost'
  )
  expect(detail.searchParams.get('tokenId')).toBe('0')
  expect(detail.searchParams.get('billing')).toBe('true')
  expect(detail.searchParams.get('billingUserId')).toBe('91')
  expect(detail.searchParams.get('billingMode')).toBe('token')
  expect(detail.searchParams.has('username')).toBe(false)
  view.unmount()
  client.clear()
})

async function renderQualityStatement(
  quality: BillingDataQuality,
  language = 'en',
  balance: number | null = 1781719699,
  mode = 'unknown'
) {
  const i18n = createInstance().use(initReactI18next)
  await i18n.init({ lng: language, resources: { en: { translation: {} }, zh } })
  const client = new QueryClient()
  const usage = {
    requests: 153,
    billable_calls: 0,
    refunded_calls: 0,
    input_tokens: 0,
    output_tokens: 0,
    cache_read_tokens: 0,
    cache_write_tokens: 0,
    gross_quota: 230573421,
    refund_quota: 58175440,
    net_quota: 172397981,
  }
  client.setQueryData(
    ['billing-customer-reconciliation', true, 91, 'channel', 1000, 1200, null],
    {
      result: {
        user_id: 91,
        username: 'randy',
        current_balance: balance,
        summary: { ...usage, requests: 12313, net_quota: 831775936 },
        original_quota: 956064294,
        discount_quota: 124288358,
        data_quality: quality,
        groups: [
          {
            id: 97,
            name: 'Channel #97',
            deleted: true,
            usage,
            models: [
              {
                model_name: 'video',
                billing_mode: mode,
                usage,
                detail_filter: {
                  start_timestamp: 1000,
                  end_timestamp: 1200,
                  channel_id: 97,
                },
              },
            ],
          },
          { id: 76, name: 'current channel', usage, models: [] },
        ],
      },
    }
  )
  const view = render(
    <I18nextProvider i18n={i18n}>
      <QueryClientProvider client={client}>
        <CustomerStatementView
          isAdmin
          userId={91}
          dimension='channel'
          period={{ start_timestamp: 1000, end_timestamp: 1200 }}
          onDimensionChange={vi.fn()}
        />
      </QueryClientProvider>
    </I18nextProvider>
  )
  return { view, client }
}

it('explains the actual quality reason and missing channel without inventing usage or hiding charges', async () => {
  const { view, client } = await renderQualityStatement({
    status: 'partial',
    unknown_billing_mode_requests: 370,
  })
  expect(screen.getByText('Unknown billing mode: 370 records')).toBeTruthy()
  expect(screen.queryByText(/Refresh before exporting/)).toBeNull()
  expect(screen.queryByText(/Historical prices unavailable:/)).toBeNull()
  expect(
    screen.getByText('Record unavailable; historical charges are retained.')
  ).toBeTruthy()
  expect(screen.getByText('Channel #97')).toBeTruthy()
  expect(screen.queryByText('#97')).toBeNull()
  expect(screen.getByText('current channel')).toBeTruthy()
  expect(screen.getByText('$1,663.551872')).toBeTruthy()
  fireEvent.click(screen.getAllByRole('button', { name: 'Expand models' })[0])
  expect(screen.queryByText('Billable 0 · Refunded 0')).toBeNull()
  const url = new URL(
    screen.getByRole('button', { name: 'View details' }).getAttribute('href') ??
      '',
    'http://localhost'
  )
  expect(url.searchParams.get('channel')).toBe('97')
  expect(url.searchParams.get('billingMode')).toBe('unknown')
  view.unmount()
  client.clear()
})

it('renders each reported quality cause and missing channel explanation in Chinese', async () => {
  const { view, client } = await renderQualityStatement(
    {
      status: 'partial',
      unknown_billing_mode_requests: 370,
      unavailable_requests: 2,
      cache_write_unavailable_requests: 3,
      missing_historical_price_rows: 4,
      provider_model_fallback_rows: 5,
    },
    'zh'
  )
  for (const message of [
    '计费方式暂未识别：370 条',
    '用量信息不可读取：2 条',
    '缓存写入用量不可用：3 条',
    '无法完成原价与优惠估算：4 条',
    '上游模型身份未记录：5 条',
    '渠道 #97',
    '记录已不存在，历史费用仍保留。',
    '共 12313 次请求',
  ]) {
    expect(screen.getByText(message)).toBeTruthy()
  }
  expect(screen.queryByText(/请刷新后再导出/)).toBeNull()
  view.unmount()
  client.clear()
})

it('preserves the negative sign of a refund-only model and its group', async () => {
  const i18n = createInstance().use(initReactI18next)
  await i18n.init({ lng: 'en', resources: { en: { translation: {} } } })
  const client = new QueryClient()
  const usage = {
    requests: 0,
    billable_calls: 0,
    refunded_calls: 11,
    input_tokens: 0,
    output_tokens: 0,
    cache_read_tokens: 0,
    cache_write_tokens: 0,
    gross_quota: 0,
    refund_quota: 8242035,
    net_quota: -8242035,
  }
  client.setQueryData(
    ['billing-customer-reconciliation', true, 91, 'channel', 1000, 1200, null],
    {
      result: {
        user_id: 91,
        username: 'customer',
        current_balance: 0,
        summary: usage,
        groups: [
          {
            id: 76,
            name: 'channel',
            usage,
            original_quota: -9473603,
            models: [
              {
                model_name: 'refunded-model',
                billing_mode: 'per_call',
                usage,
                original_quota: -9473603,
                detail_filter: {
                  start_timestamp: 1000,
                  end_timestamp: 1200,
                  channel_id: 76,
                },
              },
            ],
          },
        ],
      },
    }
  )
  const view = render(
    <I18nextProvider i18n={i18n}>
      <QueryClientProvider client={client}>
        <CustomerStatementView
          isAdmin
          userId={91}
          dimension='channel'
          period={{ start_timestamp: 1000, end_timestamp: 1200 }}
          onDimensionChange={vi.fn()}
        />
      </QueryClientProvider>
    </I18nextProvider>
  )
  fireEvent.click(screen.getByRole('button', { name: 'Expand models' }))
  expect(screen.getAllByText('-$16.48407')).toHaveLength(3)
  expect(screen.getAllByText('-$18.947206')).toHaveLength(2)
  view.unmount()
  client.clear()
})

it('shows an unavailable balance for a deleted customer while keeping historical totals', async () => {
  const { view, client } = await renderQualityStatement(
    { status: 'complete' },
    'en',
    null
  )
  expect(
    screen.getByText(
      'Customer record unavailable; current balance cannot be read.'
    )
  ).toBeTruthy()
  expect(screen.queryByText('Read directly from the main database')).toBeNull()
  expect(screen.getByText('$1,663.551872')).toBeTruthy()
  const balanceCard = screen.getByText('Current balance').parentElement
  if (!balanceCard) throw new Error('Missing balance card')
  expect(balanceCard.textContent).toBe('Current balance-')
  view.unmount()
  client.clear()
})

it('shows duration billing without token or per-call counters and preserves the detail filter', async () => {
  const { view, client } = await renderQualityStatement(
    { status: 'complete' },
    'en',
    0,
    'per_second'
  )
  fireEvent.click(screen.getAllByRole('button', { name: 'Expand models' })[0])
  expect(screen.getByText('Duration billing')).toBeTruthy()
  expect(screen.queryByText('Unknown billing mode')).toBeNull()
  expect(screen.queryByText(/Billable 0/)).toBeNull()
  expect(screen.queryByText(/Output 0/)).toBeNull()
  const detail = new URL(
    screen.getByRole('button', { name: 'View details' }).getAttribute('href') ??
      '',
    'http://localhost'
  )
  expect(detail.searchParams.get('billingMode')).toBe('per_second')
  view.unmount()
  client.clear()
})
