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
  admin: boolean
): Promise<GetLogsResponse> {
  const { billing_statement: _, ...query } = params
  const res = await api.get(
    `${billingStatementPath(admin)}?${buildQueryParams(query)}`
  )
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
