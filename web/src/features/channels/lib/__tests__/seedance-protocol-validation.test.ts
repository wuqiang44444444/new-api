import assert from 'node:assert/strict'

import { describe, test } from 'vitest'

import {
  CHANNEL_TYPE_OPTIONS,
  CHANNEL_TYPE_SEEDANCE_LINK,
} from '../../constants'
import {
  CHANNEL_FORM_DEFAULT_VALUES,
  channelFormSchema,
  transformFormDataToCreatePayload,
} from '../channel-form'
import {
  getCompatibleSeedanceAssetProtocols,
  getDefaultSeedanceAssetProtocol,
  isOfficialSeedanceAssetProtocol,
  type SeedanceVideoProtocol,
} from '../seedance-protocol-pairing'

const seedanceForm = {
  ...CHANNEL_FORM_DEFAULT_VALUES,
  name: 'seedance',
  type: 61,
  key: 'one-key',
  models: 'seedance-customer-model',
  group: ['default'],
  base_url: 'https://provider.example.com',
  video_upstream_protocol: 'tokensave_media_task_v1' as const,
  asset_upstream_protocol: 'none' as const,
}

describe('Seedance protocol validation', () => {
  test('links every video protocol to its default asset library', () => {
    const cases: Array<[SeedanceVideoProtocol, string]> = [
      ['modelark_v3_volcengine', 'volcengine_assets_action_v2024_01_01'],
      ['modelark_v3_byteplus', 'byteplus_assets_action_v2024_01_01'],
      ['modelark_v3_cmcc', 'cmcc_aicc_assets_v2'],
      ['tokensave_media_task_v1', 'tokensave_assets_v1'],
      ['moxing_media_task_v1', 'moxing_joycreator_assets_v1'],
      ['moxing_modelark_media_v1', 'moxing_volc_assets_v1'],
      ['ark_media_v1', 'ark_assets_v1'],
      ['feicai_videos_v1', 'none'],
      ['funcloud_seedance', 'funcloud_material'],
    ]

    for (const [videoProtocol, assetProtocol] of cases) {
      assert.equal(
        getDefaultSeedanceAssetProtocol(videoProtocol),
        assetProtocol
      )
    }
  })

  test('allows manual disable but never offers an incompatible library', () => {
    assert.deepEqual(
      getCompatibleSeedanceAssetProtocols('modelark_v3_volcengine'),
      ['volcengine_assets_action_v2024_01_01', 'none']
    )
    assert.deepEqual(
      getCompatibleSeedanceAssetProtocols('tokensave_media_task_v1'),
      ['tokensave_assets_v1', 'none']
    )
    assert.deepEqual(
      getCompatibleSeedanceAssetProtocols('moxing_media_task_v1'),
      ['moxing_joycreator_assets_v1', 'none']
    )
    assert.deepEqual(
      getCompatibleSeedanceAssetProtocols('moxing_modelark_media_v1'),
      ['moxing_volc_assets_v1', 'none']
    )
    assert.deepEqual(getCompatibleSeedanceAssetProtocols('feicai_videos_v1'), [
      'none',
    ])
    assert.deepEqual(getCompatibleSeedanceAssetProtocols('funcloud_seedance'), [
      'funcloud_material',
      'none',
    ])
    assert.deepEqual(
      getCompatibleSeedanceAssetProtocols(
        'funcloud_seedance',
        '{"customer-next":"seedance-2-5"}'
      ),
      ['none']
    )
    assert.equal(
      getDefaultSeedanceAssetProtocol(
        'funcloud_seedance',
        '{"customer-next":"seedance-2-5"}'
      ),
      'none'
    )
  })

  test('treats domestic and overseas official libraries as credentialed', () => {
    assert.equal(
      isOfficialSeedanceAssetProtocol('volcengine_assets_action_v2024_01_01'),
      true
    )
    assert.equal(
      isOfficialSeedanceAssetProtocol('byteplus_assets_action_v2024_01_01'),
      true
    )
    assert.equal(isOfficialSeedanceAssetProtocol('cmcc_aicc_assets_v2'), true)
    assert.equal(isOfficialSeedanceAssetProtocol('tokensave_assets_v1'), false)
  })

  test('uses the dedicated channel name in the localized type selector', () => {
    assert.deepEqual(
      CHANNEL_TYPE_OPTIONS.find(
        (option) => option.value === CHANNEL_TYPE_SEEDANCE_LINK
      ),
      {
        value: CHANNEL_TYPE_SEEDANCE_LINK,
        label: 'Seedance Dedicated Channel',
      }
    )
  })

  test('accepts a single credential with a code-backed protocol', () => {
    assert.equal(channelFormSchema.safeParse(seedanceForm).success, true)
  })

  test('rejects removed generic protocol identifiers', () => {
    assert.equal(
      channelFormSchema.safeParse({
        ...seedanceForm,
        video_upstream_protocol: 'media_task_v1',
        asset_upstream_protocol: 'relay_assets_v1',
      }).success,
      false
    )
    assert.equal(
      channelFormSchema.safeParse({
        ...seedanceForm,
        video_upstream_protocol: 'funcloud_seedance_v2',
        asset_upstream_protocol: 'funcloud_material_v2',
      }).success,
      false
    )
  })

  test('rejects multi-key and mismatched asset protocols', () => {
    const multiKey = channelFormSchema.safeParse({
      ...seedanceForm,
      multi_key_mode: 'batch',
    })
    assert.equal(multiKey.success, false)

    const mismatchedAsset = channelFormSchema.safeParse({
      ...seedanceForm,
      asset_upstream_protocol: 'ark_assets_v1',
      asset_min_url_ttl_seconds: 3600,
    })
    assert.equal(mismatchedAsset.success, false)
  })

  test('rejects FunCloud 2.5 with the FunCloud material library', () => {
    const result = channelFormSchema.safeParse({
      ...seedanceForm,
      models: 'customer-next',
      model_mapping: '{"customer-next":"seedance-2-5"}',
      video_upstream_protocol: 'funcloud_seedance',
      asset_upstream_protocol: 'funcloud_material',
      asset_min_url_ttl_seconds: 3600,
    })
    assert.equal(result.success, false)
  })

  test('rejects FunCloud 2.5 when it is one of multiple administrator mappings', () => {
    const result = channelFormSchema.safeParse({
      ...seedanceForm,
      models: 'customer-standard,customer-next',
      model_mapping:
        '{"customer-standard":"seedance-2","customer-next":"seedance-2-5"}',
      video_upstream_protocol: 'funcloud_seedance',
      asset_upstream_protocol: 'funcloud_material',
      asset_min_url_ttl_seconds: 3600,
    })
    assert.equal(result.success, false)
  })

  test('accepts BytePlus official video and asset credentials together', () => {
    const result = channelFormSchema.safeParse({
      ...seedanceForm,
      video_upstream_protocol: 'modelark_v3_byteplus',
      asset_upstream_protocol: 'byteplus_assets_action_v2024_01_01',
      asset_min_url_ttl_seconds: 3600,
      asset_provider_project: 'project-a',
      asset_region: 'ap-southeast-1',
      asset_access_key_id: 'access-key',
      asset_secret_access_key: 'secret-key',
    })
    assert.equal(result.success, true)
  })

  test('accepts Volcengine official video with its domestic asset library', () => {
    const result = channelFormSchema.safeParse({
      ...seedanceForm,
      video_upstream_protocol: 'modelark_v3_volcengine',
      asset_upstream_protocol: 'volcengine_assets_action_v2024_01_01',
      asset_min_url_ttl_seconds: 3600,
      asset_provider_project: 'default',
      asset_region: 'cn-beijing',
      asset_access_key_id: 'access-key',
      asset_secret_access_key: 'secret-key',
    })
    assert.equal(result.success, true)

    const wrongRegion = channelFormSchema.safeParse({
      ...seedanceForm,
      video_upstream_protocol: 'modelark_v3_volcengine',
      asset_upstream_protocol: 'volcengine_assets_action_v2024_01_01',
      asset_min_url_ttl_seconds: 3600,
      asset_provider_project: 'default',
      asset_region: 'ap-southeast-1',
      asset_access_key_id: 'access-key',
      asset_secret_access_key: 'secret-key',
    })
    assert.equal(wrongRegion.success, false)
  })

  test('accepts CMCC video and separate asset credentials without project fields', () => {
    const form = {
      ...seedanceForm,
      models: 'seedance-2.0-cmcc',
      model_mapping: '{"seedance-2.0-cmcc":"doubao-seedance-2.0"}',
      base_url: 'https://zhenze-huhehaote.cmecloud.cn',
      video_upstream_protocol: 'modelark_v3_cmcc' as const,
      asset_upstream_protocol: 'cmcc_aicc_assets_v2' as const,
      asset_min_url_ttl_seconds: 3600,
      asset_access_key_id: 'access-key',
      asset_secret_access_key: 'secret-key',
    }
    assert.equal(channelFormSchema.safeParse(form).success, true)
    const payload = transformFormDataToCreatePayload(form)
    assert.deepEqual(payload.asset_credential, {
      access_key_id: 'access-key',
      secret_access_key: 'secret-key',
    })
    const settings = JSON.parse(payload.channel.settings || '{}')
    assert.equal(settings.video_upstream_protocol, 'modelark_v3_cmcc')
    assert.equal(settings.asset_upstream_protocol, 'cmcc_aicc_assets_v2')
    assert.equal('asset_provider_project' in settings, false)
    assert.equal('asset_region' in settings, false)
  })

  test('removes Seedance protocol fields when saving a native DoubaoVideo channel', () => {
    const payload = transformFormDataToCreatePayload({
      ...CHANNEL_FORM_DEFAULT_VALUES,
      name: 'native-doubao',
      type: 54,
      key: 'video-key',
      models: 'doubao-video-model',
      settings: JSON.stringify({
        video_upstream_protocol: 'tokensave_media_task_v1',
        asset_upstream_protocol: 'tokensave_assets_v1',
      }),
    })
    const settings = JSON.parse(payload.channel.settings || '{}')
    assert.equal('video_upstream_protocol' in settings, false)
    assert.equal('asset_upstream_protocol' in settings, false)
  })
})
