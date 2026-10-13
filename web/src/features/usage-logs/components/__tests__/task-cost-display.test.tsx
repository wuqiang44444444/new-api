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
import { act, render, screen, within } from '@testing-library/react'
import { createInstance } from 'i18next'
import { I18nextProvider } from 'react-i18next'
import { expect, test } from 'vitest'

import en from '@/i18n/locales/en.json'
import zh from '@/i18n/locales/zh.json'
import { formatLogQuota } from '@/lib/format'

import type { LogOtherData } from '../../types'
import { LogCostDisplay } from '../log-cost-display'

async function showCost(
  logType: number,
  quota: number,
  other: LogOtherData,
  language = 'en'
) {
  const i18n = createInstance()
  await i18n.init({
    lng: language,
    fallbackLng: 'en',
    resources: { en, zh },
    interpolation: { escapeValue: false },
  })
  const view = render(
    <I18nextProvider i18n={i18n}>
      <LogCostDisplay {...{ logType, quota, other }} />
    </I18nextProvider>
  )
  return { ...view, i18n }
}

test('settlement refund shows the reservation, refund and saved final cost', async () => {
  const other = {
    task_id: 'task-cost',
    task_billing_event: 'adjustment',
    pre_consumed_quota: 938800,
    actual_quota: 300700,
  }
  await showCost(6, 638100, other)
  const summary = screen.getByRole('list', { name: 'Task cost breakdown' })
  const rows = within(summary).getAllByRole('listitem')
  expect(rows).toHaveLength(3)
  expect(rows[0]).toHaveTextContent('Pre-consumed cost')
  expect(rows[0]).toHaveTextContent(formatLogQuota(938800))
  expect(rows[1]).toHaveTextContent('Refund this event')
  expect(rows[1]).toHaveTextContent(`−${formatLogQuota(638100)}`)
  expect(rows[2]).toHaveTextContent('Final cost')
  expect(rows[2]).toHaveTextContent(formatLogQuota(300700))
})

test('settlement additional charge shows only the event debit and the saved final cost', async () => {
  await showCost(2, 50, {
    task_id: 'task-cost',
    task_billing_event: 'adjustment',
    pre_consumed_quota: 100,
    actual_quota: 150,
  })
  const rows = screen.getAllByRole('listitem')
  expect(rows[1]).toHaveTextContent('Additional charge this event')
  expect(rows[1]).toHaveTextContent(`+${formatLogQuota(50)}`)
  expect(rows[2]).toHaveTextContent(formatLogQuota(150))
})

test('failure refund preserves a recorded zero final cost', async () => {
  await showCost(6, 100, {
    task_id: 'task-cost',
    task_billing_event: 'refund',
    adjustment_before_quota: 100,
    actual_quota: 0,
  })
  const rows = screen.getAllByRole('listitem')
  expect(rows[0]).toHaveTextContent('Cost before adjustment')
  expect(rows[0]).toHaveTextContent(formatLogQuota(100))
  expect(rows[2]).toHaveTextContent(`Final cost${formatLogQuota(0)}`)
  expect(screen.queryByText('Not recorded')).not.toBeInTheDocument()
})

test('manual refund uses the saved net cost and does not rename the previous bill as a reservation', async () => {
  await showCost(6, 100, {
    task_id: 'task-cost',
    task_billing_event: 'customer_refund',
    adjustment_before_quota: 100,
    net_quota: 0,
  })
  expect(screen.getAllByRole('listitem')[2]).toHaveTextContent(
    `Cost after adjustment${formatLogQuota(0)}`
  )
  expect(screen.queryByText('Pre-consumed cost')).not.toBeInTheDocument()
  expect(screen.queryByText('Final cost')).not.toBeInTheDocument()
})

test.each([
  ['missing amounts', {}],
  ['contradictory amounts', { pre_consumed_quota: 100, actual_quota: 60 }],
  ['negative amounts', { pre_consumed_quota: -100, actual_quota: -50 }],
  [
    'unsafe amounts',
    { pre_consumed_quota: Number.MAX_SAFE_INTEGER + 1, actual_quota: Infinity },
  ],
])(
  'refund with %s keeps the event amount and leaves unproven amounts unrecorded',
  async (_name, amounts) => {
    await showCost(6, 50, {
      task_id: 'task-cost',
      task_billing_event: 'adjustment',
      ...amounts,
    })
    expect(screen.getAllByText('Not recorded')).toHaveLength(2)
    expect(screen.getAllByRole('listitem')[1]).toHaveTextContent(
      `−${formatLogQuota(50)}`
    )
  }
)

test('historical refund shows its recorded final cost without inventing a reservation', async () => {
  await showCost(6, 50, {
    task_id: 'task-cost',
    task_billing_event: 'adjustment',
    actual_quota: 60,
  })
  expect(screen.getAllByRole('listitem')[0]).toHaveTextContent('Not recorded')
  expect(screen.getAllByRole('listitem')[2]).toHaveTextContent(
    formatLogQuota(60)
  )
})

test('subscription adjustment remains identified as converted quota rather than wallet money', async () => {
  await showCost(6, 50, {
    task_id: 'task-cost',
    task_billing_event: 'adjustment',
    pre_consumed_quota: 100,
    actual_quota: 50,
    billing_source: 'subscription',
  })
  expect(screen.getByText('Subscription')).toBeInTheDocument()
  expect(
    screen.getByText('Amounts are converted subscription quota.')
  ).toBeInTheDocument()
  expect(screen.getAllByRole('listitem')[2]).toHaveTextContent(
    formatLogQuota(50)
  )
})

test.each([
  ['ordinary consumption', 2, {}],
  ['task creation', 2, { task_id: 'task-cost', task_billing_event: 'create' }],
  ['non-task refund', 6, {}],
])('%s keeps its original amount display', async (_name, type, other) => {
  const view = await showCost(type, 50, other)
  expect(screen.queryByRole('list')).not.toBeInTheDocument()
  expect(view.container).toHaveTextContent(formatLogQuota(50))
})

test('cost labels update when switching from English to Chinese', async () => {
  const view = await showCost(6, 50, {
    task_id: 'task-cost',
    task_billing_event: 'adjustment',
    pre_consumed_quota: 100,
    actual_quota: 50,
  })
  await act(() => view.i18n.changeLanguage('zh'))
  expect(screen.getByRole('list', { name: '任务费用明细' })).toBeInTheDocument()
  expect(screen.getByText('预扣费用')).toBeInTheDocument()
  expect(screen.getByText('本次退款')).toBeInTheDocument()
  expect(screen.getByText('最终费用')).toBeInTheDocument()
})
