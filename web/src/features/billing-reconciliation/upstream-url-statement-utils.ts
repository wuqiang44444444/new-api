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
import type { TFunction } from 'i18next'

import { billingModeLabel } from './lib'
import type {
  ProviderUrlChannelSummary,
  ProviderUrlGroupSummary,
  ProviderUrlModelSummary,
} from './types'
import {
  billingDataQualityLabel,
  escapeCsvCell,
  upstreamModelLabel,
} from './upstream-statement-utils'

export function upstreamUrlGroupLabel(
  group: ProviderUrlGroupSummary,
  t: TFunction
): string {
  if (!group.unidentified) return group.base_url ?? group.url_key
  if (group.deleted) {
    return `${t('Deleted channel')} · #${group.channel_ids.join(', #')}`
  }
  return group.display_name
}

export type UpstreamUrlStatementCsvOptions = {
  groups: ProviderUrlGroupSummary[]
  generatedAt: number
  month: string
  t: TFunction
}

// CSV rows are the per-channel leaf rows only, so parent URL and model totals
// are never double-counted next to their children.
export function buildUpstreamUrlStatementCsv(
  options: UpstreamUrlStatementCsvOptions
) {
  const headers = [
    options.t('Billing month'),
    options.t('Upstream base URL'),
    options.t('URL grouping'),
    options.t('Upstream channel'),
    'channel_id',
    options.t('Model'),
    options.t('Billing mode'),
    options.t('Input tokens'),
    options.t('Cache read tokens'),
    options.t('Cache write tokens'),
    options.t('Output tokens'),
    options.t('Requests'),
    options.t('Billable calls'),
    options.t('Data status'),
    'generated_at',
  ]
  const lines = [headers.map(escapeCsvCell).join(',')]
  for (const group of options.groups) {
    for (const model of group.models) {
      for (const channel of model.channels) {
        lines.push(
          upstreamUrlStatementCsvRow(options, group, model, channel)
            .map(escapeCsvCell)
            .join(',')
        )
      }
    }
  }
  return lines.join('\n')
}

function upstreamUrlStatementCsvRow(
  options: UpstreamUrlStatementCsvOptions,
  group: ProviderUrlGroupSummary,
  model: ProviderUrlModelSummary,
  channel: ProviderUrlChannelSummary
): Array<string | number> {
  const grouping = group.unidentified
    ? options.t('Unidentified URL — kept per channel')
    : options.t('Current channel base URL')
  return [
    options.month,
    group.unidentified ? '' : (group.base_url ?? group.url_key),
    grouping,
    channel.channel_name,
    channel.channel_id,
    upstreamModelLabel(channel),
    options.t(billingModeLabel(model.billing_mode)),
    channel.usage.input_tokens,
    channel.usage.cache_read_tokens,
    channel.data_quality?.cache_write_unavailable_requests
      ? options.t('Not recorded')
      : channel.usage.cache_write_tokens,
    channel.usage.output_tokens,
    channel.usage.requests,
    channel.usage.billable_calls,
    billingDataQualityLabel(channel.data_quality, options.t),
    new Date(options.generatedAt * 1000).toISOString(),
  ]
}

export function downloadUpstreamUrlStatementCsv(
  options: UpstreamUrlStatementCsvOptions
) {
  const csv = buildUpstreamUrlStatementCsv(options)
  const blob = new Blob([`\uFEFF${csv}`], {
    type: 'text/csv;charset=utf-8',
  })
  const url = URL.createObjectURL(blob)
  const link = document.createElement('a')
  link.href = url
  link.download = `upstream-url-usage-statement-${options.month}.csv`
  document.body.append(link)
  link.click()
  link.remove()
  URL.revokeObjectURL(url)
}
