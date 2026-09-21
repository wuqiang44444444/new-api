import type { QueryClient } from '@tanstack/react-query'

import type { getAdminUpstreamReconciliation } from '../../api'
import type { BillingDataQuality, ProviderUrlGroupSummary } from '../../types'

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
  const groups = data.result.url_groups.filter(
    (group) =>
      (!params.url_key || group.url_key === params.url_key) &&
      (!params.search ||
        `${group.custom_name} ${group.url_key}`
          .toLowerCase()
          .includes(params.search.toLowerCase()))
  )
  const channels = groups.flatMap((group) => group.channels)
  const models = channels
    .filter((channel) => channel.channel_id === params.channel_id)
    .flatMap((channel) => channel.models)
  const page = params.page ?? 1
  const page_size = params.page_size ?? 20
  const level = params.level ?? 'groups'
  const offset = (page - 1) * page_size
  let total = groups.length
  if (level === 'channels') total = channels.length
  if (level === 'models') total = models.length
  return {
    success: true,
    data: {
      generated_at: data.generated_at,
      result: {
        url_groups:
          level === 'groups' || level === 'options'
            ? groups.slice(offset, offset + page_size).map((group) => ({
                ...group,
                channels: [],
                channel_ids: group.unidentified ? group.channel_ids : [],
              }))
            : [],
        channels:
          level === 'channels'
            ? channels
                .slice(offset, offset + page_size)
                .map((channel) => ({ ...channel, models: [] }))
            : [],
        models:
          level === 'models' ? models.slice(offset, offset + page_size) : [],
        total,
        page,
        page_size,
        model_count: groups.reduce((n, g) => n + g.model_count, 0),
        data_quality: params.url_key
          ? groups[0]?.data_quality
          : data.result.data_quality,
      },
    },
  }
}
export function seedUpstreamPages(
  client: QueryClient,
  period: Period,
  data: SummaryFixture
) {
  client.setQueryData(
    [
      'billing-upstream-initialize',
      period.start_timestamp,
      period.end_timestamp,
    ],
    {}
  )
  client.setQueryData(['upstream-test-source', period.start_timestamp], data)
  const queries: Parameters<typeof getAdminUpstreamReconciliation>[0][] = [
    { ...period, level: 'groups', page: 1, page_size: 10 },
    { ...period, level: 'options', search: '', page: 1, page_size: 20 },
  ]
  for (const group of data.result.url_groups) {
    queries.push({
      ...period,
      level: 'groups',
      url_key: group.url_key,
      page: 1,
      page_size: 10,
    })
    queries.push({
      ...period,
      level: 'channels',
      url_key: group.url_key,
      page: 1,
      page_size: 10,
    })
    for (const channel of group.channels) {
      queries.push({
        ...period,
        level: 'models',
        url_key: group.url_key,
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
