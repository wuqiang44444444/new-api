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
import { cleanup, render, screen, waitFor } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { createInstance } from 'i18next'
import { I18nextProvider } from 'react-i18next'
import { afterEach, expect, it, vi } from 'vitest'

import en from '@/i18n/locales/en.json'
import zh from '@/i18n/locales/zh.json'
import { api } from '@/lib/api'

import { VideoFundLogs } from '..'
import { VideoFundProgressPanel } from '../progress'

vi.mock('@/stores/auth-store', () => ({
  useAuthStore: (
    select: (value: { auth: { user: { role: number } } }) => unknown
  ) => select({ auth: { user: { role: 10 } } }),
}))
vi.mock('@/features/auth/secure-verification', () => ({
  useSecureVerification: () => ({
    requestVerification: vi.fn(),
    dialogProps: {},
  }),
  SecureVerificationDialog: () => null,
}))
afterEach(() => {
  cleanup()
  vi.restoreAllMocks()
})

const item = {
  kind: 'attempt',
  id: 1,
  task_id: 'video_held',
  request_id: 'attempt_held',
  user_id: 10,
  app_id: 1,
  channel_id: 2,
  model: 'video-model',
  source: 'wallet',
  business_status: 'unknown',
  fund_state: 'held',
  quota: 25,
  refunded_quota: 0,
  refund_amount_known: false,
  waived_quota: 0,
  created_at: 1758000000,
  deadline_at: 1758086400,
  refunded_at: 0,
  retry_at: 0,
  operator_id: 0,
  note: '',
  failure: '',
  delivery: 'unverified',
  version: 'a'.repeat(64),
  can_refund: true,
}

it('shows a video hold without an error event and restricts administrators to reading', async () => {
  vi.spyOn(api, 'get').mockImplementation(async (path) => ({
    data: {
      success: true,
      data: path === '/api/video-funds' ? { items: [item], total: 1 } : item,
    },
  }))
  const i18n = createInstance()
  await i18n.init({ lng: 'zh', resources: { en, zh } })
  render(
    <I18nextProvider i18n={i18n}>
      <QueryClientProvider
        client={
          new QueryClient({ defaultOptions: { queries: { retry: false } } })
        }
      >
        <VideoFundLogs />
      </QueryClientProvider>
    </I18nextProvider>
  )
  expect(await screen.findByText('未知')).toBeInTheDocument()
  await userEvent.click(
    await screen.findByRole('button', { name: 'video_held' })
  )
  await waitFor(() =>
    expect(screen.getByText('尚未确认客户收到结果')).toBeInTheDocument()
  )
  expect(
    screen.queryByRole('button', { name: '确认视频退款' })
  ).not.toBeInTheDocument()
})

it('does not show stale rows as successful data after a failed request', async () => {
  vi.spyOn(api, 'get').mockRejectedValue(new Error('database unavailable'))
  const i18n = createInstance()
  await i18n.init({ lng: 'en', resources: { en, zh } })
  render(
    <I18nextProvider i18n={i18n}>
      <QueryClientProvider
        client={
          new QueryClient({ defaultOptions: { queries: { retry: false } } })
        }
      >
        <VideoFundLogs />
      </QueryClientProvider>
    </I18nextProvider>
  )
  expect(await screen.findByRole('alert')).toHaveTextContent(
    'Failed to load video fund records'
  )
  expect(screen.queryByText('video_held')).not.toBeInTheDocument()
})

it('shows customer progress for a creation hold without a formal task', async () => {
  const get = vi
    .spyOn(api, 'get')
    .mockResolvedValue({
      data: { success: true, data: { items: [item], total: 1 } },
    })
  const i18n = createInstance()
  await i18n.init({ lng: 'en', resources: { en, zh } })
  render(
    <I18nextProvider i18n={i18n}>
      <QueryClientProvider
        client={
          new QueryClient({ defaultOptions: { queries: { retry: false } } })
        }
      >
        <VideoFundProgressPanel />
      </QueryClientProvider>
    </I18nextProvider>
  )
  expect(await screen.findByText('Funds held')).toBeInTheDocument()
  expect(get).toHaveBeenCalledWith('/api/video-funds/self', {
    params: { p: 1, task_id: '' },
  })
  expect(screen.getByText('Automatic refund deadline')).toBeInTheDocument()
  expect(
    screen.queryByRole('button', { name: 'Confirm video refund' })
  ).not.toBeInTheDocument()
})
