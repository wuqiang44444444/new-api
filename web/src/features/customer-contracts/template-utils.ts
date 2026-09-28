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
  CustomerContractCatalog,
} from '@/features/users/types'

import type { ContractTemplateSnapshot } from './template-types'

/**
 * Converts a stored template rule into an editable draft rule. Ratio and
 * price facts come from the management catalog; a group or model missing
 * from the catalog keeps its identity with an unconfigured price reference
 * instead of a fake ratio or price.
 */
export function templateRuleToDraft(
  rule: ContractTemplateSnapshot['rules'][number],
  catalog: CustomerContractCatalog
): ContractRuleDraft {
  const discount = Number(rule.ratio_units / 100_000_000)
    .toFixed(8)
    .replace(/\.?0+$/, '')
  const groupEntry = catalog.groups.find(
    (entry) => entry.group === rule.route_group
  )
  const nativeRatio = groupEntry?.native_group_ratio ?? ''
  const modelEntry = groupEntry?.models.find(
    (model) => model.model === rule.public_model
  )
  return {
    model: rule.public_model,
    channel_id: rule.channel_id,
    route_group: rule.route_group,
    discount,
    available: rule.available,
    unavailable_reason: rule.unavailable_reason,
    native_group_ratio: nativeRatio,
    effective_multiplier: nativeRatio,
    special_group_ratio: groupEntry?.special_group_ratio ?? false,
    price: modelEntry?.price ?? null,
  }
}
