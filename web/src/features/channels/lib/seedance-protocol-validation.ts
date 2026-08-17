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
    addIssue('video_upstream_protocol', 'Select a Seedance video protocol')
  }
  if (data.multi_key_mode && data.multi_key_mode !== 'single') {
    addIssue(
      'multi_key_mode',
      'Seedance channels use one credential and do not support multi-key mode'
    )
  }

  const assetProtocol = data.asset_upstream_protocol || 'none'
  if (assetProtocol === 'none') return
  if (!data.asset_min_url_ttl_seconds) {
    addIssue(
      'asset_min_url_ttl_seconds',
      'A verified Provider URL fetch window is required'
    )
  }
  if (
    assetProtocol === 'ark_assets_v1' &&
    data.video_upstream_protocol !== 'ark_media_v1'
  ) {
    addIssue(
      'asset_upstream_protocol',
      'Ark Assets requires the Ark Media video protocol'
    )
  }
  if (
    assetProtocol === 'tokensave_assets_v1' &&
    data.video_upstream_protocol !== 'tokensave_media_task_v1'
  ) {
    addIssue(
      'asset_upstream_protocol',
      'TokenSave Assets requires the TokenSave Media Task video protocol'
    )
  }
  if (
    assetProtocol === 'moxing_joycreator_assets_v1' &&
    data.video_upstream_protocol !== 'moxing_media_task_v1'
  ) {
    addIssue(
      'asset_upstream_protocol',
      'Moxing JoyCreator Assets requires the Moxing Media Task video protocol'
    )
  }
  if (
    assetProtocol === 'moxing_volc_assets_v1' &&
    data.video_upstream_protocol !== 'moxing_modelark_media_v1'
  ) {
    addIssue(
      'asset_upstream_protocol',
      'Moxing Volcengine Assets requires the Moxing ModelArk video protocol'
    )
  }
  if (
    assetProtocol === 'funcloud_material' &&
    data.video_upstream_protocol !== 'funcloud_seedance'
  ) {
    addIssue(
      'asset_upstream_protocol',
      'FunCloud Material requires the FunCloud Seedance video protocol'
    )
  }
  if (
    assetProtocol === 'funcloud_material' &&
    isFunCloud25ProviderModel(data.model_mapping)
  ) {
    addIssue(
      'asset_upstream_protocol',
      'FunCloud Seedance 2.5 does not support the FunCloud Material Library'
    )
  }
  if (
    assetProtocol === 'volcengine_assets_action_v2024_01_01' &&
    data.video_upstream_protocol !== 'modelark_v3_volcengine'
  ) {
    addIssue(
      'asset_upstream_protocol',
      'Volcengine Assets requires the Volcengine ModelArk V3 video protocol'
    )
  }
  if (
    assetProtocol === 'byteplus_assets_action_v2024_01_01' &&
    data.video_upstream_protocol !== 'modelark_v3_byteplus'
  ) {
    addIssue(
      'asset_upstream_protocol',
      'BytePlus Assets requires the BytePlus ModelArk V3 video protocol'
    )
  }
  if (!isOfficialSeedanceAssetProtocol(assetProtocol)) return

  if (!data.asset_provider_project?.trim()) {
    addIssue('asset_provider_project', 'A Provider Project is required')
  }
  if (
    assetProtocol === 'volcengine_assets_action_v2024_01_01' &&
    data.asset_region?.trim() !== 'cn-beijing'
  ) {
    addIssue(
      'asset_region',
      'Volcengine Assets uses the fixed cn-beijing region'
    )
  }
  if (
    assetProtocol === 'byteplus_assets_action_v2024_01_01' &&
    !officialAssetRegionPattern.test(data.asset_region?.trim() || '')
  ) {
    addIssue(
      'asset_region',
      'Region must use a Provider region ID such as ap-southeast-1'
    )
  }
  const accessKeyID = data.asset_access_key_id?.trim() || ''
  const secretAccessKey = data.asset_secret_access_key?.trim() || ''
  if ((accessKeyID === '') !== (secretAccessKey === '')) {
    addIssue(
      accessKeyID ? 'asset_secret_access_key' : 'asset_access_key_id',
      'Asset Access Key ID and Secret Access Key must be provided together'
    )
  } else if (accessKeyID === '' && !data.asset_credential_configured) {
    addIssue('asset_access_key_id', 'Asset credentials are required')
  }
}
