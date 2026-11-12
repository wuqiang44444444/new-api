import { useQuery } from '@tanstack/react-query'
import { useTranslation } from 'react-i18next'

import { getAdminUpstreamReconciliation } from './api'

export function useUpstreamPage(
  params: Parameters<typeof getAdminUpstreamReconciliation>[0]
) {
  const { t } = useTranslation()
  return useQuery({
    queryKey: [
      'billing-upstream-reconciliation',
      params.start_timestamp,
      params.end_timestamp,
      params,
    ],
    queryFn: async ({ signal }) => {
      const response = await getAdminUpstreamReconciliation(params, signal)
      if (!response.success || !response.data) {
        throw new Error(
          response.message || t('Unable to load upstream reconciliation.')
        )
      }
      return response.data
    },
    staleTime: 30_000,
    retry: false,
  })
}
