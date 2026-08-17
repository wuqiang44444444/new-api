package seedance

const (
	modelFunCloud20     = "seedance-2"
	modelFunCloud20Fast = "seedance-2-fast"
	modelFunCloud20Mini = "seedance-2-mini"
	modelFunCloud25     = "seedance-2-5"
)

func funCloudProviderSpec(model string) (providerModelSpec, bool) {
	switch model {
	case modelFunCloud20:
		return providerModelSpec{
			minDuration: 4, maxDuration: 15, intelligentDuration: 15,
			resolutions: stringSet("480p", "720p", "1080p"), maxImages: 3, maxVideos: 1, maxAudios: 1,
			allowVideos: true, allowAudios: true,
		}, true
	case modelFunCloud20Fast, modelFunCloud20Mini:
		return providerModelSpec{
			minDuration: 4, maxDuration: 15, intelligentDuration: 15,
			resolutions: stringSet("480p", "720p"), maxImages: 3, maxVideos: 1, maxAudios: 1,
			allowVideos: true, allowAudios: true,
		}, true
	case modelFunCloud25:
		return providerModelSpec{
			minDuration: 4, maxDuration: 30, intelligentDuration: 30, allowIntelligentDuration: true,
			resolutions: stringSet("480p", "720p"), maxImages: 9, maxVideos: 3, maxAudios: 3,
			allowVideos: true, allowAudios: true,
		}, true
	default:
		return providerModelSpec{}, false
	}
}
