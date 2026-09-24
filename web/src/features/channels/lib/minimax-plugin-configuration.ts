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
  const video = snapshot?.configuration?.videos?.find(
    (item) => item.protocol === 'jdcloud_video_task_v1'
  )
  if (
    !snapshot?.version?.trim() ||
    !video ||
    !Array.isArray(video.models) ||
    video.models.length === 0
  ) {
    throw new Error('MiniMax plugin configuration is unavailable')
  }
  return snapshot
}
