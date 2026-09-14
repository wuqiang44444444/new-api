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
import { fireEvent, render, screen } from '@testing-library/react'
import { expect, test } from 'vitest'

import type { TaskLog } from '../../types'
import { useTaskLogsColumns } from '../columns/task-logs-columns'

function TaskRow(props: { log: TaskLog }) {
  const table = useReactTable({
    data: [props.log],
    columns: useTaskLogsColumns(false, false),
    getCoreRowModel: getCoreRowModel(),
  })
  return table
    .getRowModel()
    .rows[0].getAllCells()
    .filter((cell) => ['status', 'fail_reason'].includes(cell.column.id))
    .map((cell) => (
      <div key={cell.id}>
        {flexRender(cell.column.columnDef.cell, cell.getContext())}
      </div>
    ))
}

test('shows reconciliation as a warning and removes active diagnostics after recovery', () => {
  const client = new QueryClient({
    defaultOptions: { queries: { retry: false } },
  })
  const reason = 'Observation could not be verified'
  const log: TaskLog = {
    id: 1,
    user_id: 1,
    platform: 'seedance',
    task_id: 'task-fixture',
    action: 'text_to_video',
    channel_id: 1,
    group: 'default',
    quota: 100,
    submit_time: 1,
    status: 'RECONCILIATION_REQUIRED',
    fail_reason: reason,
  }
  const view = (task: TaskLog) => (
    <QueryClientProvider client={client}>
      <TaskRow log={task} />
    </QueryClientProvider>
  )
  const { rerender, unmount } = render(view(log))
  expect(screen.getByText('Status pending verification')).toBeVisible()
  expect(screen.getByText(reason)).not.toHaveClass('text-red-600')
  fireEvent.click(screen.getByRole('button', { name: 'View details' }))
  expect(screen.getByText('Verification reason')).toBeVisible()
  expect(screen.queryByText('Fail Reason')).not.toBeInTheDocument()
  const explanation =
    'The latest result is not yet confirmed. We will keep checking. Please do not submit again.'
  expect(screen.getByText(explanation)).toBeVisible()

  rerender(view({ ...log, status: 'SUCCESS', fail_reason: '' }))
  fireEvent.click(screen.getByRole('button', { name: 'View details' }))
  expect(screen.queryByText('Verification reason')).not.toBeInTheDocument()
  expect(screen.queryByText(explanation)).not.toBeInTheDocument()
  expect(screen.queryByText(reason)).not.toBeInTheDocument()

  rerender(view({ ...log, status: 'FAILURE' }))
  fireEvent.click(screen.getByRole('button', { name: 'View details' }))
  expect(screen.getByText('Fail Reason')).toBeVisible()
  expect(screen.queryByText(explanation)).not.toBeInTheDocument()
  unmount()
  client.clear()
})
