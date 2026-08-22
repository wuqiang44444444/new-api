package constant

import "github.com/QuantumNous/new-api/relaykit/dto"

const (
	FunCloudImageProviderModelNanoBanana2Lite = "nano-banana-2-lite"
	FunCloudImageProviderModelNanoBanana2     = "nano-banana-2"
	FunCloudImageProviderModelSeedream5Lite   = "seedream-5.0-lite"
	FunCloudImageProviderModelSeedream5Pro    = "seedream-5.0-pro"
)

type funCloudImageProfile struct {
	providerModel string
	publishedSize string
}

var funCloudImageProfiles = []funCloudImageProfile{
	{providerModel: FunCloudImageProviderModelNanoBanana2Lite},
	{providerModel: FunCloudImageProviderModelNanoBanana2, publishedSize: "1K"},
	{providerModel: FunCloudImageProviderModelSeedream5Lite, publishedSize: "2K"},
	{providerModel: FunCloudImageProviderModelSeedream5Pro, publishedSize: "1K"},
}

func FunCloudImageProviderModels() []string {
	models := make([]string, 0, len(funCloudImageProfiles))
	for _, profile := range funCloudImageProfiles {
		models = append(models, profile.providerModel)
	}
	return models
}

func FunCloudImagePublishedSize(providerModel string) (string, bool) {
	for _, profile := range funCloudImageProfiles {
		if profile.providerModel == providerModel {
			return profile.publishedSize, true
		}
	}
	return "", false
}

func ImageRelayProviderModels(protocol dto.ImageUpstreamProtocol) []string {
	switch protocol {
	case dto.ImageUpstreamProtocolFunCloudAIGCV2:
		return FunCloudImageProviderModels()
	case dto.ImageUpstreamProtocolMoxingImagesV1:
		return MoxingImageProviderModels()
	default:
		return nil
	}
}

func ImageRelaySupportsProviderModel(protocol dto.ImageUpstreamProtocol, providerModel string) bool {
	for _, model := range ImageRelayProviderModels(protocol) {
		if model == providerModel {
			return true
		}
	}
	return false
}

func ImageRelayTestSize(protocol dto.ImageUpstreamProtocol, providerModel string) (string, bool) {
	switch protocol {
	case dto.ImageUpstreamProtocolFunCloudAIGCV2:
		return FunCloudImagePublishedSize(providerModel)
	case dto.ImageUpstreamProtocolMoxingImagesV1:
		return MoxingImageFixedSize(providerModel)
	default:
		return "", false
	}
}
