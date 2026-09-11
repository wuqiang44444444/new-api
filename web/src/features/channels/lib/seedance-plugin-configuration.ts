import { api } from '@/lib/api'

import type {
  SeedanceAssetProtocol,
  SeedanceVideoProtocol,
} from './seedance-protocol-pairing'

export type SeedanceConfigurationText = {
  required: boolean
  fixed?: string
  default?: string
  format?: 'region_id'
}

export type SeedancePluginConfiguration = {
  videos: {
    protocol: SeedanceVideoProtocol
    label: string
    models: string[]
    modelPolicy?: 'listed' | 'configured'
    assetProtocols: SeedanceAssetProtocol[]
    defaultAssetProtocol: SeedanceAssetProtocol
  }[]
  assets: {
    protocol: SeedanceAssetProtocol
    label: string
    groupPolicy: 'none' | 'default_fallback' | 'hosted'
    credential: 'none' | 'channel' | 'asset_key_pair'
    connectivity?: boolean
    project?: SeedanceConfigurationText
    region?: SeedanceConfigurationText
    defaultURLTTLSeconds?: number
  }[]
}

export type SeedanceConfigurationSnapshot = {
  version: string
  configuration: SeedancePluginConfiguration | null
}

export async function getSeedancePluginConfiguration(): Promise<SeedanceConfigurationSnapshot> {
  const response = await api.get<{
    success: boolean
    data: SeedanceConfigurationSnapshot
  }>('/api/channel/seedance/configuration')
  if (!response.data.success) {
    throw new Error('Seedance plugin configuration is unavailable')
  }
  return response.data.data
}
