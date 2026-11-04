import type { QueryClient } from '@tanstack/react-query'

import type { getAdminUpstreamReconciliation } from '../../api'
import type {
  BillingDataQuality,
  ProviderUrlChannelGroupSummary,
} from '../../types'

export type ProviderUrlGroupSummary = {
  usage_only?: boolean
  known_original_amount?: number
  known_reference_amount?: number
  url_key: string
  display_name: string
  custom_name?: string
  base_url?: string
  unidentified?: boolean
  deleted?: boolean
  channel_ids: number[]
  channel_count: number
  model_count: number
  usage: ProviderUrlChannelGroupSummary['usage']
  channels: ProviderUrlChannelGroupSummary[]
  data_quality?: BillingDataQuality
  original_amount?: number
  reference_amount?: number
  reference_known: boolean
  discount_pending_channels: number
  estimate_reasons?: string[]
}

type Period = { start_timestamp: number; end_timestamp: number }
export type SummaryFixture = {
  result: {
    url_groups: ProviderUrlGroupSummary[]
    data_quality?: BillingDataQuality
  }
  generated_at: number
}
export function projectUpstreamPage(
  data: SummaryFixture,
  params: Parameters<typeof getAdminUpstreamReconciliation>[0]
) {
  const channels = data.result.url_groups
    .flatMap((group) => group.channels)
    .filter(
      (channel) =>
        (!params.channel_id || channel.channel_id === params.channel_id) &&
        (!params.search ||
          `${channel.channel_name} ${channel.channel_id} ${channel.upstream_name || ''}`
            .toLowerCase()
            .includes(params.search.toLowerCase()))
    )
  const models = channels
    .filter((channel) => channel.channel_id === params.channel_id)
    .flatMap((channel) => channel.models)
  const page = params.page ?? 1,
    page_size = params.page_size ?? 20
  const offset = (page - 1) * page_size
  const modelLevel = params.level === 'models'
  return {
    success: true,
    data: {
      generated_at: data.generated_at,
      result: {
        channels: modelLevel
          ? []
          : channels
              .slice(offset, offset + page_size)
              .map((channel) => ({ ...channel, models: [] })),
        models: modelLevel ? models.slice(offset, offset + page_size) : [],
        total: modelLevel ? models.length : channels.length,
        page,
        page_size,
        model_count: channels.reduce((n, c) => n + c.models.length, 0),
        data_quality: data.result.data_quality,
      },
    },
  }
}
export function seedUpstreamPages(
  client: QueryClient,
  period: Period,
  data: SummaryFixture
) {
  client.setQueryData(['upstream-test-source', period.start_timestamp], data)
  const queries: Parameters<typeof getAdminUpstreamReconciliation>[0][] = [
    { ...period, level: 'channels', search: '', page: 1, page_size: 10 },
    { ...period, level: 'channels', page: 1, page_size: 10 },
  ]
  for (const group of data.result.url_groups) {
    for (const channel of group.channels) {
      queries.push({
        ...period,
        level: 'models',
        channel_id: channel.channel_id,
        page: 1,
        page_size: 10,
      })
    }
  }
  for (const params of queries) {
    client.setQueryData(
      [
        'billing-upstream-reconciliation',
        period.start_timestamp,
        period.end_timestamp,
        params,
      ],
      projectUpstreamPage(data, params).data
    )
  }
}
