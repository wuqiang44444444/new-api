import { queryOptions, useQuery, useQueryClient } from '@tanstack/react-query'
import { useEffect } from 'react'

import { postAdminUpstreamDiscountInit } from './api'

type Period = { start_timestamp: number; end_timestamp: number }

export function upstreamDiscountInitializationOptions(period: Period) {
  return queryOptions({
    queryKey: [
      'billing-upstream-initialize',
      period.start_timestamp,
      period.end_timestamp,
    ],
    queryFn: async () => {
      const response = await postAdminUpstreamDiscountInit({
        period_start: period.start_timestamp,
        end_timestamp: period.end_timestamp + 1,
      })
      if (!response.success) throw new Error(response.message)
      return response.data?.counts ?? {}
    },
    staleTime: 30_000,
    retry: false,
  })
}

// Initialize the whole month before rendering any amounts. Membership is
// resolved server-side in batches, independently of which page is visible.
export function useInitializeUpstreamDiscounts(period: Period) {
  const queryClient = useQueryClient()
  const query = useQuery(upstreamDiscountInitializationOptions(period))
  useEffect(() => {
    if (query.data?.created || query.data?.defaulted) {
      // Do not cancel/restart a groups query that is already waiting for this
      // initialization. It will read the newly initialized values itself.
      void queryClient.invalidateQueries({
        queryKey: [
          'billing-upstream-reconciliation',
          period.start_timestamp,
          period.end_timestamp,
        ],
        refetchType: 'none',
      })
    }
  }, [
    query.data,
    query.dataUpdatedAt,
    queryClient,
    period.start_timestamp,
    period.end_timestamp,
  ])
  return query
}
