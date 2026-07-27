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
import dayjs from '@/lib/dayjs'

import type {
  BillingPeriod,
  BillingPreset,
  BillingStatementItem,
  BillingSummary,
} from './types'

export function resolveBillingPeriod(
  preset: BillingPreset,
  customStart?: number,
  customEnd?: number
): BillingPeriod {
  const now = dayjs()
  const currentMonday = now.startOf('day').subtract((now.day() + 6) % 7, 'day')

  if (
    preset === 'custom' &&
    customStart != null &&
    customEnd != null &&
    customStart > 0 &&
    customEnd >= customStart
  ) {
    return {
      start_timestamp: Math.floor(customStart / 1000),
      end_timestamp: Math.floor(customEnd / 1000),
    }
  }

  if (preset === 'last-week') {
    return {
      start_timestamp: currentMonday.subtract(7, 'day').unix(),
      end_timestamp: currentMonday.subtract(1, 'second').unix(),
    }
  }
  if (preset === 'last-7-days') {
    return {
      start_timestamp: now.subtract(7, 'day').unix(),
      end_timestamp: now.unix(),
    }
  }
  if (preset === 'last-30-days') {
    return {
      start_timestamp: now.subtract(30, 'day').unix(),
      end_timestamp: now.unix(),
    }
  }
  return {
    start_timestamp: currentMonday.unix(),
    end_timestamp: now.unix(),
  }
}

export function summarizeBillingItems(
  items: BillingStatementItem[],
  period: BillingPeriod
): BillingSummary {
  const summary: BillingSummary = {
    requests: 0,
    prompt_tokens: 0,
    completion_tokens: 0,
    total_tokens: 0,
    gross_quota: 0,
    refund_quota: 0,
    net_quota: 0,
    average_rpm: 0,
    average_tpm: 0,
    average_use_time_seconds: 0,
    stream_ratio: 0,
  }
  let totalUseTime = 0
  let streamRequests = 0
  for (const item of items) {
    summary.requests += item.requests
    summary.prompt_tokens += item.prompt_tokens
    summary.completion_tokens += item.completion_tokens
    summary.gross_quota += item.gross_quota
    summary.refund_quota += item.refund_quota
    summary.net_quota += item.net_quota
    totalUseTime += item.average_use_time_seconds * item.requests
    streamRequests += item.stream_requests
  }
  summary.total_tokens = summary.prompt_tokens + summary.completion_tokens
  if (summary.requests > 0) {
    summary.average_use_time_seconds = totalUseTime / summary.requests
    summary.stream_ratio = streamRequests / summary.requests
  }
  const periodMinutes = (period.end_timestamp - period.start_timestamp) / 60
  if (periodMinutes > 0) {
    summary.average_rpm = summary.requests / periodMinutes
    summary.average_tpm = summary.total_tokens / periodMinutes
  }
  return summary
}

export function formatBillingDuration(seconds: number, locale?: string) {
  if (!Number.isFinite(seconds)) return '-'
  if (seconds < 1) {
    return `${new Intl.NumberFormat(locale, {
      maximumFractionDigits: 0,
    }).format(seconds * 1000)} ms`
  }
  return `${new Intl.NumberFormat(locale, {
    maximumFractionDigits: 2,
  }).format(seconds)} s`
}

export function escapeCsvCell(value: string | number) {
  const text = String(value)
  if (!/[",\n]/.test(text)) return text
  return `"${text.replaceAll('"', '""')}"`
}
