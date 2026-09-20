import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import { fireEvent, render, screen } from '@testing-library/react'
import { createInstance } from 'i18next'
import { I18nextProvider, initReactI18next } from 'react-i18next'
import { beforeAll, describe, expect, it, vi } from 'vitest'

import { AdminBillingReconciliation } from '../admin-billing-reconciliation'
import type { BillingSection } from '../types'
import { MyBilling } from '../my-billing'

vi.mock('../components/customer-statement', () => ({
  CustomerStatementView: (props: {
    isAdmin: boolean
    toolbar?: React.ReactNode
  }) => (
    <div data-testid={props.isAdmin ? 'admin-statement' : 'self-statement'}>
      {props.toolbar}
    </div>
  ),
}))

vi.mock('../components/customer-statements-list', () => ({
  CustomerStatementsListView: () => <div data-testid='customer-list' />,
}))

vi.mock('../components/upstream-reconciliation', () => ({
  UpstreamReconciliationView: () => (
    <div data-testid='upstream-reconciliation' />
  ),
}))

const i18n = createInstance().use(initReactI18next)

beforeAll(async () => {
  await i18n.init({
    lng: 'en',
    fallbackLng: 'en',
    resources: { en: { translation: {} } },
    interpolation: { escapeValue: false },
  })
})

function renderWithI18n(node: React.ReactNode) {
  const client = new QueryClient({
    defaultOptions: { queries: { retry: false }, mutations: { retry: false } },
  })
  return render(
    <QueryClientProvider client={client}>
      <I18nextProvider i18n={i18n}>{node}</I18nextProvider>
    </QueryClientProvider>
  )
}

describe('billing page split', () => {
  it('keeps the self-service page focused on the current user', () => {
    const onMonthChange = vi.fn()
    renderWithI18n(<MyBilling month='2026-08' onMonthChange={onMonthChange} />)

    expect(screen.getByText('My billing')).toBeTruthy()
    expect(screen.getByTestId('self-statement')).toBeTruthy()
    expect(screen.queryByRole('tablist')).toBeNull()
    expect(screen.queryByTestId('customer-list')).toBeNull()

    fireEvent.change(screen.getByLabelText('Billing month'), {
      target: { value: '2026-07' },
    })
    expect(onMonthChange).toHaveBeenCalledWith('2026-07')
  })

  it('keeps customer reconciliation controls on the admin page', () => {
    renderWithI18n(
      <AdminBillingReconciliation search={{}} onSearchChange={vi.fn()} />
    )

    expect(screen.getByText('Billing reconciliation')).toBeTruthy()
    expect(
      screen.getByRole('tablist', {
        name: 'Billing reconciliation sections',
      })
    ).toBeTruthy()
    expect(screen.getByTestId('customer-list')).toBeTruthy()
    expect(screen.queryByTestId('self-statement')).toBeNull()
  })

  it('keeps upstream reconciliation on the admin page with exactly two sections', () => {
    renderWithI18n(
      <AdminBillingReconciliation
        search={{ section: 'upstream' }}
        onSearchChange={vi.fn()}
      />
    )

    expect(screen.getAllByText('Upstream reconciliation')).toHaveLength(2)
    expect(screen.getByTestId('upstream-reconciliation')).toBeTruthy()
    expect(screen.queryByTestId('upstream-url-statement')).toBeNull()
    expect(screen.queryByRole('tab', { name: 'Upstream URL summary' })).toBeNull()
    expect(screen.queryByLabelText('Billing month')).toBeNull()
  })

  it('lands the legacy upstream_url section on the merged upstream view', () => {
    renderWithI18n(
      <AdminBillingReconciliation
        search={{ section: 'upstream_url' as BillingSection, month: '2026-08' }}
        onSearchChange={vi.fn()}
      />
    )

    expect(screen.getByTestId('upstream-reconciliation')).toBeTruthy()
    expect(screen.queryByTestId('customer-list')).toBeNull()
  })
})
