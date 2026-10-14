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
  cleanup,
  render,
  screen,
  waitFor,
  within,
} from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import i18next from 'i18next'
import { afterEach, beforeEach, expect, test, vi } from 'vitest'

import zh from '@/i18n/locales/zh.json'
import { api } from '@/lib/api'
import { useAuthStore } from '@/stores/auth-store'

import { EvidenceCleanup } from '../evidence-cleanup'

const clients: QueryClient[] = []
const task = {
  id: 1,
  task_id: 'cleanup-1',
  type: 'task_request_evidence_retention',
  status: 'succeeded',
  created_at: 100,
  updated_at: 200,
  payload: { target_timestamp: 100, trigger: 'manual' },
  result: {
    scanned: 3,
    cleaned_indexes: 2,
    deleted_files: 4,
    freed_bytes: 202,
    missing_files: 1,
    failed_items: 1,
    unattributed_files: 0,
  },
}

function renderCleanup() {
  const client = new QueryClient({
    defaultOptions: { queries: { retry: false }, mutations: { retry: false } },
  })
  clients.push(client)
  render(
    <QueryClientProvider client={client}>
      <EvidenceCleanup />
    </QueryClientProvider>
  )
}

beforeEach(() => {
  useAuthStore.getState().auth.setUser({ id: 21, username: 'root', role: 100 })
  vi.spyOn(api, 'get').mockResolvedValue({
    data: { success: true, data: null, total: 0 },
  })
})

afterEach(async () => {
  cleanup()
  clients.splice(0).forEach((client) => client.clear())
  useAuthStore.getState().auth.setUser(null)
  vi.restoreAllMocks()
  vi.useRealTimers()
  await i18next.changeLanguage('en')
})

test('non-Root cannot see or query evidence cleanup', () => {
  useAuthStore.getState().auth.setUser({ id: 22, username: 'admin', role: 10 })
  renderCleanup()
  expect(
    screen.queryByRole('button', { name: 'Clean evidence' })
  ).not.toBeInTheDocument()
  expect(api.get).not.toHaveBeenCalled()
})

test('Root confirms all-business irreversible scope and submits the selected Unix cutoff', async () => {
  const user = userEvent.setup()
  vi.spyOn(api, 'post').mockResolvedValue({
    data: { success: true, data: task },
  })
  renderCleanup()
  const button = await screen.findByRole('button', { name: 'Clean evidence' })
  await waitFor(() => expect(button).toBeEnabled())
  await user.click(screen.getByRole('button', { name: '24 hours ago' }))
  await user.click(button)
  const dialog = await screen.findByRole('alertdialog')
  expect(
    within(dialog).getByText(
      /This includes historical evidence of all types and cannot be undone/
    )
  ).toBeVisible()
  expect(dialog.textContent).toContain('time zone:')
  await user.click(
    within(dialog).getByRole('button', { name: 'Delete evidence' })
  )
  await waitFor(() =>
    expect(api.post).toHaveBeenCalledWith('/api/system-task/evidence-cleanup', {
      target_timestamp: expect.any(Number),
    })
  )
  const timestamp = vi.mocked(api.post).mock.calls[0][1] as {
    target_timestamp: number
  }
  expect(timestamp.target_timestamp).toBeGreaterThan(
    Math.floor(Date.now() / 1000) - 86410
  )
  expect(timestamp.target_timestamp).toBeLessThanOrEqual(
    Math.floor(Date.now() / 1000) - 86400
  )
  expect(
    await within(
      screen.getByRole('group', { name: 'Bytes removed' })
    ).findByText('202')
  ).toBeVisible()
})

test('completed results are restored from history after reopening the page', async () => {
  vi.mocked(api.get).mockImplementation(async (url, config) => ({
    data: {
      success: true,
      data:
        url === '/api/system-task/list' &&
        config?.params?.type === 'task_request_evidence_retention'
          ? [task]
          : null,
      total: 1,
    },
  }))
  renderCleanup()
  expect(
    await within(
      await screen.findByRole('group', { name: 'Files removed' })
    ).findByText('4')
  ).toBeVisible()
  expect(
    within(screen.getByRole('group', { name: 'Bytes removed' })).getByText(
      '202'
    )
  ).toBeVisible()
})

