package model

import (
	"fmt"
	"net/url"
	"strings"

	"github.com/QuantumNous/new-api/dto"
)

const (
	funCloudBaseURL            = "https://mm-internal-cn.leonecloud.com"
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
	if baseURL != funCloudBaseURL {
		return fmt.Errorf("FunCloud video profile base url must be %s", funCloudBaseURL)
	}
	createPath := strings.TrimSpace(settings.VideoUpstreamCreatePath)
	if createPath != funCloudStandardCreatePath && createPath != funCloudFastCreatePath {
		return fmt.Errorf("FunCloud video profile create path is not supported")
	}
	if strings.TrimSpace(settings.VideoUpstreamQueryPathTemplate) != funCloudQueryPath {
		return fmt.Errorf("FunCloud video profile query path must be %s", funCloudQueryPath)
	}
	capabilityModels := map[string]string{}
	if !settings.LinkImplementation.Empty() {
		executions, err := DeriveChannelLinkExecutions(channel, settings)
		if err != nil {
			return err
		}
		for _, execution := range executions {
			capabilityModels[execution.CustomerModel] = execution.LinkSKU
		}
	}
	for _, rawModel := range strings.Split(channel.Models, ",") {
		customerModel := strings.TrimSpace(rawModel)
		if customerModel == "" {
			continue
		}
		modelName := customerModel
		if linkSKU := capabilityModels[customerModel]; linkSKU != "" {
			modelName = linkSKU
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
