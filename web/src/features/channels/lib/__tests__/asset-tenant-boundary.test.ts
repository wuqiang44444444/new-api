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
import { describe, expect, test } from 'vitest'

import type { Channel } from '../../types'
import {
  assetTenantBoundarySnapshotFromChannel,
  assetTenantBoundarySnapshotFromForm,
  collectAssetTenantBoundaryChanges,
} from '../asset-tenant-boundary'
import { CHANNEL_FORM_DEFAULT_VALUES } from '../channel-form'

const channel = {
  id: 77,
  type: 62,
  base_url: 'https://ark.example.com/',
  settings: JSON.stringify({
    video_upstream_protocol: 'modelark_v3_volcengine',
    asset_upstream_protocol: 'volcengine_assets_action_v2024_01_01',
    asset_provider_project: 'default',
    asset_region: 'cn-beijing',
  }),
} as Channel

describe('asset tenant boundary changes', () => {
  test('reports the exact changed field while ignoring URL slash differences', () => {
    const previous = assetTenantBoundarySnapshotFromChannel(channel)
    const next = assetTenantBoundarySnapshotFromForm({
      ...CHANNEL_FORM_DEFAULT_VALUES,
      base_url: 'https://ark.example.com',
      video_upstream_protocol: 'modelark_v3_volcengine',
      asset_upstream_protocol: 'volcengine_assets_action_v2024_01_01',
      asset_provider_project: 'lumen-test',
      asset_region: 'cn-beijing',
    })

    expect(collectAssetTenantBoundaryChanges(previous, next)).toEqual([
      {
        field: 'asset_provider_project',
        previous: 'default',
        next: 'lumen-test',
      },
    ])
  })

  test('does not require replacement before an asset boundary exists', () => {
    const previous = assetTenantBoundarySnapshotFromChannel({
      ...channel,
      settings: JSON.stringify({
        video_upstream_protocol: 'modelark_v3_volcengine',
        asset_upstream_protocol: 'none',
      }),
    })
    const next = assetTenantBoundarySnapshotFromForm({
      ...CHANNEL_FORM_DEFAULT_VALUES,
      asset_upstream_protocol: 'volcengine_assets_action_v2024_01_01',
      asset_provider_project: 'default',
    })

    expect(collectAssetTenantBoundaryChanges(previous, next)).toEqual([])
  })
})