test('a 409 follows the actual running scope and disables repeated cleanup', async () => {
  const user = userEvent.setup()
  vi.spyOn(api, 'post').mockRejectedValue({
    isAxiosError: true,
    response: {
      status: 409,
      data: {
        success: false,
        data: {
          ...task,
          task_id: 'already-running',
          status: 'running',
          result: undefined,
          state: task.result,
        },
      },
    },
  })
  renderCleanup()
  const button = await screen.findByRole('button', { name: 'Clean evidence' })
  await waitFor(() => expect(button).toBeEnabled())
  await user.click(button)
  await user.click(
    within(await screen.findByRole('alertdialog')).getByRole('button', {
      name: 'Delete evidence',
    })
  )
  expect(
    await screen.findByRole('button', { name: 'Cleaning...' })
  ).toBeDisabled()
  expect(
    screen.getByRole('progressbar', { name: 'Evidence cleanup progress' })
  ).toBeInTheDocument()
  expect(api.post).toHaveBeenCalledTimes(1)
})

test('failed runs show real partial results and the sanitized failure explanation', async () => {
  vi.mocked(api.get).mockImplementation(async (url, config) => ({
    data: {
      success: true,
      data:
        url === '/api/system-task/list' &&
        config?.params?.type === 'task_request_evidence_retention'
          ? [
              {
                ...task,
                status: 'failed',
                result: { ...task.result, error_category: 'delete_failed' },
              },
            ]
          : null,
      total: 1,
    },
  }))
  renderCleanup()
  expect(
    await screen.findByText('Failed to delete evidence objects')
  ).toBeVisible()
  expect(
    within(screen.getByRole('group', { name: 'Failed items' })).getByText('1')
  ).toBeVisible()
  expect(
    within(screen.getByRole('group', { name: 'Bytes removed' })).getByText(
      '202'
    )
  ).toBeVisible()
})

test('load failure disables destructive action and offers retry', async () => {
  vi.mocked(api.get).mockRejectedValue(new Error('offline'))
  renderCleanup()
  expect(
    await screen.findByText('Failed to load request evidence cleanup')
  ).toBeVisible()
  expect(screen.getByRole('button', { name: 'Clean evidence' })).toBeDisabled()
})

test('health alerts display backlog and preserve unknown inode counts', async () => {
  vi.mocked(api.get).mockImplementation(async (_url, config) => ({
    data: {
      success: true,
      data:
        config?.params?.type === 'task_request_evidence_monitor'
          ? [
              {
                ...task,
                result: {
                  checked_at: 200,
                  pending_indexes: 5,
                  free_bytes: 1234,
                  alerts: ['cleanup_stalled', 'cleanup_backlog'],
                },
              },
            ]
          : null,
      total: 1,
    },
  }))
  renderCleanup()
  expect(
    await screen.findByText('Evidence cleanup has not succeeded for two hours.')
  ).toBeVisible()
  expect(
    within(screen.getByRole('group', { name: 'Pending indexes' })).getByText(
      '5'
    )
  ).toBeVisible()
  expect(
    within(screen.getByRole('group', { name: 'Free inodes' })).getByText(
      'Unavailable'
    )
  ).toBeVisible()
})

test('clearing the selected timestamp prevents submission', async () => {
  const user = userEvent.setup()
  const post = vi.spyOn(api, 'post')
  renderCleanup()
  await user.click(screen.getByRole('button', { name: 'Clear' }))
  expect(
    screen.getByText('Select a valid timestamp not later than now.')
  ).toBeVisible()
  expect(screen.getByRole('button', { name: 'Clean evidence' })).toBeDisabled()
  expect(post).not.toHaveBeenCalled()
})

test('a future calendar date is rejected before confirmation', async () => {
  vi.useFakeTimers({ toFake: ['Date'] })
  vi.setSystemTime(new Date('2026-10-14T04:00:00Z'))
  const user = userEvent.setup()
  renderCleanup()
  await user.click(screen.getByRole('button', { name: '2026-10-07' }))
  await user.click(screen.getByRole('button', { name: /October 15th, 2026/ }))
  expect(
    screen.getByText('Select a valid timestamp not later than now.')
  ).toBeVisible()
  expect(screen.getByRole('button', { name: 'Clean evidence' })).toBeDisabled()
})

test('cleanup controls and health alerts support Simplified Chinese', async () => {
  i18next.addResourceBundle('zh', 'translation', zh.translation, true, true)
  await i18next.changeLanguage('zh')
  renderCleanup()
  expect(
    await screen.findByRole('heading', { name: '清理请求证据' })
  ).toBeVisible()
  expect(screen.getByRole('button', { name: '清理证据' })).toBeVisible()
})
