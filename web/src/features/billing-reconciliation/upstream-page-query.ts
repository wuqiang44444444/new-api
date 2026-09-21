import { useQuery, useQueryClient } from '@tanstack/react-query'
import { useTranslation } from 'react-i18next'

import { getAdminUpstreamReconciliation } from './api'
import { upstreamDiscountInitializationOptions } from './upstream-discount-init'

export function useUpstreamPage(
  params: Parameters<typeof getAdminUpstreamReconciliation>[0],
  enabled = true
) {
  const { t } = useTranslation()
  const queryClient = useQueryClient()
  return useQuery({
    queryKey: [
      'billing-upstream-reconciliation',
      params.start_timestamp,
      params.end_timestamp,
      params,
    ],
    queryFn: async ({ signal }) => {
      if (!params.level || params.level === 'groups') {
        await queryClient.fetchQuery(
          upstreamDiscountInitializationOptions(params)
        )
      }
      const response = await getAdminUpstreamReconciliation(params, signal)
      if (!response.success || !response.data) {
        throw new Error(
          response.message || t('Unable to load upstream reconciliation.')
        )
      }
      return response.data
    },
    enabled,
    staleTime: 30_000,
    retry: false,
  })
}

// Manual initialization reads channel identities across every page of this URL.
export async function collectUpstreamChannelPages(params: {
  start_timestamp: number
  end_timestamp: number
  url_key: string
}) {
  const channels = [] as import('./types').ProviderUrlChannelGroupSummary[]
  for (let page = 1; ; page++) {
    const response = await getAdminUpstreamReconciliation({
      ...params,
      level: 'channels',
      page,
      page_size: 100,
    })
    if (!response.success || !response.data) {
      throw new Error(
        response.message || 'Unable to load upstream reconciliation.'
      )
    }
    const result = response.data.result
    channels.push(...result.channels)
    if (page * result.page_size >= result.total) return channels
  }
}
