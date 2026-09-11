package seedance

import (
	"fmt"
	"net/url"
	"strings"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/dto"
	"github.com/QuantumNous/new-api/model"
	kitdto "github.com/QuantumNous/new-api/relaykit/dto"
)

func validateSynlinkMedia(request *dto.ModelArkVideoCreateRequest) error {
	for _, item := range request.Content {
		for _, media := range []*dto.VideoMediaURL{item.ImageURL, item.VideoURL, item.AudioURL} {
			if media == nil {
				continue
			}
			ref := strings.TrimSpace(media.URL)
			if strings.HasPrefix(ref, "asset://"+model.FunCloudHostedAssetIDPrefix) && item.Type == "image_url" && media == item.ImageURL {
				continue
			}
			// No documented prohibition of audio/video data URLs. Opaque asset
			// references still require the registered hosted-image conversion.
			if (item.Type == "audio_url" || item.Type == "video_url") && strings.HasPrefix(ref, "data:") && strings.IndexByte(ref, ',') > len("data:") {
				continue
			}
			parsed, err := url.Parse(ref)
			if err != nil || parsed.Host == "" || parsed.User != nil || (parsed.Scheme != "http" && parsed.Scheme != "https") {
				return fmt.Errorf("this video protocol requires a platform-hosted image or an HTTP(S) media URL")
			}
		}
	}
	return nil
}

// attachSynlinkLastFrame publishes the internal content marker only when the
// accepted request asked for a last frame. No upstream URL or key is public.
func attachSynlinkLastFrame(task *model.Task, body []byte) ([]byte, error) {
	request := task.PrivateData.ClientRequest
	if request == nil || request.ReturnLastFrame == nil || !*request.ReturnLastFrame {
		return body, nil
	}
	var result map[string]any
	if err := common.Unmarshal(body, &result); err != nil {
		return nil, err
	}
	if result["status"] != "succeeded" {
		return body, nil
	}
	content, ok := result["content"].(map[string]any)
	if !ok {
		return nil, fmt.Errorf("completed task has no content")
	}
	content["last_frame_url"] = kitdto.SynlinkLastFramePath(task.GetUpstreamTaskID())
	return common.Marshal(result)
}
