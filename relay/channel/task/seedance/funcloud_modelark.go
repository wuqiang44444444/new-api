package seedance

import (
	"errors"
	"strings"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/relaykit/dto"
	"github.com/gin-gonic/gin"
)

func funCloudModelArkProviderSpec(model string) (providerModelSpec, bool) {
	spec, ok := dto.FunCloudModelArkSpec(model)
	if !ok {
		return providerModelSpec{}, false
	}
	return providerModelSpec{
		minDuration: spec.MinDuration, maxDuration: spec.MaxDuration,
		intelligentDuration: spec.MaxDuration, allowIntelligentDuration: spec.IntelligentDuration,
		resolutions: stringSet(spec.Resolutions...), maxImages: spec.MaxImages, maxVideos: spec.MaxVideos, maxAudios: spec.MaxAudios,
		allowVideos: true, allowAudios: true, allowAudioOnly: true, defaultGenerateAudio: true, outputFormats: stringSet("mp4", "mov"),
	}, true
}

// buildFunCloudModelArkRequest keeps the published ModelArk V3 north contract:
// it only defaults duration/resolution, pins the FunCloud real-person mode, and
// resolves platform-hosted asset references into internal signed URLs. Hosted
// facts are frozen before the funding hold; missing facts fail closed instead
// of leaking the platform namespace upstream.
func buildFunCloudModelArkRequest(c *gin.Context, body *requestPayload) ([]byte, error) {
	payload := *body
	if payload.Duration == nil {
		value := dto.IntValue(5)
		payload.Duration = &value
	}
	if payload.Resolution == "" {
		payload.Resolution = "720p"
	}
	var err error
	payload.Content, err = resolveHostedImageContent(c, payload.Content)
	if err != nil {
		return nil, err
	}
	if err := rejectUnresolvedHostedMedia(payload.Content); err != nil {
		return nil, err
	}
	output := struct {
		*requestPayload
		OmniReferenceTaskType *string `json:"omni_reference_task_type,omitempty"`
		RealPersonMode        *bool   `json:"real_person_mode,omitempty"`
	}{requestPayload: &payload}
	mode := true
	output.RealPersonMode = &mode
	for _, item := range payload.Content {
		if item.Type == "video_url" {
			mode := "reference"
			output.OmniReferenceTaskType = &mode
			break
		}
	}
	return common.Marshal(output)
}

// rejectUnresolvedHostedMedia 保证 FunCloud V3 带托管前缀的引用都不会在未解析为内部
// URL 的情况下发送给上游；解析缺口一律失败关闭。
func rejectUnresolvedHostedMedia(content []ContentItem) error {
	for _, item := range content {
		for _, media := range []*MediaURL{item.ImageURL, item.VideoURL, item.AudioURL} {
			if media == nil {
				continue
			}
			if strings.HasPrefix(strings.TrimSpace(media.URL), "asset://"+model.FunCloudHostedAssetIDPrefix) {
				return errors.New("hosted asset reference was not resolved before request build")
			}
		}
	}
	return nil
}
