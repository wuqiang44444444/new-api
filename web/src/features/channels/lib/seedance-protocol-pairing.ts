import type { ChannelFormValues } from './channel-form'
import type { SeedancePluginConfiguration } from './seedance-plugin-configuration'

export type SeedanceVideoProtocol = NonNullable<
  ChannelFormValues['video_upstream_protocol']
>
export type SeedanceAssetProtocol = NonNullable<
  ChannelFormValues['asset_upstream_protocol']
>

export function getDefaultSeedanceAssetProtocol(
  videoProtocol: SeedanceVideoProtocol,
  configuration: SeedancePluginConfiguration
): SeedanceAssetProtocol {
  return (
    configuration.videos.find((video) => video.protocol === videoProtocol)
      ?.defaultAssetProtocol ?? 'none'
  )
}
export function getCompatibleSeedanceAssetProtocols(
  videoProtocol: SeedanceVideoProtocol | undefined,
  configuration: SeedancePluginConfiguration
): SeedanceAssetProtocol[] {
  return (
    configuration.videos.find((video) => video.protocol === videoProtocol)
      ?.assetProtocols ?? []
  )
}
export function isOfficialSeedanceAssetProtocol(
  assetProtocol: string | undefined,
  configuration?: SeedancePluginConfiguration
): boolean {
  return (
    configuration?.assets.find((asset) => asset.protocol === assetProtocol)
      ?.credential === 'asset_key_pair'
  )
}
