import i18next from 'i18next'
import { z } from 'zod'

import { CHANNEL_TYPE_SEEDANCE_LINK } from '../constants'
import type { ChannelFormValues } from './channel-form'

const officialAssetRegionPattern = /^[a-z]{2}(?:-[a-z]+)+-\d+$/

export function refineSeedanceProtocols(
  data: ChannelFormValues,
  ctx: z.RefinementCtx
): void {
  if (data.type !== CHANNEL_TYPE_SEEDANCE_LINK) return

  const addIssue = (path: string, message: string): void => {
    ctx.addIssue({ code: z.ZodIssueCode.custom, path: [path], message })
  }

  if (!data.video_upstream_protocol) {
    addIssue(
      'video_upstream_protocol',
      i18next.t('Select a Seedance video protocol')
    )
  }
  if (data.multi_key_mode && data.multi_key_mode !== 'single') {
    addIssue(
      'multi_key_mode',
      i18next.t(
        'Seedance channels use one credential and do not support multi-key mode'
      )
    )
  }

  const assetProtocol = data.asset_upstream_protocol || 'none'
  const declaration = data.seedance_plugin_configuration
  const video = declaration?.videos.find(
    (candidate) => candidate.protocol === data.video_upstream_protocol
  )
  if (video) {
    const asset = declaration?.assets.find(
      (candidate) => candidate.protocol === assetProtocol
    )
    if (!video.assetProtocols.includes(assetProtocol) || !asset) {
      addIssue(
        'asset_upstream_protocol',
        i18next.t('Asset upstream configuration is invalid.')
      )
      return
    }
    if (asset.defaultURLTTLSeconds && !data.asset_min_url_ttl_seconds) {
      addIssue(
        'asset_min_url_ttl_seconds',
        i18next.t('A verified Provider URL fetch window is required')
      )
    }
    for (const [field, rule] of [
      ['asset_provider_project', asset.project],
      ['asset_region', asset.region],
    ] as const) {
      if (!rule) continue
      const value = data[field]?.trim() || ''
      if (
        (rule.required && !value) ||
        (rule.fixed && value !== rule.fixed) ||
        (value &&
          rule.format === 'region_id' &&
          !officialAssetRegionPattern.test(value))
      ) {
        addIssue(field, i18next.t('Asset upstream configuration is invalid.'))
      }
    }
    if (asset.credential === 'asset_key_pair') {
      const accessKeyID = data.asset_access_key_id?.trim() || ''
      const secretAccessKey = data.asset_secret_access_key?.trim() || ''
      if ((accessKeyID === '') !== (secretAccessKey === '')) {
        addIssue(
          accessKeyID ? 'asset_secret_access_key' : 'asset_access_key_id',
          i18next.t(
            'Asset Access Key ID and Secret Access Key must be provided together'
          )
        )
      } else if (!accessKeyID && !data.asset_credential_configured) {
        addIssue(
          'asset_access_key_id',
          i18next.t('Asset credentials are required')
        )
      }
    }
    return
  }
  addIssue('video_upstream_protocol', i18next.t('Select a Seedance video protocol'))
}
