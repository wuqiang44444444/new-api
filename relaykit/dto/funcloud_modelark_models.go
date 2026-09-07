package dto

import "sort"

// FunCloudModelArkModelSpec is the shared V3 model contract for validation and
// public projection. It contains no customer pricing or credentials.
type FunCloudModelArkModelSpec struct {
	MinDuration, MaxDuration        int
	IntelligentDuration             bool
	Resolutions                     []string
	MaxImages, MaxVideos, MaxAudios int
}

var funCloudModelArkModels = map[string]FunCloudModelArkModelSpec{
	"seedance-2-0":      {MinDuration: 4, MaxDuration: 15, Resolutions: []string{"480p", "720p"}, MaxImages: 9, MaxVideos: 3, MaxAudios: 3},
	"seedance-2-0-fast": {MinDuration: 4, MaxDuration: 15, Resolutions: []string{"480p", "720p"}, MaxImages: 9, MaxVideos: 3, MaxAudios: 3},
	"seedance-2-0-mini": {MinDuration: 4, MaxDuration: 15, Resolutions: []string{"480p", "720p"}, MaxImages: 9, MaxVideos: 3, MaxAudios: 3},
	"seedance-2-5":      {MinDuration: 4, MaxDuration: 30, IntelligentDuration: true, Resolutions: []string{"480p", "720p", "1080p"}, MaxImages: 9, MaxVideos: 3, MaxAudios: 3},
}

func FunCloudModelArkModels() []string {
	models := make([]string, 0, len(funCloudModelArkModels))
	for model := range funCloudModelArkModels {
		models = append(models, model)
	}
	sort.Strings(models)
	return models
}

func FunCloudModelArkSpec(model string) (FunCloudModelArkModelSpec, bool) {
	spec, ok := funCloudModelArkModels[model]
	spec.Resolutions = append([]string(nil), spec.Resolutions...)
	return spec, ok
}
