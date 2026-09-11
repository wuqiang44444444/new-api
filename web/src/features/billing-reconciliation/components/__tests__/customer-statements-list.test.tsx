import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import { fireEvent, render, screen, within } from '@testing-library/react'
import { createInstance } from 'i18next'
import { I18nextProvider, initReactI18next } from 'react-i18next'
import { expect, it, vi } from 'vitest'

import zh from '@/i18n/locales/zh.json'

import { CustomerStatementsListView } from '../customer-statements-list'

async function renderStatement(missingPrice = false, language = 'en') {
  const i18n = createInstance().use(initReactI18next)
  await i18n.init({ lng: language, resources: { en: { translation: {} }, zh } })
  const client = new QueryClient({
    defaultOptions: { queries: { retry: false } },
  })
  const usage = {
    requests: 12313,
    billable_calls: 0,
    refunded_calls: 0,
    input_tokens: 0,
    output_tokens: 0,
    cache_read_tokens: 0,
    cache_write_tokens: 0,
    gross_quota: 1827905721,
    refund_quota: 996129785,
    net_quota: 831775936,
  }
  const item = {
    user_id: 91,
    username: 'randy',
    display_name: 'randy',
    usage,
    original_quota: missingPrice ? undefined : 956064294,
    discount_quota: missingPrice ? undefined : 124288358,
    data_quality: { status: 'partial', unknown_billing_mode_requests: 370 },
    last_activity_at: 1789038325,
  }
  client.setQueryData(
    [
      'billing-customer-statements',
      1000,
      1200,
      '',
      'all',
      'net_quota',
      'desc',
      1,
      20,
    ],
    {
      generated_at: 1789061550,
      result: {
        items: [item],
        summary: { ...item, customer_count: 1 },
        total: 1,
        page: 1,
        page_size: 20,
      },
    }
  )
  const onSelectUser = vi.fn()
  const onSearchChange = vi.fn()
  const view = render(
    <I18nextProvider i18n={i18n}>
      <QueryClientProvider client={client}>
        <CustomerStatementsListView
          period={{ start_timestamp: 1000, end_timestamp: 1200 }}
          search={{}}
          onSelectUser={onSelectUser}
          onSearchChange={onSearchChange}
        />
      </QueryClientProvider>
    </I18nextProvider>
  )
  return {
    onSelectUser,
    onSearchChange,
    cleanup: () => {
      view.unmount()
      client.clear()
    },
  }
}

it('separates settled prices from precharge flows for the randy statement', async () => {
  const view = await renderStatement()
  const headers = screen
    .getAllByRole('columnheader')
    .map((cell) => cell.textContent)
  expect(headers.slice(2, 7)).toEqual([
    'Settled list price',
    'Discount',
    'Net settled amount',
    'Total charges',
    'Total returns',
  ])
  const row = screen.getByRole('row', { name: /randy/ })
  const cells = within(row).getAllByRole('cell')
  expect(cells.slice(2, 7).map((cell) => cell.textContent)).toEqual([
    '$1,912.13',
    '$248.58',
    '$1,663.55',
    '$3,655.81',
    '$1,992.26',
  ])
  expect(
    screen.getByText('Settled list price − discounts = net settled amount.', {
      exact: false,
    })
  ).toBeTruthy()
  expect(
    screen.getByText(
      'Total charges include precharges. Total returns include released precharges and refunds.'
    )
  ).toBeTruthy()
  expect(screen.getByText('Unknown billing mode: 370 records')).toBeTruthy()
  expect(
    screen.queryByRole('columnheader', { name: 'Settled amount' })
  ).toBeNull()
  fireEvent.click(screen.getByRole('button', { name: 'Net settled amount' }))
  expect(view.onSearchChange).toHaveBeenCalledWith({
    customerSortBy: 'net_quota',
    customerSortOrder: 'asc',
    customerPage: 1,
  })
  fireEvent.click(screen.getByRole('button', { name: 'View statement' }))
  expect(view.onSelectUser).toHaveBeenCalledWith(91)
  view.cleanup()
})

it('uses Chinese labels that identify precharge flows and the missing billing-mode records', async () => {
  const view = await renderStatement(false, 'zh')
  const headers = screen
    .getAllByRole('columnheader')
    .map((cell) => cell.textContent)
  expect(headers.slice(2, 7)).toEqual([
    '结算原价',
    '优惠',
    '净结算金额',
    '累计扣减',
    '累计退回',
  ])
  expect(
    screen.getByText('累计扣减包含预扣；累计退回包含预扣退差额及退款。')
  ).toBeTruthy()
  expect(screen.getByText('计费方式未记录：370 条')).toBeTruthy()
  expect(screen.queryByRole('columnheader', { name: '实付消费' })).toBeNull()
  view.cleanup()
})

it('keeps settled list price and savings unknown when the API lacks historical prices', async () => {
  const view = await renderStatement(true)
  const row = screen.getByRole('row', { name: /randy/ })
  const cells = within(row).getAllByRole('cell')
  expect(cells[2].textContent).toBe('-')
  expect(cells[3].textContent).toBe('-')
  expect(cells[4].textContent).toBe('$1,663.55')
  view.cleanup()
})
