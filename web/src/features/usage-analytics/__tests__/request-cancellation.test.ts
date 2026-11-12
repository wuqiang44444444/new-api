import { QueryClient, QueryObserver } from '@tanstack/react-query'
import type { AxiosAdapter } from 'axios'
import { expect, it } from 'vitest'

import { api } from '@/lib/api'

import {
  getAdminUsageCustomers,
  getAdminUsageCustomerSummary,
  getAdminUsageUpstreamSummary,
  getUsageSelfSummary,
} from '../api'

it.each([
  ['personal summary', getUsageSelfSummary],
  ['admin customer list', getAdminUsageCustomers],
  ['admin customer summary', getAdminUsageCustomerSummary],
  ['admin upstream summary', getAdminUsageUpstreamSummary],
] as const)(
  'loads the %s after an immediate remount cancels the first request',
  async (_name, getUsage) => {
    const params = { period: 'day', date: '2026-11-10', user_id: 7 } as const
    const report = { success: true, data: { period: params, result: {} } }
    const adapter: AxiosAdapter = async (config) => ({
      config,
      data: report,
      status: 200,
      statusText: 'OK',
      headers: {},
    })
    const originalAdapter = api.defaults.adapter
    api.defaults.adapter = adapter
    const client = new QueryClient()
    const observer = new QueryObserver(client, {
      queryKey: ['usage-remount', params],
      queryFn: async ({ signal }) =>
        (await getUsage(params, signal)).data.period,
      retry: false,
    })

    // Reproduce the unsubscribe and resubscribe sequence used by StrictMode.
    observer.subscribe(() => {})()
    const unsubscribe = observer.subscribe(() => {})
    try {
      // Join the request started by the second subscription.
      const result = await observer.refetch({
        cancelRefetch: false,
        throwOnError: true,
      })
      expect(result.data).toEqual(params)
    } finally {
      unsubscribe()
      client.clear()
      api.defaults.adapter = originalAdapter
    }
  }
)
