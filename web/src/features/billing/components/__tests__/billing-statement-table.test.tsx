import {
  createMemoryHistory,
  createRootRoute,
  createRouter,
  RouterProvider,
} from '@tanstack/react-router'
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
import { fireEvent, render, screen } from '@testing-library/react'
import { createInstance } from 'i18next'
import { I18nextProvider, initReactI18next } from 'react-i18next'
import { describe, expect, test } from 'vitest'

import type { BillingBreakdownItem, BillingStatementItem } from '../../types'
import { BillingStatementTable } from '../billing-statement-table'

const i18n = createInstance()
await i18n.use(initReactI18next).init({
  lng: 'en',
  resources: { en: { translation: {} } },
  returnNull: false,
})

const statementItems: BillingStatementItem[] = [
  {
    token_id: 1,
    token_name: 'customer-key',
    model_name: 'model-a',
    requests: 2,
    prompt_tokens: 1_000,
    completion_tokens: 200,
    total_tokens: 1_200,
    gross_quota: 320,
    refund_quota: 20,
    net_quota: 300,
    average_use_time_seconds: 2,
    stream_requests: 1,
    latest_request_timestamp: 20,
    data_quality: { saturated: true },
  },
]

const breakdownItems: BillingBreakdownItem[] = [
  {
    token_id: 1,
    token_name: 'customer-key',
    model_name: 'model-a',
    requests: 2,
    gross_quota: 320,
    unallocated_adjustment_quota: 50,
    cache: {
      hit_requests: 1,
      write_requests: 1,
      denominator_requests: 2,
      denominator_scope: 'all_settled_requests',
      read_tokens: 400,
      write_tokens: 200,
      write_tokens_5m: 150,
      write_tokens_1h: 50,
      hit_request_gross_quota: 120,
      hit_request_ratio: 0.5,
    },
  },
]

describe('billing statement table', () => {
  test('exposes disclosure state and reveals professional row details', async () => {
    const rootRoute = createRootRoute({
      component: () => (
        <BillingStatementTable
          items={statementItems}
          breakdownItems={breakdownItems}
          breakdownLoading={false}
          breakdownUnavailable={false}
          period={{ start_timestamp: 1, end_timestamp: 2 }}
          view='detail'
          loading={false}
          onViewChange={() => undefined}
        />
      ),
    })
    const router = createRouter({
      routeTree: rootRoute,
      history: createMemoryHistory({ initialEntries: ['/'] }),
    })
    await router.load()

    render(
      <I18nextProvider i18n={i18n}>
        <RouterProvider router={router} />
      </I18nextProvider>
    )

    const disclosure = screen.getAllByRole('button', { name: /Details/ })[0]
    expect(disclosure).toBeDefined()
    expect(disclosure?.getAttribute('aria-expanded')).toBe('false')
    expect(screen.getAllByText(/Gross:/).length).toBeGreaterThan(0)
    expect(screen.getAllByText(/Refund:/).length).toBeGreaterThan(0)
    expect(screen.getAllByText(/Net:/).length).toBeGreaterThan(0)

    if (!disclosure) throw new Error('missing details disclosure')
    fireEvent.click(disclosure)

    expect(disclosure.getAttribute('aria-expanded')).toBe('true')
    expect(screen.getAllByText('Usage composition').length).toBeGreaterThan(0)
    expect(screen.getAllByText('Gross Usage Cost').length).toBeGreaterThan(0)
    expect(
      screen.getAllByText('Unallocated Async Adjustments').length
    ).toBeGreaterThan(0)
    expect(screen.getAllByText('Reporting Limit Reached').length).toBeGreaterThan(
      0
    )
    expect(
      screen.getAllByText(
        'The cache hit denominator is all settled requests in this row.'
      ).length
    ).toBeGreaterThan(0)
    expect(
      screen.getAllByText(
        'Historical logs do not reliably separate ordinary input from cache read and write tokens.'
      ).length
    ).toBeGreaterThan(0)
    expect(
      screen.getAllByText(
        'Async adjustments are included in gross usage cost but are not assigned to cache, Context, or dynamic billing categories.'
      ).length
    ).toBeGreaterThan(0)
  })
})
