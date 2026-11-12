import { api } from '@/lib/api'

import type {
  GetLogsParams,
  GetLogsResponse,
  GetLogStatsParams,
  GetLogStatsResponse,
} from '../types'
import { buildQueryParams } from './query-params'

function billingStatementPath(admin: boolean) {
  return admin
    ? '/api/billing/admin/customer-logs'
    : '/api/billing/statement/self/logs'
}
export async function fetchBillingStatementLogs(
  params: GetLogsParams,
  admin: boolean,
  signal?: AbortSignal
): Promise<GetLogsResponse> {
  const { billing_statement: _, ...query } = params
  const url = `${billingStatementPath(admin)}?${buildQueryParams(query)}`
  // A replacement query must not inherit the previous query's aborted signal.
  const res = signal
    ? await api.get(url, { signal, disableDuplicate: true })
    : await api.get(url)
  return res.data
}
export async function fetchBillingStatementStats(
  params: GetLogStatsParams,
  admin: boolean
): Promise<GetLogStatsResponse> {
  const { billing_statement: _, ...query } = params
  const res = await api.get(
    `${billingStatementPath(admin)}/stat?${buildQueryParams(query)}`
  )
  return res.data
}
