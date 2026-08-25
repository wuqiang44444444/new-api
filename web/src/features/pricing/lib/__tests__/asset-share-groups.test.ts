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
import assert from 'node:assert/strict'

import { describe, it } from 'vitest'

import type { PricingModel } from '../../types'
import { buildAssetShareGroups } from '../asset-share-groups'

function model(
  modelName: string,
  scope: string,
  available = true
): PricingModel {
  return {
    id: 1,
    model_name: modelName,
    quota_type: 1,
    model_ratio: 0,
    completion_ratio: 0,
    enable_groups: ['default'],
    available,
    api: { assets: { supported: true, reuse_scope: scope } },
  }
}

describe('buildAssetShareGroups', () => {
  it('derives stable labels from scope and groups models by exact scope', () => {
    const scope = 'asset_scope_a7f312345678'
    const groups = buildAssetShareGroups([
      model('model-b', scope),
      model('model-a', scope),
    ])

    assert.deepEqual(groups.get(scope), {
      label: 'A7F3',
      models: ['model-a', 'model-b'],
    })
    assert.equal(
      buildAssetShareGroups([model('model-a', scope)]).get(scope)?.label,
      'A7F3'
    )
  })

  it('extends colliding prefixes and excludes unavailable models', () => {
    const first = 'asset_scope_abcd1aaa'
    const second = 'asset_scope_abcd2bbb'
    const groups = buildAssetShareGroups([
      model('first', first),
      model('second', second),
      model('hidden', 'asset_scope_ffff0000', false),
    ])

    assert.equal(groups.get(first)?.label, 'ABCD1')
    assert.equal(groups.get(second)?.label, 'ABCD2')
    assert.equal(groups.size, 2)
  })
})
