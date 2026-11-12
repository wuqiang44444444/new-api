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
import { api } from '@/lib/api'
import { requireServerSuccess } from '@/lib/server-error-message'

import type { ApiResponse, ProviderDiscount } from './types'

export type ChannelBillingConfig = {
  channel_id: number
  channel_name: string
  upstream_name: string
  deleted: boolean
  expected_version: number
  discount: ProviderDiscount | null
}

export function channelBillingConfigQuery(
  channelId: number,
  periodStart: number
) {
  return {
    queryKey: ['channel-billing-config', channelId, periodStart],
    queryFn: async () =>
      requireServerSuccess(
        (
          await api.get<ApiResponse<ChannelBillingConfig>>(
            '/api/billing/admin/channel-config',
            { params: { channel_id: channelId, period_start: periodStart } }
          )
        ).data
      ).data,
    retry: false,
  }
}
