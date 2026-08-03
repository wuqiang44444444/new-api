import { z } from 'zod'

import type { ChannelFormValues } from './channel-form'

const officialAssetRegionPattern = /^[a-z]{2}(?:-[a-z]+)+-\d+$/

export function refineAssetUpstreamProfile(
  data: ChannelFormValues,
  ctx: z.RefinementCtx
): void {
  if (
    data.type !== 54 ||
    !data.asset_upstream_profile ||
    data.asset_upstream_profile === 'none'
  ) {
    return
  }

  const addIssue = (path: string, message: string): void => {
    ctx.addIssue({ code: z.ZodIssueCode.custom, path: [path], message })
  }

  if (!data.asset_min_url_ttl_seconds || data.asset_min_url_ttl_seconds <= 0) {
    addIssue(
      'asset_min_url_ttl_seconds',
      'A verified Provider URL fetch window is required'
    )
  }
  if (data.multi_key_mode && data.multi_key_mode !== 'single') {
    addIssue(
      'asset_upstream_profile',
      'Asset upstream profiles require a single API key'
    )
  }
  if (
    data.asset_upstream_profile === 'ark_assets' &&
    data.video_upstream_profile !== 'third_party_reverse_proxy'
  ) {
    addIssue(
      'asset_upstream_profile',
      'Ark Assets requires the third-party reverse proxy video profile'
    )
  }
  if (
    data.asset_upstream_profile === 'relay_assets' &&
    data.video_upstream_profile !== 'third_party_relay'
  ) {
    addIssue(
      'asset_upstream_profile',
      'Relay Assets requires the third-party relay video profile'
    )
  }
  if (
    data.asset_upstream_profile === 'official_action_assets' &&
    data.video_upstream_profile !== 'official'
  ) {
    addIssue(
      'asset_upstream_profile',
      'Official Action Assets requires the official video profile'
    )
  }
  if (
    data.asset_upstream_profile === 'official_action_assets' &&
    !data.asset_provider_project?.trim()
  ) {
    addIssue(
      'asset_provider_project',
      'A Provider Project is required for official Actions'
    )
  }
  if (
    data.asset_upstream_profile === 'official_action_assets' &&
    !data.asset_region?.trim()
  ) {
    addIssue('asset_region', 'A Region is required for official Actions')
  } else if (
    data.asset_upstream_profile === 'official_action_assets' &&
    !officialAssetRegionPattern.test(data.asset_region?.trim() || '')
  ) {
    addIssue(
      'asset_region',
      'Region must use a Provider region ID such as ap-southeast-1'
    )
  }
  if (data.asset_upstream_profile === 'official_action_assets') {
    const accessKeyID = data.asset_access_key_id?.trim() || ''
    const secretAccessKey = data.asset_secret_access_key?.trim() || ''
    if ((accessKeyID === '') !== (secretAccessKey === '')) {
      if (accessKeyID === '') {
        addIssue(
          'asset_access_key_id',
          'Asset Access Key ID and Secret Access Key must be provided together'
        )
      }
      if (secretAccessKey === '') {
        addIssue(
          'asset_secret_access_key',
          'Asset Access Key ID and Secret Access Key must be provided together'
        )
      }
    } else if (
      accessKeyID === '' &&
      data.asset_credential_configured !== true
    ) {
      addIssue(
        'asset_access_key_id',
        'Asset Access Key ID is required for official Actions'
      )
      addIssue(
        'asset_secret_access_key',
        'Asset Secret Access Key is required for official Actions'
      )
    }
  }
}
