package model

import (
	"fmt"
	"net/url"
	"strings"

	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/dto"
)

func validateJSONVideoMediaArraysChannel(channel *Channel, settings *dto.ChannelOtherSettings) error {
	if channel == nil || settings == nil {
		return nil
	}
	for _, modelName := range strings.Split(channel.Models, ",") {
		publicModel := strings.TrimSpace(modelName)
		capability, registered := ResolveVideoSKUCapability(publicModel)
		if !registered {
			if settings.VideoUpstreamProfile == dto.VideoUpstreamProfileThirdPartyJSONVideoMediaArrays {
				return fmt.Errorf("video model %q has no published capability for the selected channel profile", publicModel)
			}
			continue
		}
		if channel.Type != constant.ChannelTypeDoubaoVideo ||
			!capability.SupportsProfile(string(settings.VideoUpstreamProfile)) {
			return fmt.Errorf("video model %q is not capability-equivalent to the selected channel profile", capability.PublicModel)
		}
	}
	if settings.VideoUpstreamProfile != dto.VideoUpstreamProfileThirdPartyJSONVideoMediaArrays {
		return nil
	}
	baseURL := ""
	if channel.BaseURL != nil {
		baseURL = strings.TrimSpace(*channel.BaseURL)
	}
	parsed, err := url.Parse(baseURL)
	if err != nil || !strings.EqualFold(parsed.Scheme, "https") || parsed.Host == "" ||
		parsed.User != nil || parsed.RawQuery != "" || parsed.Fragment != "" ||
		(parsed.Path != "" && parsed.Path != "/") {
		return fmt.Errorf("JSON video media-arrays profile base url must be an HTTPS origin without userinfo, path, query, or fragment")
	}
	return nil
}
