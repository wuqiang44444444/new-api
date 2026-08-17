import type { ChannelFormValues } from './channel-form'
import { extractRedirectModels } from './model-mapping-validation'

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
  tokensave_media_task_v1: 'tokensave_assets_v1',
  moxing_media_task_v1: 'moxing_joycreator_assets_v1',
  moxing_modelark_media_v1: 'moxing_volc_assets_v1',
  ark_media_v1: 'ark_assets_v1',
  feicai_videos_v1: 'none',
  funcloud_seedance: 'funcloud_material',
}

export function getDefaultSeedanceAssetProtocol(
  videoProtocol: SeedanceVideoProtocol,
  modelMapping?: string
): SeedanceAssetProtocol {
  if (
    videoProtocol === 'funcloud_seedance' &&
    isFunCloud25ProviderModel(modelMapping)
  ) {
    return 'none'
  }
  return DEFAULT_ASSET_PROTOCOL_BY_VIDEO[videoProtocol]
}

export function getCompatibleSeedanceAssetProtocols(
  videoProtocol?: SeedanceVideoProtocol,
  modelMapping?: string
): SeedanceAssetProtocol[] {
  if (!videoProtocol) return ['none']
  const defaultProtocol = getDefaultSeedanceAssetProtocol(
    videoProtocol,
    modelMapping
  )
  return defaultProtocol === 'none' ? ['none'] : [defaultProtocol, 'none']
}

export function isFunCloud25ProviderModel(modelMapping?: string): boolean {
  const providerModels = extractRedirectModels(modelMapping || '')
  return providerModels.length === 1 && providerModels[0] === 'seedance-2-5'
}

export function isOfficialSeedanceAssetProtocol(
  assetProtocol?: string
): boolean {
  return (
    assetProtocol === 'volcengine_assets_action_v2024_01_01' ||
    assetProtocol === 'byteplus_assets_action_v2024_01_01'
  )
}
