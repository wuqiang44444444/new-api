package feicai

import (
	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/dto"
)

type createRequest struct {
	Model    string   `json:"model"`
	Prompt   string   `json:"prompt"`
	Duration int      `json:"duration"`
	Ratio    string   `json:"ratio"`
	Images   []string `json:"images,omitempty"`
	Audios   []string `json:"audios,omitempty"`
	Videos   []string `json:"videos,omitempty"`
}

func CreateRequest(input *dto.ModelArkVideoCreateRequest, providerModel string) ([]byte, error) {
	resolved, err := ResolveRequest(input, providerModel)
	if err != nil {
		return nil, err
	}
	return common.Marshal(createRequest{
		Model: resolved.Model, Prompt: resolved.Prompt, Duration: resolved.Duration, Ratio: resolved.Ratio,
		Images: resolved.Images, Audios: resolved.Audios, Videos: resolved.Videos,
	})
}
