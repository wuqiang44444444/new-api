package dto

import kitdto "github.com/QuantumNous/new-api/relaykit/dto"

type AudioRequest = kitdto.AudioRequest
type BillingUsage = kitdto.BillingUsage
type BoolValue = kitdto.BoolValue
type ChannelOtherSettings = kitdto.ChannelOtherSettings
type ChannelSettings = kitdto.ChannelSettings
type LinkImplementationRef = kitdto.LinkImplementationRef
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
type Request = kitdto.Request
type RerankRequest = kitdto.RerankRequest
type Usage = kitdto.Usage

type AdvancedCustomConfig = kitdto.AdvancedCustomConfig
type AdvancedCustomRoute = kitdto.AdvancedCustomRoute
type AdvancedCustomRouteAuth = kitdto.AdvancedCustomRouteAuth

const (
	MaxImageN = kitdto.MaxImageN

	BillingUsageSemanticAnthropic = kitdto.BillingUsageSemanticAnthropic
	BillingUsageSemanticGemini    = kitdto.BillingUsageSemanticGemini
	BillingUsageSemanticOpenAI    = kitdto.BillingUsageSemanticOpenAI

	VideoStatusUnknown    = kitdto.VideoStatusUnknown
	VideoStatusQueued     = kitdto.VideoStatusQueued
	VideoStatusInProgress = kitdto.VideoStatusInProgress
	VideoStatusCompleted  = kitdto.VideoStatusCompleted
	VideoStatusFailed     = kitdto.VideoStatusFailed

	AdvancedCustomConverterMediaTaskImageBlocking = kitdto.AdvancedCustomConverterMediaTaskImageBlocking
	AdvancedCustomAuthTypeNone                    = kitdto.AdvancedCustomAuthTypeNone
	AdvancedCustomAuthTypeHeader                  = kitdto.AdvancedCustomAuthTypeHeader
	AdvancedCustomAuthTypeQuery                   = kitdto.AdvancedCustomAuthTypeQuery
)

var HasGeminiUsageMetadataTokens = kitdto.HasGeminiUsageMetadataTokens
var IsAdvancedCustomConverterAllowed = kitdto.IsAdvancedCustomConverterAllowed
var NewOpenAIVideo = kitdto.NewOpenAIVideo
