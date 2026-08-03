package model

import (
	"fmt"
	"net/url"
	"strings"

	"github.com/QuantumNous/new-api/dto"
)

const (
	funCloudStandardCreatePath = "/api/v2/open/aigc/seedance2-0"
	funCloudFastCreatePath     = "/api/v2/open/aigc/seedance2-0-fast"
	funCloudQueryPath          = "/api/v2/open/aigc/{task_id}"
)

func validateFunCloudVideoProfileChannel(channel *Channel, settings *dto.ChannelOtherSettings) error {
	if channel == nil || settings == nil || settings.VideoUpstreamProfile != dto.VideoUpstreamProfileThirdPartyFunCloudSeedanceV2 {
		return nil
	}
	baseURL := ""
	if channel.BaseURL != nil {
		baseURL = strings.TrimSpace(*channel.BaseURL)
	}
	parsed, err := url.Parse(baseURL)
	if err != nil || parsed.Scheme != "https" || parsed.Host == "" || parsed.User != nil || parsed.RawQuery != "" || parsed.Fragment != "" {
		return fmt.Errorf("FunCloud video profile base url must be HTTPS without userinfo, query, or fragment")
	}
	createPath := strings.TrimSpace(settings.VideoUpstreamCreatePath)
	if createPath != funCloudStandardCreatePath && createPath != funCloudFastCreatePath {
		return fmt.Errorf("FunCloud video profile create path is not supported")
	}
	if strings.TrimSpace(settings.VideoUpstreamQueryPathTemplate) != funCloudQueryPath {
		return fmt.Errorf("FunCloud video profile query path must be %s", funCloudQueryPath)
	}
	for _, rawModel := range strings.Split(channel.Models, ",") {
		modelName := strings.TrimSpace(rawModel)
		if modelName == "" {
			continue
		}
		capability, ok := ResolveVideoSKUCapability(modelName)
		if !ok || !capability.SupportsProfile(VideoProfileFunCloudSeedanceV2) {
			return fmt.Errorf("video model %q is not capability-equivalent to the FunCloud profile", modelName)
		}
		if createPath == funCloudFastCreatePath && modelName != VideoSKUSeedance20Fast {
			return fmt.Errorf("FunCloud Fast only supports %s", VideoSKUSeedance20Fast)
		}
		if createPath == funCloudStandardCreatePath && modelName != VideoSKUSeedance20Standard {
			return fmt.Errorf("FunCloud Standard only supports %s", VideoSKUSeedance20Standard)
		}
	}
	return nil
}
