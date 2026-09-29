import { afterEach, beforeEach, expect, it, vi } from 'vitest'

import { api } from '@/lib/api'

import { fetchLogsByCategory } from '../lib/utils'

vi.mock('@/lib/api', () => ({ api: { get: vi.fn() } }))

beforeEach(() => {
  vi.useFakeTimers()
  vi.setSystemTime(new Date('2026-09-29T12:00:00Z'))
  vi.mocked(api.get).mockResolvedValue({ data: { success: true, data: [] } })
})
afterEach(() => {
  vi.useRealTimers()
  vi.clearAllMocks()
})

it.each(['task', 'drawing'] as const)(
  'keeps historical and cross-day %s tasks visible with an explicit zero start',
  async (logCategory) => {
    const endTime = new Date('2026-09-28T18:00:00Z').getTime()
    await fetchLogsByCategory({
      logCategory,
      isAdmin: true,
      page: 1,
      pageSize: 20,
      searchParams: { filter: 'historical-task', startTime: 0, endTime },
      columnFilters: [],
    })
    const requested = new URL(
      String(vi.mocked(api.get).mock.calls[0][0]),
      'https://example.test'
    )
    expect(requested.searchParams.get('start_timestamp')).toBe('0')
    expect(requested.searchParams.get('end_timestamp')).toBe(
      String(logCategory === 'drawing' ? endTime : endTime / 1000)
    )
    expect(
      requested.searchParams.get(
        logCategory === 'drawing' ? 'mj_id' : 'task_id'
      )
    ).toBe('historical-task')
  }
)

it('uses the default day only when neither time boundary is supplied', async () => {
  await fetchLogsByCategory({
    logCategory: 'task',
    isAdmin: true,
    page: 1,
    pageSize: 20,
    searchParams: {},
    columnFilters: [],
  })
  const requested = new URL(
    String(vi.mocked(api.get).mock.calls[0][0]),
    'https://example.test'
  )
  const today = new Date()
  today.setHours(0, 0, 0, 0)
  expect(requested.searchParams.get('start_timestamp')).toBe(
    String(today.getTime() / 1000)
  )
  expect(requested.searchParams.get('end_timestamp')).toBe(
    String(Date.now() / 1000 + 3600)
  )
})
