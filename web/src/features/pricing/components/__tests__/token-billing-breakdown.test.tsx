/*
Copyright (C) 2023-2026 QuantumNous

This program is free software: you can redistribute it and/or modify
it under the terms of the GNU Affero General Public License as
published by the Free Software Foundation, either version 3 of the
License, or (at your option) any later version.

This program is distributed in the hope that it will be useful,
but WITHOUT ANY WARRANTY; without even the implied warranty of
MERCHANTABILITY or FITNESS FOR A PARTICULAR PURPOSE. See the
GNU Affero General Public License for more details.

You should have received a copy of the GNU Affero General Public License
along with this program. If not, see <https://www.gnu.org/licenses/>.

For commercial licensing, please contact support@quantumnous.com
*/
import { cleanup, render, screen, within } from '@testing-library/react'
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'

import {
  DEFAULT_CURRENCY_CONFIG,
  useSystemConfigStore,
} from '@/stores/system-config-store'

import { billingDisplayFixture } from '../../__tests__/billing-display-fixtures'
import { DynamicPricingBreakdown } from '../dynamic-pricing-breakdown'

const i18n = vi.hoisted(() => ({ language: 'en' }))

vi.mock('react-i18next', () => ({
  useTranslation: () => ({ i18n, t: (key: string) => key }),
}))

export const expression = `tier("base", p * 1.5 + cr * 0.05 + c * 4.5) * (
  weekday("Asia/Shanghai") >= 1 &&
  weekday("Asia/Shanghai") <= 5 &&
  (
    (hour("Asia/Shanghai") >= 9 && hour("Asia/Shanghai") < 12) ||
    (hour("Asia/Shanghai") >= 14 && hour("Asia/Shanghai") < 18)
  )
  ? 2 : 1
) / 6.71`

beforeEach(() => {
  i18n.language = 'en'
  useSystemConfigStore.getState().setConfig({
    currency: { ...DEFAULT_CURRENCY_CONFIG, quotaDisplayType: 'USD' },
  })
})
afterEach(cleanup)

describe('complete token price display', () => {
  it('renders weekday pricing conditions in Simplified Chinese', () => {
    i18n.language = 'zhCN'
    render(
      <DynamicPricingBreakdown
        billingExpr={expression}
        billingDisplay={billingDisplayFixture(expression)}
      />
    )
    const rows = screen.getAllByRole('row')
    expect(rows).toHaveLength(3)
    const peak = within(rows[2])
    expect(peak.getByText(/星期一/)).toHaveTextContent('星期五')
    expect(peak.getByText(/09:00/)).toHaveTextContent('12:00')
    expect(peak.getByText(/14:00/)).toHaveTextContent('18:00')
    expect(peak.getByText(/星期一/)).toHaveTextContent('Asia/Shanghai')
  })

  it.each([6.71, 6.9])(
    'shows actual standard and adjusted prices using exchange rate %s',
    (exchange) => {
      const expr = expression.replace('6.71', String(exchange))
      render(
        <DynamicPricingBreakdown
          billingExpr={expr}
          billingDisplay={billingDisplayFixture(expr)}
        />
      )
      const rows = screen.getAllByRole('row')
      expect(rows).toHaveLength(3)
      const base = within(rows[1])
      const peak = within(rows[2])
      const usd = (value: number) =>
        `$${new Intl.NumberFormat('en', { maximumFractionDigits: value >= 1 ? 4 : 6 }).format(value)}`
      expect(base.getByText(usd(1.5 / exchange))).toBeVisible()
      expect(peak.getByText(usd(3 / exchange))).toBeVisible()
      expect(peak.getByText(usd(9 / exchange))).toBeVisible()
      expect(peak.getByText(usd(0.1 / exchange))).toBeVisible()
      expect(peak.getByText(/Monday/)).toHaveTextContent('Friday')
      expect(peak.getByText(/09:00/)).toHaveTextContent('12:00')
      expect(peak.getByText(/14:00/)).toHaveTextContent('18:00')
      expect(peak.getByText(/09:00/)).toHaveTextContent('Asia/Shanghai')
      expect(screen.queryByText('Matched')).not.toBeInTheDocument()
    }
  )

  it('applies the contract multiplier once to both effective price rows', () => {
    render(
      <DynamicPricingBreakdown
        billingExpr={expression}
        billingDisplay={billingDisplayFixture(expression)}
        priceMultiplier={0.87}
      />
    )
    const peak = within(screen.getAllByRole('row')[2])
    expect(peak.getByText('$0.388972')).toBeVisible()
  })

  it('keeps direct time-tier conditions and actual fixed request charges', () => {
    const expr =
      'hour("Asia/Shanghai") >= 9 ? tier("peak", p * 3) : tier("base", p * 1.5)'
    const view = render(
      <DynamicPricingBreakdown
        billingExpr={expr}
        billingDisplay={billingDisplayFixture(expr)}
      />
    )
    expect(
      within(screen.getAllByRole('row')[1]).getByText(/09:00/)
    ).toBeVisible()
    expect(
      within(screen.getAllByRole('row')[2]).getByText(/Not.*09:00/)
    ).toBeVisible()
    const fixed = 'tier("base", p * 2 + 10000) + 20000'
    view.rerender(
      <DynamicPricingBreakdown
        billingExpr={fixed}
        billingDisplay={billingDisplayFixture(fixed)}
      />
    )
    expect(screen.getByText('$0.03/request')).toBeVisible()
  })

  it('uses recorded matches without guessing from the base tier label', () => {
    const display = billingDisplayFixture(expression)
    if (!display?.rules?.[0]) throw new Error('Missing time-price fixture')
    const view = render(
      <DynamicPricingBreakdown
        billingExpr={expression}
        billingDisplay={display}
        matchedTierLabel='base'
      />
    )
    expect(screen.queryByText('Matched')).not.toBeInTheDocument()
    expect(
      screen.getByText('Historical condition results are unavailable.')
    ).toBeVisible()
    view.rerender(
      <DynamicPricingBreakdown
        billingExpr={expression}
        billingDisplay={display}
        matchedTierLabel='base'
        requestRules={[
          { cond: display.rules[0].text, multiplier: 2, matched: true },
        ]}
      />
    )
    expect(
      within(screen.getAllByRole('row')[2]).getByText('Matched')
    ).toBeVisible()
    expect(
      within(screen.getAllByRole('row')[1]).queryByText('Matched')
    ).not.toBeInTheDocument()
  })
})
