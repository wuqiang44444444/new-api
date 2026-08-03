import assert from 'node:assert/strict'
import { describe, test } from 'node:test'

import { channelFormSchema } from '../channel-form'

const validRelayAssetChannel = {
  name: 'asset-channel',
  type: 54,
  base_url: 'https://example.com',
  key: 'sk-one',
  models: 'video-model',
  group: ['default'],
  status: 1,
  multi_key_mode: 'single' as const,
  video_upstream_profile: 'third_party_relay' as const,
  video_upstream_create_path: '/v1/media/generations',
  video_upstream_query_path_template: '/v1/media/tasks/{task_id}',
  asset_upstream_profile: 'relay_assets' as const,
  asset_min_url_ttl_seconds: 3600,
}

function issueMessages(
  input: Record<string, unknown>
): Array<{ path: string; message: string }> {
  const result = channelFormSchema.safeParse(input)
  if (result.success) return []
  return result.error.issues.map((issue) => ({
    path: issue.path.join('.'),
    message: issue.message,
  }))
}

describe('asset upstream profile validation', () => {
  test('accepts a matching single-key relay profile', () => {
    assert.equal(
      channelFormSchema.safeParse(validRelayAssetChannel).success,
      true
    )
  })

  test('requires a positive verified URL fetch window', () => {
    assert.deepEqual(
      issueMessages({
        ...validRelayAssetChannel,
        asset_min_url_ttl_seconds: 0,
      }),
      [
        {
          path: 'asset_min_url_ttl_seconds',
          message: 'A verified Provider URL fetch window is required',
        },
      ]
    )
  })

  test('rejects multi-key asset channels', () => {
    assert.deepEqual(
      issueMessages({ ...validRelayAssetChannel, multi_key_mode: 'batch' }),
      [
        {
          path: 'asset_upstream_profile',
          message: 'Asset upstream profiles require a single API key',
        },
      ]
    )
  })

  test('requires matching video and asset profiles', () => {
    assert.deepEqual(
      issueMessages({
        ...validRelayAssetChannel,
        asset_upstream_profile: 'ark_assets',
      }),
      [
        {
          path: 'asset_upstream_profile',
          message:
            'Ark Assets requires the third-party reverse proxy video profile',
        },
      ]
    )
  })

  test('accepts a complete official Action profile', () => {
    assert.equal(
      channelFormSchema.safeParse({
        ...validRelayAssetChannel,
        key: 'video-api-key',
        video_upstream_profile: 'official',
        video_upstream_create_path: '',
        video_upstream_query_path_template: '',
        asset_upstream_profile: 'official_action_assets',
        asset_provider_project: 'default',
        asset_region: 'ap-southeast-1',
        asset_access_key_id: 'access',
        asset_secret_access_key: 'secret',
      }).success,
      true
    )
  })

  test('rejects unsafe official Action regions', () => {
    assert.deepEqual(
      issueMessages({
        ...validRelayAssetChannel,
        key: 'video-api-key',
        video_upstream_profile: 'official',
        video_upstream_create_path: '',
        video_upstream_query_path_template: '',
        asset_upstream_profile: 'official_action_assets',
        asset_provider_project: 'default',
        asset_region: 'https://internal.example',
        asset_access_key_id: 'access',
        asset_secret_access_key: 'secret',
      }),
      [
        {
          path: 'asset_region',
          message:
            'Region must use a Provider region ID such as ap-southeast-1',
        },
      ]
    )
  })

  test('rejects a non-provider-shaped official Action region', () => {
    assert.deepEqual(
      issueMessages({
        ...validRelayAssetChannel,
        key: 'video-api-key',
        video_upstream_profile: 'official',
        video_upstream_create_path: '',
        video_upstream_query_path_template: '',
        asset_upstream_profile: 'official_action_assets',
        asset_provider_project: 'default',
        asset_region: 'a',
        asset_access_key_id: 'access',
        asset_secret_access_key: 'secret',
      }),
      [
        {
          path: 'asset_region',
          message:
            'Region must use a Provider region ID such as ap-southeast-1',
        },
      ]
    )
  })

  test('allows an edit to preserve an already configured asset credential', () => {
    assert.equal(
      channelFormSchema.safeParse({
        ...validRelayAssetChannel,
        key: '',
        video_upstream_profile: 'official',
        video_upstream_create_path: '',
        video_upstream_query_path_template: '',
        asset_upstream_profile: 'official_action_assets',
        asset_provider_project: 'default',
        asset_region: 'ap-southeast-1',
        asset_credential_configured: true,
      }).success,
      true
    )
  })

  test('requires both fields when rotating the asset credential', () => {
    assert.deepEqual(
      issueMessages({
        ...validRelayAssetChannel,
        key: 'video-api-key',
        video_upstream_profile: 'official',
        video_upstream_create_path: '',
        video_upstream_query_path_template: '',
        asset_upstream_profile: 'official_action_assets',
        asset_provider_project: 'default',
        asset_region: 'ap-southeast-1',
        asset_credential_configured: true,
        asset_access_key_id: 'new-access',
      }),
      [
        {
          path: 'asset_secret_access_key',
          message:
            'Asset Access Key ID and Secret Access Key must be provided together',
        },
      ]
    )
  })
})
