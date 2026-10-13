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
import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import {
  flexRender,
  getCoreRowModel,
  useReactTable,
} from '@tanstack/react-table'
import { render, screen, within } from '@testing-library/react'
import { createInstance } from 'i18next'
import { I18nextProvider } from 'react-i18next'
import { expect, test, vi } from 'vitest'

import en from '@/i18n/locales/en.json'
import { formatLogQuota } from '@/lib/format'

import { usageLogSchema, type UsageLog } from '../../data/schema'
import { useCommonLogsColumns } from '../columns/common-logs-columns'
import { CommonLogMobileCard } from '../common-log-mobile-card'
import { DetailsDialog } from '../dialogs/details-dialog'
import { UsageLogsProvider } from '../usage-logs-provider'

vi.mock('@lobehub/icons', () => ({}))

function CostViews(props: { log: UsageLog }) {
  const table = useReactTable({
    data: [props.log],
    columns: useCommonLogsColumns(false, false),
    getCoreRowModel: getCoreRowModel(),
  })
  const cells = table.getRowModel().rows[0].getAllCells()
  return (
    <>
      <section aria-label='Desktop log'>
        {cells
          .filter((cell) => ['quota', 'content'].includes(cell.column.id))
          .map((cell) => (
            <div key={cell.id}>
              {flexRender(cell.column.columnDef.cell, cell.getContext())}
            </div>
          ))}
      </section>
      <section aria-label='Mobile log'>
        <CommonLogMobileCard
          log={props.log}
          cells={new Map(cells.map((cell) => [cell.column.id, cell]))}
        />
      </section>
      <DetailsDialog
        log={props.log}
        isAdmin={false}
        isRoot={false}
        open
        onOpenChange={() => undefined}
      />
    </>
  )
}

test.each([
  [6, 50, 50, 'Task settlement refund', 'Refund this event'],
  [2, 50, 150, null, 'Additional charge this event'],
])(
  'type %s exposes the same committed task costs in desktop, mobile and details',
  async (type, quota, actual, description, changeLabel) => {
    const log = usageLogSchema.parse({
      id: 1,
      user_id: 1,
      created_at: 1000,
      content: '',
      model_name: 'customer-video',
      type,
      quota,
      other: JSON.stringify({
        task_id: 'task-cost',
        task_billing_event: 'adjustment',
        pre_consumed_quota: 100,
        actual_quota: actual,
      }),
    })
    const i18n = createInstance()
    await i18n.init({ lng: 'en', resources: { en } })
    const queryClient = new QueryClient({
      defaultOptions: { queries: { retry: false } },
    })
    render(
      <I18nextProvider i18n={i18n}>
        <QueryClientProvider client={queryClient}>
          <UsageLogsProvider>
            <CostViews log={log} />
          </UsageLogsProvider>
        </QueryClientProvider>
      </I18nextProvider>
    )
    const desktop = screen.getByRole('region', {
      name: 'Desktop log',
      hidden: true,
    })
    const mobile = screen.getByRole('region', {
      name: 'Mobile log',
      hidden: true,
    })
    const details = await screen.findByRole('dialog')
    for (const view of [desktop, mobile, details]) {
      const summary = within(view).getByRole('list', {
        name: 'Task cost breakdown',
        hidden: true,
      })
      const rows = within(summary).getAllByRole('listitem', { hidden: true })
      expect(rows[0]).toHaveTextContent(formatLogQuota(100))
      expect(rows[1]).toHaveTextContent(changeLabel)
      expect(rows[2]).toHaveTextContent(`Final cost${formatLogQuota(actual)}`)
      for (const row of rows) expect(row).toHaveClass('flex-wrap', 'min-w-0')
    }
    if (description) expect(desktop).toHaveTextContent(description)
    expect(log.quota).toBe(50)
    queryClient.clear()
  }
)
