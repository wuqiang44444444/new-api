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
          maxImages?: number
          allowVideos?: boolean
          maxVideos?: number
          allowAudios?: boolean
          maxAudios?: number
          allowFrameImages?: boolean
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
  const snapshot = response.data.data
  const videos = snapshot?.configuration?.videos
  if (
    !snapshot?.version?.trim() ||
    !Array.isArray(videos) ||
    !videos.some(
      (video) => Array.isArray(video.models) && video.models.length > 0
    )
  ) {
    throw new Error('MiniMax plugin configuration is unavailable')
  }
  return snapshot
}
