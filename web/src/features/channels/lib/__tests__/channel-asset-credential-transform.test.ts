import assert from 'node:assert/strict'
import { describe, test } from 'node:test'

import {
  CHANNEL_FORM_DEFAULT_VALUES,
  transformFormDataToCreatePayload,
  transformFormDataToUpdatePayload,
} from '../channel-form'

const officialForm = {
  ...CHANNEL_FORM_DEFAULT_VALUES,
  name: 'official',
  type: 54,
  key: 'video-api-key',
  models: 'video-model',
  video_upstream_profile: 'official' as const,
  asset_upstream_profile: 'official_action_assets' as const,
  asset_min_url_ttl_seconds: 3600,
  asset_provider_project: 'project-a',
  asset_region: 'ap-southeast-1',
}

describe('channel asset credential transforms', () => {
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
