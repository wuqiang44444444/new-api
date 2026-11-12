import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import {
  fireEvent,
  render,
  screen,
  waitFor,
  within,
} from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { createInstance } from 'i18next'
import { I18nextProvider, initReactI18next } from 'react-i18next'
import { expect, it, vi } from 'vitest'

import zh from '@/i18n/locales/zh.json'

import { CustomerStatementsListView } from '../customer-statements-list'

const exportsApi = vi.hoisted(() => ({
  createCustomerStatementsExport: vi.fn(),
  listSelfExports: vi.fn(),
  requestExportDownload: vi.fn(),
  downloadExportFile: vi.fn(),
}))
vi.mock('../../export-api', async (importOriginal) => ({
  ...(await importOriginal<typeof import('../../export-api')>()),
  ...exportsApi,
}))

const reportVersion = 'aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa'

async function renderStatement(
  missingPrice = false,
  language = 'en',
  search = '',
  total = 1
) {
  const i18n = createInstance().use(initReactI18next)
  await i18n.init({
    lng: language,
    resources: { en: { translation: {} }, zh },
  })
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
      search,
      'all',
      'net_quota',
      'desc',
      1,
      20,
    ],
    {
      generated_at: 1789061550,
      billing_report: {
        version: reportVersion,
        published_days: 1,
        expected_days: 1,
        as_of: 1789061550,
      },
      result: {
        recorded_original_amounts: true,
        items: [item],
        summary: { ...item, customer_count: 1 },
        total,
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
          search={{ customerSearch: search }}
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

it('left-aligns the report status when it wraps below the customer controls', async () => {
  const view = await renderStatement()

  const report = screen.getByText(/1 of 1 days published/)
  const status = report.closest('div')?.parentElement

  expect(status?.parentElement).toHaveClass('flex-wrap', 'justify-between')
  expect(status).not.toHaveClass('lg:ms-auto')
  view.cleanup()
})

it('keeps filters, totals and pagination outside the customer table scroll area', async () => {
  const view = await renderStatement()
  const table = screen.getByRole('table')
  const scrollArea = table.parentElement

  expect(scrollArea).toHaveClass('overflow-auto', 'overscroll-contain')
  expect(
    within(table)
      .getByRole('columnheader', { name: 'Customer' })
      .closest('thead')
  ).toHaveClass('sticky', 'top-0')
  expect(scrollArea).not.toContainElement(
    screen.getByRole('textbox', { name: 'Customer search' })
  )
  expect(scrollArea).not.toContainElement(screen.getByText('Customers'))
  expect(scrollArea).not.toContainElement(
    screen.getByRole('group', { name: 'Customer billing' })
  )
  view.cleanup()
})

it('keeps the range and full paging controls together below the customer table', async () => {
  const view = await renderStatement(false, 'en', '', 34)
  const user = userEvent.setup()
  const range = screen.getByText('Showing 1–20 of 34')
  const footer = range.closest('[data-slot="card-footer"]')
  const controls = screen.getByRole('group', { name: 'Customer billing' })
  expect(footer).toContainElement(controls)
  // The query container cannot size a flex sibling by its content width.
  expect(controls.parentElement).toHaveClass(
    'sm:grid-cols-[auto_minmax(0,1fr)]'
  )
  const pager = within(controls)
  expect(pager.getByText('Page 1 of 2')).toBeVisible()
  expect(
    pager.getByRole('button', { name: 'Go to previous page' })
  ).toBeDisabled()
  await user.click(pager.getByRole('button', { name: 'Go to page 2' }))
  expect(view.onSearchChange).toHaveBeenLastCalledWith(
    { customerPage: 2 },
    true
  )
  await user.click(pager.getByRole('combobox', { name: 'Rows per page' }))
  await user.click(await screen.findByRole('option', { name: '50' }))
  expect(view.onSearchChange).toHaveBeenLastCalledWith({
    customerPageSize: 50,
    customerPage: 1,
  })
  view.cleanup()
})

it('separates settled prices from precharge flows for the randy statement', async () => {
  const view = await renderStatement()
  const user = userEvent.setup()
  const funds = screen.getByRole('button', { name: 'Fund movements' })
  expect(
    screen.queryByRole('columnheader', { name: 'Total charges' })
  ).toBeNull()
  await user.click(funds)
  expect(funds).toHaveAttribute('aria-pressed', 'true')
  const headers = screen
    .getAllByRole('columnheader')
    .map((cell) => cell.textContent)
  expect(headers.slice(2, 7)).toEqual([
    'List price',
    'Discount',
    'Net settled amount',
    'Total charges',
    'Total returns',
  ])
  const row = screen.getByRole('row', { name: /randy/ })
  const cells = within(row).getAllByRole('cell')
  expect(cells.slice(2, 7).map((cell) => cell.textContent)).toEqual([
    '$1,912.128588',
    '$248.576716',
    '$1,663.551872',
    '$3,655.811442',
    '$1,992.25957',
  ])
  await user.click(screen.getByRole('button', { name: 'Billing notes' }))
  expect(
    screen.getByText('List price minus savings equals net settled amount.', {
      exact: false,
    })
  ).toBeTruthy()
  expect(
    screen.getByText(
      'Total charges include precharges. Total returns include released precharges and refunds.'
    )
  ).toBeTruthy()
  await user.keyboard('{Escape}')
  expect(
    screen.queryByText('Billing mode needs identification · 370 billing events')
  ).toBeNull()
  await user.click(
    screen.getByRole('button', {
      name: /Billing mode needs identification 370 billing events/,
    })
  )
  expect(
    screen.getByText('Billing mode needs identification · 370 billing events')
  ).toBeTruthy()
  expect(screen.getByText(/Last activity/)).toBeTruthy()
  await user.keyboard('{Escape}')
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
  expect(view.onSelectUser).toHaveBeenCalledWith(91, reportVersion)
  view.cleanup()
})

it('uses Chinese labels that identify precharge flows and the missing billing-mode records', async () => {
  const view = await renderStatement(false, 'zh')
  const user = userEvent.setup()
  await user.click(screen.getByRole('button', { name: '资金流水' }))
  const headers = screen
    .getAllByRole('columnheader')
    .map((cell) => cell.textContent)
  expect(headers.slice(2, 7)).toEqual([
    '原价',
    '优惠',
    '净结算金额',
    '累计扣减',
    '累计退回',
  ])
  await user.click(screen.getByRole('button', { name: '账单说明' }))
  expect(
    screen.getByText('累计扣减包含预扣；累计退回包含预扣退差额及退款。')
  ).toBeTruthy()
  await user.keyboard('{Escape}')
  await user.click(
    screen.getByRole('button', { name: /计费方式待识别 370 条账务事件/ })
  )
  expect(screen.getByText('计费方式待识别 · 370 条账务事件')).toBeTruthy()
  expect(screen.queryByRole('columnheader', { name: '实付消费' })).toBeNull()
  view.cleanup()
})

it('keeps settled list price and savings unknown when the API lacks historical prices', async () => {
  const view = await renderStatement(true)
  const row = screen.getByRole('row', { name: /randy/ })
  const cells = within(row).getAllByRole('cell')
  expect(cells[2].textContent).toBe('-')
  expect(cells[3].textContent).toBe('-')
  expect(cells[4].textContent).toBe('$1,663.551872')
  view.cleanup()
})

it('exports every customer in the selected month even when the list is filtered', async () => {
  exportsApi.createCustomerStatementsExport.mockResolvedValue({
    job_id: 'all-statements',
    status: 'succeeded',
    artifact: { files: [{ file_name: 'customer-statements.csv' }] },
  })
  exportsApi.listSelfExports.mockResolvedValue({ items: [], total: 0 })
  exportsApi.requestExportDownload.mockResolvedValue({
    files: [{ url: 'https://example.test/statements.csv' }],
  })
  const view = await renderStatement(false, 'en', 'randy')
  try {
    await userEvent.click(
      screen.getByRole('button', { name: 'Export and download' })
    )
    const drawer = within(
      screen.getByRole('dialog', { name: 'Export and download' })
    )
    expect(drawer.getByText('All customers')).toBeVisible()
    await userEvent.click(drawer.getByRole('button', { name: 'Export CSV' }))
    expect(
      exportsApi.createCustomerStatementsExport
    ).toHaveBeenCalledExactlyOnceWith({
      start_timestamp: 1000,
      end_timestamp: 1201,
      report_version: reportVersion,
    })
    await waitFor(() =>
      expect(exportsApi.downloadExportFile).toHaveBeenCalledExactlyOnceWith(
        'https://example.test/statements.csv'
      )
    )
  } finally {
    view.cleanup()
  }
})
