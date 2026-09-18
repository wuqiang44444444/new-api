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
import { useQuery } from '@tanstack/react-query'
import { useMemo } from 'react'

import { useStatus } from '@/hooks/use-status'
import { useAuthStore } from '@/stores/auth-store'

import { getPricing } from '../api'
import { buildAssetShareGroups } from '../lib/asset-share-groups'

/**
 * Pricing query 的唯一 key 构造器：缓存播种（测试）与失效必须使用同一
 * 形状，避免 key 演进时静默分叉。
 */
export function getPricingQueryKey(
  userId: number | undefined,
  contractId: number | 'batch' | null = null
): ['pricing', number | undefined, number | 'batch' | null] {
  return ['pricing', userId, contractId]
}

export function usePricingData(
  enabled = true,
  contractId: number | 'batch' | null = null
) {
  const { status } = useStatus()
  const userId = useAuthStore((state) => state.auth.user?.id)

  const { data, isLoading, error, refetch } = useQuery({
    queryKey: getPricingQueryKey(userId, contractId),
    queryFn: () => getPricing(contractId),
    staleTime: 5 * 60 * 1000,
    enabled,
  })

  // Ensure rates never reach zero to prevent division errors
  const priceRate = useMemo(
    () => Math.max((status?.price as number) ?? 1, 0.001),
    [status?.price]
  )
  const usdExchangeRate = useMemo(
    () => Math.max((status?.usd_exchange_rate as number) ?? priceRate, 0.001),
    [status?.usd_exchange_rate, priceRate]
  )

  const models = useMemo(() => {
    if (!data?.data || !data?.vendors) return []

    const vendorMap = new Map(data.vendors.map((v) => [v.id, v]))

    const hydratedModels = data.data.map((model) => {
      const vendor = model.vendor_id
        ? vendorMap.get(model.vendor_id)
        : undefined
      return {
        ...model,
        key: model.model_name,
        vendor_name: vendor?.name,
        vendor_icon: vendor?.icon,
        vendor_description: vendor?.description,
        group_ratio: model.group_ratio ?? data.group_ratio,
      }
    })
    const assetShareGroups = buildAssetShareGroups(hydratedModels)
    return hydratedModels.map((model) => {
      const scope = model.api?.assets?.reuse_scope?.trim()
      const assetShareGroup = scope ? assetShareGroups.get(scope) : undefined
      return assetShareGroup
        ? { ...model, asset_share_group: assetShareGroup }
        : model
    })
  }, [data])

  return {
    models,
    vendors: data?.vendors ?? [],
    groupRatio: data?.group_ratio ?? {},
    usableGroup: data?.usable_group ?? {},
    endpointMap: data?.supported_endpoint ?? {},
    autoGroups: data?.auto_groups ?? [],
    isLoading,
    error,
    refetch,
    priceRate,
    usdExchangeRate,
  }
}
