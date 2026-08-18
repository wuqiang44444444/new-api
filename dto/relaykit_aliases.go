package dto

import kitdto "github.com/QuantumNous/new-api/relaykit/dto"

type AudioRequest = kitdto.AudioRequest
type BillingUsage = kitdto.BillingUsage
type BoolValue = kitdto.BoolValue
type ChannelOtherSettings = kitdto.ChannelOtherSettings
type ChannelSettings = kitdto.ChannelSettings
type ClaudeErrorWithStatusCode = kitdto.ClaudeErrorWithStatusCode
type ClaudeRequest = kitdto.ClaudeRequest
type EmbeddingRequest = kitdto.EmbeddingRequest
type GeminiChatRequest = kitdto.GeminiChatRequest
type GeminiUsageMetadata = kitdto.GeminiUsageMetadata
type GeneralErrorResponse = kitdto.GeneralErrorResponse
type GeneralOpenAIRequest = kitdto.GeneralOpenAIRequest
type ImageData = kitdto.ImageData
type ImageRequest = kitdto.ImageRequest
type ImageResponse = kitdto.ImageResponse
type InputTokenDetails = kitdto.InputTokenDetails
type IntValue = kitdto.IntValue
type OpenAIErrorWithStatusCode = kitdto.OpenAIErrorWithStatusCode
type OpenAIResponsesRequest = kitdto.OpenAIResponsesRequest
type OpenAIVideo = kitdto.OpenAIVideo
type OpenAIVideoError = kitdto.OpenAIVideoError
type PlayGroundRequest = kitdto.PlayGroundRequest
type PublicAPIOperation = kitdto.PublicAPIOperation
type PublicAssetAPI = kitdto.PublicAssetAPI
type PublicAssetCreation = kitdto.PublicAssetCreation
type PublicAssetCreateExample = kitdto.PublicAssetCreateExample
type PublicAssetMedia = kitdto.PublicAssetMedia
type PublicAssetSourceContract = kitdto.PublicAssetSourceContract
type PublicAssetSourceExample = kitdto.PublicAssetSourceExample
type PublicAssetSourceMediaTypes = kitdto.PublicAssetSourceMediaTypes
type PublicModelAPI = kitdto.PublicModelAPI
type PublicVideoAPI = kitdto.PublicVideoAPI
type Request = kitdto.Request
type RerankRequest = kitdto.RerankRequest
type Usage = kitdto.Usage

type AdvancedCustomConfig = kitdto.AdvancedCustomConfig
type AdvancedCustomRoute = kitdto.AdvancedCustomRoute
type AdvancedCustomRouteAuth = kitdto.AdvancedCustomRouteAuth

const (
	MaxImageN                        = kitdto.MaxImageN
	PublicAssetNameMaxCharacters     = kitdto.PublicAssetNameMaxCharacters
	PublicAssetFunCloudMaxBytes      = kitdto.PublicAssetFunCloudMaxBytes
	PublicAssetFunCloudRedirectLimit = kitdto.PublicAssetFunCloudRedirectLimit
	PublicAssetGroupRequired         = kitdto.PublicAssetGroupRequired
	PublicAssetGroupOptional         = kitdto.PublicAssetGroupOptional
	PublicAssetGroupUnsupported      = kitdto.PublicAssetGroupUnsupported

	BillingUsageSemanticAnthropic = kitdto.BillingUsageSemanticAnthropic
	BillingUsageSemanticGemini    = kitdto.BillingUsageSemanticGemini
	BillingUsageSemanticOpenAI    = kitdto.BillingUsageSemanticOpenAI

	VideoStatusUnknown    = kitdto.VideoStatusUnknown
	VideoStatusQueued     = kitdto.VideoStatusQueued
	VideoStatusInProgress = kitdto.VideoStatusInProgress
	VideoStatusCompleted  = kitdto.VideoStatusCompleted
	VideoStatusFailed     = kitdto.VideoStatusFailed

	AdvancedCustomAuthTypeNone   = kitdto.AdvancedCustomAuthTypeNone
	AdvancedCustomAuthTypeHeader = kitdto.AdvancedCustomAuthTypeHeader
	AdvancedCustomAuthTypeQuery  = kitdto.AdvancedCustomAuthTypeQuery
)

var HasGeminiUsageMetadataTokens = kitdto.HasGeminiUsageMetadataTokens
var IsAdvancedCustomConverterAllowed = kitdto.IsAdvancedCustomConverterAllowed
var NewOpenAIVideo = kitdto.NewOpenAIVideo
