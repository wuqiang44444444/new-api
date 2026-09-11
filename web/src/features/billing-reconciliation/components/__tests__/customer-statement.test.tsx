import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import { fireEvent, render, screen } from '@testing-library/react'
import { createInstance } from 'i18next'
import { I18nextProvider, initReactI18next } from 'react-i18next'
import { expect, it, vi } from 'vitest'

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
    screen.getByText('Original amount minus net settled amount')
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
