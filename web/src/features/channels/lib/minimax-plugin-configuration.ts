import { api } from '@/lib/api'

export type MinimaxConfigurationSnapshot = {
  version: string
  configuration: {
    videos: {
      protocol: string
      label: string
      models: string[]
      modelMetadata: Record<
        string,
        {
          defaultDuration?: number
          minDuration?: number
          maxDuration?: number
          resolutions?: string[]
          ratios?: string[]
          deleteVideo?: boolean
        }
      >
      assetProtocols: string[]
      defaultAssetProtocol: string
    }[]
    assets: unknown[]
  } | null
}

export async function getMinimaxPluginConfiguration(): Promise<MinimaxConfigurationSnapshot> {
  const response = await api.get<{
    success: boolean
    data: MinimaxConfigurationSnapshot
  }>('/api/channel/minimax/configuration')
  if (!response.data.success) {
    throw new Error('MiniMax plugin configuration is unavailable')
  }
  return response.data.data
}
