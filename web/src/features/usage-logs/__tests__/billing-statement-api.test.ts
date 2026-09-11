import { afterEach, expect, it, vi } from 'vitest'

import { api } from '@/lib/api'

import { getAllLogs, getUserLogs, getLogStats, getUserLogStats } from '../api'
import { buildApiParams } from '../lib/utils'

vi.mock('@/lib/api', () => ({ api: { get: vi.fn() } }))
afterEach(() => vi.clearAllMocks())

it('keeps the same customer, zero key, model and billing mode for details and statistics', async () => {
  vi.mocked(api.get).mockResolvedValue({
    data: { success: true, data: { items: [], total: 0 } },
  })
  for (const admin of [false, true]) {
    const params = buildApiParams({
      page: 2,
      pageSize: 20,
      isAdmin: admin,
      searchParams: {
        billing: true,
        billingUserId: 91,
        billingMode: 'token',
        tokenId: 0,
        model: 'model',
        startTime: 1788192000000,
        endTime: 1790783999000,
      },
    })
    await (admin ? getAllLogs : getUserLogs)(params)
    await (admin ? getLogStats : getUserLogStats)(params)
    const base = admin
      ? '/api/billing/admin/customer-logs'
      : '/api/billing/statement/self/logs'
    const calls = vi.mocked(api.get).mock.calls.slice(-2)
    for (const [i, call] of calls.entries()) {
      const url = new URL(String(call[0]), 'http://localhost')
      expect(url.pathname).toBe(base + (i === 1 ? '/stat' : ''))
      expect(url.searchParams.get('token_id')).toBe('0')
      expect(url.searchParams.get('user_id')).toBe('91')
      expect(url.searchParams.get('billing_mode')).toBe('token')
      expect(url.searchParams.get('model_name')).toBe('model')
      expect(url.searchParams.get('start_timestamp')).toBe('1788192000')
      expect(url.searchParams.get('end_timestamp')).toBe('1790783999')
    }
  }
})

it('keeps ordinary logs on the native endpoint', async () => {
  vi.mocked(api.get).mockResolvedValue({ data: { success: true } })
  await getAllLogs({ token_id: 90 })
  expect(String(vi.mocked(api.get).mock.calls[0][0])).toContain('/api/log?')
  const params = buildApiParams({
    page: 1,
    pageSize: 20,
    isAdmin: true,
    searchParams: { tokenId: 0 },
  })
  expect(params.token_id).toBeUndefined()
})
