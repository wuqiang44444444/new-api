import { api } from '@/lib/api'

type MinimaxVideoMetadata = {
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

type MinimaxVideoConfiguration = {
  protocol: string
  label: string
  models?: string[] | null
  modelPolicy?: 'listed' | 'configured'
  modelMetadata?: Record<string, MinimaxVideoMetadata>
  defaultModelMetadata?: MinimaxVideoMetadata
  assetProtocols: string[]
  defaultAssetProtocol: string
}

export type MinimaxConfigurationSnapshot = {
  version: string
  enabled: boolean
  configuration: {
    videos: MinimaxVideoConfiguration[]
    assets: unknown[]
  } | null
}

function hasMinimaxVideoCapabilities(video: MinimaxVideoConfiguration) {
  if (video.modelPolicy === 'configured') {
    return Boolean(video.defaultModelMetadata)
  }
  return Array.isArray(video.models) && video.models.length > 0
}

export function findMinimaxVideoDeclaration(
  snapshot: MinimaxConfigurationSnapshot | undefined,
  protocol: string | undefined
) {
  return snapshot?.configuration?.videos.find(
    (video) => video.protocol === protocol && hasMinimaxVideoCapabilities(video)
  )
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
    !videos.some(hasMinimaxVideoCapabilities)
  ) {
    throw new Error('MiniMax plugin configuration is unavailable')
  }
  return snapshot
}
