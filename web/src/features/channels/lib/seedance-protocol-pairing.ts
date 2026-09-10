import type { ChannelFormValues } from './channel-form'

export type SeedanceVideoProtocol = NonNullable<
  ChannelFormValues['video_upstream_protocol']
>
export type SeedanceAssetProtocol = NonNullable<
  ChannelFormValues['asset_upstream_protocol']
>

const DEFAULT_ASSET_PROTOCOL_BY_VIDEO: Record<
  SeedanceVideoProtocol,
  SeedanceAssetProtocol
> = {
  modelark_v3_volcengine: 'volcengine_assets_action_v2024_01_01',
  modelark_v3_byteplus: 'byteplus_assets_action_v2024_01_01',
  modelark_v3_cmcc: 'cmcc_aicc_assets_v2',
  tokensave_media_task_v1: 'tokensave_assets_v1',
  moxing_modelark_media_v1: 'moxing_volc_assets_v1',
  ark_media_v1: 'ark_assets_v1',
  feicai_videos_v1: 'none',
  funcloud_modelark_v3: 'funcloud_material',
  synlink_video_v1: 'funcloud_material_hosted',
}

export function getDefaultSeedanceAssetProtocol(
  videoProtocol: SeedanceVideoProtocol
): SeedanceAssetProtocol {
  return DEFAULT_ASSET_PROTOCOL_BY_VIDEO[videoProtocol]
}

export function getCompatibleSeedanceAssetProtocols(
  videoProtocol?: SeedanceVideoProtocol
): SeedanceAssetProtocol[] {
  if (!videoProtocol) return ['none']
  if (videoProtocol === 'funcloud_modelark_v3') {
    return ['funcloud_material', 'funcloud_material_hosted', 'none']
  }
  const defaultProtocol = getDefaultSeedanceAssetProtocol(videoProtocol)
  return defaultProtocol === 'none' ? ['none'] : [defaultProtocol, 'none']
}

export function isOfficialSeedanceAssetProtocol(
  assetProtocol?: string
): boolean {
  return (
    assetProtocol === 'volcengine_assets_action_v2024_01_01' ||
    assetProtocol === 'byteplus_assets_action_v2024_01_01' ||
    assetProtocol === 'cmcc_aicc_assets_v2'
  )
}
