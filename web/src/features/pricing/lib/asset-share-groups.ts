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
import type { PricingModel } from '../types'

const DEFAULT_LABEL_LENGTH = 4

export type AssetShareGroup = {
  label: string
  models: string[]
}

export function buildAssetShareGroups(
  models: PricingModel[]
): Map<string, AssetShareGroup> {
  const modelsByScope = new Map<string, string[]>()
  for (const model of models) {
    const scope = model.api?.assets?.reuse_scope?.trim()
    if (
      model.available !== true ||
      model.api?.assets?.supported !== true ||
      !scope
    ) {
      continue
    }
    const groupModels = modelsByScope.get(scope) ?? []
    groupModels.push(model.model_name)
    modelsByScope.set(scope, groupModels)
  }

  const scopes = [...modelsByScope.keys()].sort()
  let labelLength = DEFAULT_LABEL_LENGTH
  while (labelLength < 64) {
    const labels = new Set(
      scopes.map((scope) => scopeDigest(scope).slice(0, labelLength))
    )
    if (labels.size === scopes.length) break
    labelLength += 1
  }

  const result = new Map<string, AssetShareGroup>()
  for (const scope of scopes) {
    result.set(scope, {
      label: scopeDigest(scope).slice(0, labelLength).toUpperCase(),
      models: [...(modelsByScope.get(scope) ?? [])].sort(),
    })
  }
  return result
}

function scopeDigest(scope: string): string {
  return scope.replace(/^asset_scope_/, '')
}
