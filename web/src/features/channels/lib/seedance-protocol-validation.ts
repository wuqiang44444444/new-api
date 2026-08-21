import i18next from 'i18next'
import { z } from 'zod'

import { CHANNEL_TYPE_SEEDANCE_LINK } from '../constants'
import type { ChannelFormValues } from './channel-form'
import {
  isFunCloud25ProviderModel,
  isOfficialSeedanceAssetProtocol,
} from './seedance-protocol-pairing'

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
  if (assetProtocol === 'none') return
  if (!data.asset_min_url_ttl_seconds) {
    addIssue(
      'asset_min_url_ttl_seconds',
      i18next.t('A verified Provider URL fetch window is required')
    )
  }
  if (
    assetProtocol === 'ark_assets_v1' &&
    data.video_upstream_protocol !== 'ark_media_v1'
  ) {
    addIssue(
      'asset_upstream_protocol',
      i18next.t('Ark Assets requires the Ark Media video protocol')
    )
  }
  if (
    assetProtocol === 'tokensave_assets_v1' &&
    data.video_upstream_protocol !== 'tokensave_media_task_v1'
  ) {
    addIssue(
      'asset_upstream_protocol',
      i18next.t(
        'TokenSave Assets requires the TokenSave Media Task video protocol'
      )
    )
  }
  if (
    assetProtocol === 'moxing_joycreator_assets_v1' &&
    data.video_upstream_protocol !== 'moxing_media_task_v1'
  ) {
    addIssue(
      'asset_upstream_protocol',
      i18next.t(
        'Moxing JoyCreator Assets requires the Moxing Media Task video protocol'
      )
    )
  }
  if (
    assetProtocol === 'moxing_volc_assets_v1' &&
    data.video_upstream_protocol !== 'moxing_modelark_media_v1'
  ) {
    addIssue(
      'asset_upstream_protocol',
      i18next.t(
        'Moxing Volcengine Assets requires the Moxing ModelArk video protocol'
      )
    )
  }
  if (
    assetProtocol === 'funcloud_material' &&
    data.video_upstream_protocol !== 'funcloud_seedance'
  ) {
    addIssue(
      'asset_upstream_protocol',
      i18next.t(
        'FunCloud Material requires the FunCloud Seedance video protocol'
      )
    )
  }
  if (
    assetProtocol === 'funcloud_material' &&
    isFunCloud25ProviderModel(data.model_mapping)
  ) {
    addIssue(
      'asset_upstream_protocol',
      i18next.t(
        'FunCloud Seedance 2.5 does not support the FunCloud Material Library'
      )
    )
  }
  if (
    assetProtocol === 'cmcc_aicc_assets_v2' &&
    data.video_upstream_protocol !== 'modelark_v3_cmcc'
  ) {
    addIssue(
      'asset_upstream_protocol',
      i18next.t(
        'CMCC AICC Assets requires the CMCC ModelArk V3 video protocol'
      )
    )
  }
  if (
    assetProtocol === 'volcengine_assets_action_v2024_01_01' &&
    data.video_upstream_protocol !== 'modelark_v3_volcengine'
  ) {
    addIssue(
      'asset_upstream_protocol',
      i18next.t(
        'Volcengine Assets requires the Volcengine ModelArk V3 video protocol'
      )
    )
  }
  if (
    assetProtocol === 'byteplus_assets_action_v2024_01_01' &&
    data.video_upstream_protocol !== 'modelark_v3_byteplus'
  ) {
    addIssue(
      'asset_upstream_protocol',
      i18next.t(
        'BytePlus Assets requires the BytePlus ModelArk V3 video protocol'
      )
    )
  }
  if (!isOfficialSeedanceAssetProtocol(assetProtocol)) return
  const accessKeyID = data.asset_access_key_id?.trim() || ''
  const secretAccessKey = data.asset_secret_access_key?.trim() || ''
  if ((accessKeyID === '') !== (secretAccessKey === '')) {
    addIssue(
      accessKeyID ? 'asset_secret_access_key' : 'asset_access_key_id',
      i18next.t(
        'Asset Access Key ID and Secret Access Key must be provided together'
      )
    )
  } else if (accessKeyID === '' && !data.asset_credential_configured) {
    addIssue(
      'asset_access_key_id',
      i18next.t('Asset credentials are required')
    )
  }
  if (assetProtocol === 'cmcc_aicc_assets_v2') return

  if (!data.asset_provider_project?.trim()) {
    addIssue(
      'asset_provider_project',
      i18next.t('A Provider Project is required')
    )
  }
  if (
    assetProtocol === 'volcengine_assets_action_v2024_01_01' &&
    data.asset_region?.trim() !== 'cn-beijing'
  ) {
    addIssue(
      'asset_region',
      i18next.t('Volcengine Assets uses the fixed cn-beijing region')
    )
  }
  if (
    assetProtocol === 'byteplus_assets_action_v2024_01_01' &&
    !officialAssetRegionPattern.test(data.asset_region?.trim() || '')
  ) {
    addIssue(
      'asset_region',
      i18next.t(
        'Region must use a Provider region ID such as ap-southeast-1'
      )
    )
  }
}
