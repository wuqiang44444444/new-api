package constant

const (
	MoxingImageProviderModelSeedream5Lite = "doubao-seedream-5-0-260128"
	MoxingImageProviderModelSeedream5Pro  = "doubao-seedream-5-0-pro-260628"
	MoxingImageSeedream5LiteSize          = "2K"
	MoxingImageSeedream5ProSize           = "2K"
)

type moxingImageProfile struct {
	providerModel string
	fixedSize     string
}

var moxingImageProfiles = []moxingImageProfile{
	{providerModel: MoxingImageProviderModelSeedream5Lite, fixedSize: MoxingImageSeedream5LiteSize},
	{providerModel: MoxingImageProviderModelSeedream5Pro, fixedSize: MoxingImageSeedream5ProSize},
}

func MoxingImageFixedSize(providerModel string) (string, bool) {
	for _, profile := range moxingImageProfiles {
		if profile.providerModel == providerModel {
			return profile.fixedSize, true
		}
	}
	return "", false
}

func MoxingImageProviderModels() []string {
	models := make([]string, 0, len(moxingImageProfiles))
	for _, profile := range moxingImageProfiles {
		models = append(models, profile.providerModel)
	}
	return models
}
