import { fireEvent, render, screen } from '@testing-library/react'
import { createInstance } from 'i18next'
import { I18nextProvider, initReactI18next } from 'react-i18next'
import { beforeAll, describe, expect, it, vi } from 'vitest'

import { AdminBillingReconciliation } from '../admin-billing-reconciliation'
import { MyBilling } from '../my-billing'

vi.mock('../components/customer-statement', () => ({
  CustomerStatementView: (props: { isAdmin: boolean }) => (
    <div data-testid={props.isAdmin ? 'admin-statement' : 'self-statement'} />
  ),
}))

vi.mock('../components/customer-statements-list', () => ({
  CustomerStatementsListView: () => <div data-testid='customer-list' />,
}))

vi.mock('../components/upstream-statement', () => ({
  UpstreamStatementView: () => <div data-testid='upstream-statement' />,
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
  return render(<I18nextProvider i18n={i18n}>{node}</I18nextProvider>)
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

  it('keeps upstream reconciliation on the admin page', () => {
    renderWithI18n(
      <AdminBillingReconciliation
        search={{ section: 'upstream' }}
        onSearchChange={vi.fn()}
      />
    )

    expect(screen.getAllByText('Upstream reconciliation')).toHaveLength(2)
    expect(screen.getByTestId('upstream-statement')).toBeTruthy()
    expect(screen.queryByLabelText('Billing month')).toBeNull()
  })
})
