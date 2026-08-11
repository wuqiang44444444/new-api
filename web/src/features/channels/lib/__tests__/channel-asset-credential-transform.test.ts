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

import { describe, test } from 'vitest'

import {
  CHANNEL_FORM_DEFAULT_VALUES,
  transformFormDataToCreatePayload,
  transformFormDataToUpdatePayload,
} from '../channel-form'

const officialForm = {
  ...CHANNEL_FORM_DEFAULT_VALUES,
  name: 'official',
  type: 61,
  key: 'video-api-key',
  models: 'video-model',
  video_upstream_protocol: 'modelark_v3_byteplus' as const,
  asset_upstream_protocol: 'byteplus_assets_action_v2024_01_01' as const,
  asset_min_url_ttl_seconds: 3600,
  asset_provider_project: 'project-a',
  asset_region: 'ap-southeast-1',
}

describe('channel asset credential transforms', () => {
  test('persists only the code-backed Seedance protocols', () => {
    const payload = transformFormDataToCreatePayload({
      ...CHANNEL_FORM_DEFAULT_VALUES,
      name: 'funcloud',
      type: 61,
      key: 'video-api-key',
      models: 'seedance-2.0-standard',
      video_upstream_protocol: 'funcloud_seedance_v2',
      asset_upstream_protocol: 'none',
      priority: 99,
      weight: 100,
    })

    const settings = JSON.parse(payload.channel.settings || '{}')
    assert.equal(settings.video_upstream_protocol, 'funcloud_seedance_v2')
    assert.equal(settings.asset_upstream_protocol, 'none')
    assert.equal('video_upstream_create_path' in settings, false)
    assert.equal(payload.channel.priority, null)
    assert.equal(payload.channel.weight, null)
  })

  test('creates a separate write-only credential payload', () => {
    const payload = transformFormDataToCreatePayload({
      ...officialForm,
      asset_access_key_id: ' access ',
      asset_secret_access_key: ' secret ',
    })

    assert.equal(payload.channel.key, 'video-api-key')
    assert.deepEqual(payload.asset_credential, {
      access_key_id: 'access',
      secret_access_key: 'secret',
    })
  })

  test('creates the same write-only credential payload for Volcengine assets', () => {
    const payload = transformFormDataToCreatePayload({
      ...officialForm,
      video_upstream_protocol: 'modelark_v3_volcengine',
      asset_upstream_protocol: 'volcengine_assets_action_v2024_01_01',
      asset_region: 'cn-beijing',
      asset_access_key_id: ' access ',
      asset_secret_access_key: ' secret ',
    })

    assert.deepEqual(payload.asset_credential, {
      access_key_id: 'access',
      secret_access_key: 'secret',
    })
  })

  test('omits blank credentials during edit to preserve the stored value', () => {
    const payload = transformFormDataToUpdatePayload(
      {
        ...officialForm,
        key: '',
        asset_credential_configured: true,
        asset_access_key_id: '',
        asset_secret_access_key: '',
      },
      26
    )

    assert.equal('key' in payload, false)
    assert.equal('asset_credential' in payload, false)
  })
})
