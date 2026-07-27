import type { ChannelTestResponse } from '../types'

const connectivityMessages: Record<string, string> = {
  asset_action_not_configured:
    'Official asset Action credentials are not configured.',
  asset_action_invalid_configuration:
    'Official asset Action configuration is invalid.',
  asset_action_upstream_rejected: 'Official asset Action rejected the request.',
  asset_action_upstream_unavailable:
    'Official asset Action is temporarily unavailable.',
  asset_upstream_not_configured:
    'Asset upstream credentials are not configured.',
  asset_upstream_invalid_configuration:
    'Asset upstream configuration is invalid.',
  asset_upstream_rejected: 'Asset upstream rejected the request.',
  asset_upstream_unavailable: 'Asset upstream is temporarily unavailable.',
  video_api_not_configured:
    'Official video API credentials are not configured.',
  video_api_invalid_configuration:
    'Official video API configuration is invalid.',
  video_api_upstream_rejected: 'Official video API rejected the request.',
  video_api_upstream_unavailable:
    'Official video API is temporarily unavailable.',
  asset_credential_profile_active:
    'Disable Official Action Assets and save the channel before clearing its stored credentials.',
  asset_resources_active:
    'Delete all active assets, asset groups, and real-person authorizations before clearing credentials.',
}

export function getOfficialConnectivityMessage(
  response: ChannelTestResponse,
  fallback: string
): string {
  if (response.error_code && connectivityMessages[response.error_code]) {
    return connectivityMessages[response.error_code]
  }
  return response.message || fallback
}

export function maskAssetCredentialHint(value?: string): string {
  const characters = [...(value?.trim() || '')]
  if (characters.length <= 5) return '*'.repeat(characters.length)
  return `${characters.slice(0, 2).join('')}******${characters.slice(-3).join('')}`
}

export function getOfficialConnectivityAvailability(input: {
  assetProfile?: string
  savedAssetProfile?: string
  videoProfile?: string
  savedVideoProfile?: string
  credentialConfigured: boolean
  hasPendingVideoKey: boolean
  hasPendingAssetCredential: boolean
  sensitiveLocked: boolean
}) {
  const videoCanTest =
    input.videoProfile === 'official' &&
    (input.savedVideoProfile === 'official' ||
      input.savedVideoProfile === undefined) &&
    !input.hasPendingVideoKey
  const assetProfileSupportsTest = [
    'ark_assets',
    'relay_assets',
    'official_action_assets',
  ].includes(input.assetProfile || '')
  const assetCanTest =
    assetProfileSupportsTest &&
    input.savedAssetProfile === input.assetProfile &&
    (input.assetProfile !== 'official_action_assets' ||
      input.credentialConfigured) &&
    !input.hasPendingAssetCredential
  const canClearCredential =
    input.credentialConfigured &&
    input.assetProfile !== 'official_action_assets' &&
    input.savedAssetProfile !== 'official_action_assets' &&
    !input.hasPendingAssetCredential &&
    !input.sensitiveLocked

  return {
    videoCanTest,
    assetCanTest,
    canClearCredential,
    hasUnsavedTestChanges:
      input.hasPendingVideoKey ||
      input.hasPendingAssetCredential ||
      input.assetProfile !== input.savedAssetProfile ||
      (input.savedVideoProfile !== undefined &&
        input.videoProfile !== input.savedVideoProfile),
  }
}
