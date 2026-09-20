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
import type {
  ContractRuleDraft,
  CustomerContractChannelGroupOption,
  CustomerContractGroupOption,
} from '@/features/users/types'

import type { ContractTemplateSnapshot } from './template-types'

/** Converts a stored template rule into an editable draft rule. */
export function templateRuleToDraft(
  rule: ContractTemplateSnapshot['rules'][number],
  channels: CustomerContractChannelGroupOption[],
  options: CustomerContractGroupOption[]
): ContractRuleDraft {
  const discount = Number(rule.ratio_units / 100_000_000)
    .toFixed(8)
    .replace(/\.?0+$/, '')
  const channelGroup = channels.find(
    (group) => group.group === rule.route_group
  )
  const nativeRatio = channelGroup?.native_group_ratio || '1'
  const groupOption = options.find(
    (option) => option.group === rule.route_group
  )
  return {
    model: rule.public_model,
    channel_id: rule.channel_id,
    route_group: rule.route_group,
    discount,
    available: rule.available,
    native_group_ratio: nativeRatio,
    effective_multiplier: nativeRatio,
    special_group_ratio: channelGroup?.special_group_ratio ?? false,
    price: groupOption?.prices?.[rule.public_model] || {
      price_type: 'model_ratio' as const,
    },
  }
}
