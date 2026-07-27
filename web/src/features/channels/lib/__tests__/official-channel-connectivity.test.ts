import assert from 'node:assert/strict'
import { describe, test } from 'node:test'

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
      'Delete all active assets, asset groups, and real-person authorizations before clearing credentials.'
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
      assetProfile: 'official_action_assets',
      savedAssetProfile: 'official_action_assets',
      videoProfile: 'official',
      savedVideoProfile: 'official',
      credentialConfigured: true,
      hasPendingVideoKey: true,
      hasPendingAssetCredential: true,
      sensitiveLocked: false,
    })

    assert.equal(available.videoCanTest, false)
    assert.equal(available.assetCanTest, false)
    assert.equal(available.hasUnsavedTestChanges, true)
  })

  test('allows explicit clear only after the official asset profile is saved disabled', () => {
    const active = getOfficialConnectivityAvailability({
      assetProfile: 'none',
      savedAssetProfile: 'official_action_assets',
      videoProfile: 'official',
      savedVideoProfile: 'official',
      credentialConfigured: true,
      hasPendingVideoKey: false,
      hasPendingAssetCredential: false,
      sensitiveLocked: false,
    })
    assert.equal(active.canClearCredential, false)

    const disabled = getOfficialConnectivityAvailability({
      assetProfile: 'none',
      savedAssetProfile: 'none',
      videoProfile: 'official',
      savedVideoProfile: 'official',
      credentialConfigured: true,
      hasPendingVideoKey: false,
      hasPendingAssetCredential: false,
      sensitiveLocked: false,
    })
    assert.equal(disabled.canClearCredential, true)

    assert.equal(
      getOfficialConnectivityAvailability({
        assetProfile: 'none',
        savedAssetProfile: 'none',
        videoProfile: 'official',
        savedVideoProfile: 'official',
        credentialConfigured: true,
        hasPendingVideoKey: false,
        hasPendingAssetCredential: true,
        sensitiveLocked: false,
      }).canClearCredential,
      false
    )
  })

  test('enables saved Ark and relay asset profiles without separate credentials', () => {
    for (const assetProfile of ['ark_assets', 'relay_assets']) {
      const availability = getOfficialConnectivityAvailability({
        assetProfile,
        savedAssetProfile: assetProfile,
        videoProfile:
          assetProfile === 'ark_assets'
            ? 'third_party_reverse_proxy'
            : 'third_party_relay',
        savedVideoProfile:
          assetProfile === 'ark_assets'
            ? 'third_party_reverse_proxy'
            : 'third_party_relay',
        credentialConfigured: false,
        hasPendingVideoKey: false,
        hasPendingAssetCredential: false,
        sensitiveLocked: false,
      })

      assert.equal(availability.assetCanTest, true)
      assert.equal(availability.videoCanTest, false)
      assert.equal(availability.hasUnsavedTestChanges, false)
    }
  })
})
