import assert from 'node:assert/strict'

import { describe, test } from 'vitest'

import {
  getOfficialConnectivityAvailability,
  getOfficialConnectivityMessage,
  maskAssetCredentialHint,
} from '../official-channel-connectivity'

describe('official channel connectivity', () => {
  test('maps stable backend codes to translatable messages', () => {
    assert.equal(
      getOfficialConnectivityMessage(
        {
          success: false,
          error_code: 'asset_action_upstream_rejected',
          message: 'untranslated provider detail',
        },
        'fallback'
      ),
      'Official asset Action rejected the request.'
    )
    assert.equal(
      getOfficialConnectivityMessage({ success: false }, 'fallback'),
      'fallback'
    )
    assert.equal(
      getOfficialConnectivityMessage(
        { success: false, error_code: 'asset_resources_active' },
        'fallback'
      ),
      'Delete all active assets and asset groups before clearing credentials.'
    )
    assert.equal(
      getOfficialConnectivityMessage(
        {
          success: false,
          error_code: 'asset_upstream_rejected',
          message: 'provider detail',
        },
        'fallback'
      ),
      'Asset upstream rejected the request.'
    )
  })

  test('masks asset credential hints without exposing their original length', () => {
    assert.equal(maskAssetCredentialHint('abc'), '***')
    assert.equal(maskAssetCredentialHint('AK123456789'), 'AK******789')
    assert.equal(maskAssetCredentialHint('AK******789'), 'AK******789')
  })

  test('keeps both tests visible but blocks persisted tests for draft secrets', () => {
    const available = getOfficialConnectivityAvailability({
      assetProtocol: 'byteplus_assets_action_v2024_01_01',
      savedAssetProtocol: 'byteplus_assets_action_v2024_01_01',
      videoProtocol: 'modelark_v3_byteplus',
      savedVideoProtocol: 'modelark_v3_byteplus',
      credentialConfigured: true,
      hasPendingVideoKey: true,
      hasPendingAssetCredential: true,
      sensitiveLocked: false,
    })

    assert.equal(available.videoCanTest, false)
    assert.equal(available.assetCanTest, false)
    assert.equal(available.hasUnsavedTestChanges, true)
  })

  test('enables the saved Volcengine official asset test with stored credentials', () => {
    const availability = getOfficialConnectivityAvailability({
      assetProtocol: 'volcengine_assets_action_v2024_01_01',
      savedAssetProtocol: 'volcengine_assets_action_v2024_01_01',
      videoProtocol: 'modelark_v3_volcengine',
      savedVideoProtocol: 'modelark_v3_volcengine',
      credentialConfigured: true,
      hasPendingVideoKey: false,
      hasPendingAssetCredential: false,
      sensitiveLocked: false,
    })

    assert.equal(availability.videoCanTest, true)
    assert.equal(availability.assetCanTest, true)
    assert.equal(availability.hasUnsavedTestChanges, false)
  })

  test('enables both saved CMCC connectivity probes with stored credentials', () => {
    const availability = getOfficialConnectivityAvailability({
      assetProtocol: 'cmcc_aicc_assets_v2',
      savedAssetProtocol: 'cmcc_aicc_assets_v2',
      videoProtocol: 'modelark_v3_cmcc',
      savedVideoProtocol: 'modelark_v3_cmcc',
      credentialConfigured: true,
      hasPendingVideoKey: false,
      hasPendingAssetCredential: false,
      sensitiveLocked: false,
    })

    assert.equal(availability.videoCanTest, true)
    assert.equal(availability.assetCanTest, true)
    assert.equal(availability.hasUnsavedTestChanges, false)
  })

  test('allows explicit clear only after the official asset profile is saved disabled', () => {
    const active = getOfficialConnectivityAvailability({
      assetProtocol: 'none',
      savedAssetProtocol: 'byteplus_assets_action_v2024_01_01',
      videoProtocol: 'modelark_v3_byteplus',
      savedVideoProtocol: 'modelark_v3_byteplus',
      credentialConfigured: true,
      hasPendingVideoKey: false,
      hasPendingAssetCredential: false,
      sensitiveLocked: false,
    })
    assert.equal(active.canClearCredential, false)

    const disabled = getOfficialConnectivityAvailability({
      assetProtocol: 'none',
      savedAssetProtocol: 'none',
      videoProtocol: 'modelark_v3_byteplus',
      savedVideoProtocol: 'modelark_v3_byteplus',
      credentialConfigured: true,
      hasPendingVideoKey: false,
      hasPendingAssetCredential: false,
      sensitiveLocked: false,
    })
    assert.equal(disabled.canClearCredential, true)

    assert.equal(
      getOfficialConnectivityAvailability({
        assetProtocol: 'none',
        savedAssetProtocol: 'none',
        videoProtocol: 'modelark_v3_byteplus',
        savedVideoProtocol: 'modelark_v3_byteplus',
        credentialConfigured: true,
        hasPendingVideoKey: false,
        hasPendingAssetCredential: true,
        sensitiveLocked: false,
      }).canClearCredential,
      false
    )
  })

  test('enables saved bearer-key asset profiles without separate credentials', () => {
    const cases = [
      ['ark_assets_v1', 'ark_media_v1'],
      ['tokensave_assets_v1', 'tokensave_media_task_v1'],
      ['moxing_volc_assets_v1', 'moxing_modelark_media_v1'],
      ['funcloud_material', 'funcloud_seedance'],
    ] as const
    for (const [assetProtocol, videoProtocol] of cases) {
      const availability = getOfficialConnectivityAvailability({
        assetProtocol,
        savedAssetProtocol: assetProtocol,
        videoProtocol,
        savedVideoProtocol: videoProtocol,
        credentialConfigured: false,
        hasPendingVideoKey: false,
        hasPendingAssetCredential: false,
        sensitiveLocked: false,
      })

      assert.equal(availability.assetCanTest, true)
      assert.equal(availability.videoCanTest, false)
      assert.equal(availability.hasUnsavedTestChanges, false)
    }

    const joyCreator = getOfficialConnectivityAvailability({
      assetProtocol: 'moxing_joycreator_assets_v1',
      savedAssetProtocol: 'moxing_joycreator_assets_v1',
      videoProtocol: 'moxing_media_task_v1',
      savedVideoProtocol: 'moxing_media_task_v1',
      credentialConfigured: false,
      hasPendingVideoKey: false,
      hasPendingAssetCredential: false,
      sensitiveLocked: false,
    })
    assert.equal(joyCreator.assetCanTest, false)
  })
})
