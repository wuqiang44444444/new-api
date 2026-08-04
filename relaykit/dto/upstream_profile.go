package dto

import (
	"fmt"
	"net/url"
	"strings"
)

type VideoUpstreamProfile string

const (
	VideoUpstreamProfileOfficial                       VideoUpstreamProfile = "official"
	VideoUpstreamProfileThirdPartyRelay                VideoUpstreamProfile = "third_party_relay"
	VideoUpstreamProfileThirdPartyReverseProxy         VideoUpstreamProfile = "third_party_reverse_proxy"
	VideoUpstreamProfileThirdPartyJSONVideoMediaArrays VideoUpstreamProfile = "third_party_json_video_media_arrays"
	VideoUpstreamProfileThirdPartyFunCloudSeedanceV2   VideoUpstreamProfile = "third_party_funcloud_seedance_v2"
)

func (p VideoUpstreamProfile) IsOfficial() bool {
	return p == "" || p == VideoUpstreamProfileOfficial
}

func (p VideoUpstreamProfile) IsThirdParty() bool {
	return p == VideoUpstreamProfileThirdPartyRelay ||
		p == VideoUpstreamProfileThirdPartyReverseProxy ||
		p == VideoUpstreamProfileThirdPartyJSONVideoMediaArrays ||
		p == VideoUpstreamProfileThirdPartyFunCloudSeedanceV2
}

func (p VideoUpstreamProfile) IsValid() bool {
	switch p {
	case VideoUpstreamProfileOfficial,
		VideoUpstreamProfileThirdPartyRelay,
		VideoUpstreamProfileThirdPartyReverseProxy,
		VideoUpstreamProfileThirdPartyJSONVideoMediaArrays,
		VideoUpstreamProfileThirdPartyFunCloudSeedanceV2:
		return true
	}
	return false
}

func ValidateVideoUpstreamProfile(p VideoUpstreamProfile) error {
	if p == "" || p.IsValid() {
		return nil
	}
	return fmt.Errorf("unknown video upstream profile: %q", p)
}

func ValidateVideoUpstreamURL(baseURL, createPath, queryTemplate string) error {
	baseURL = strings.TrimSpace(baseURL)
	createPath = strings.TrimSpace(createPath)
	queryTemplate = strings.TrimSpace(queryTemplate)

	parsed, err := url.Parse(baseURL)
	if err != nil || parsed.Scheme == "" || parsed.Host == "" {
		return fmt.Errorf("video upstream base url must be an absolute http(s) url")
	}
	if !strings.EqualFold(parsed.Scheme, "http") && !strings.EqualFold(parsed.Scheme, "https") {
		return fmt.Errorf("video upstream base url must use http or https")
	}
	if parsed.RawQuery != "" || parsed.Fragment != "" {
		return fmt.Errorf("video upstream base url must not include query or fragment")
	}

	if !strings.HasPrefix(createPath, "/") || strings.HasPrefix(createPath, "//") {
		return fmt.Errorf("video upstream create path must start with a single /")
	}
	if strings.Contains(createPath, "{task_id}") {
		return fmt.Errorf("video upstream create path must not contain {task_id}")
	}
	if strings.ContainsAny(createPath, "?#") {
		return fmt.Errorf("video upstream create path must not include query or fragment")
	}

	if !strings.HasPrefix(queryTemplate, "/") || strings.HasPrefix(queryTemplate, "//") {
		return fmt.Errorf("video upstream query path template must start with a single /")
	}
	if strings.Count(queryTemplate, "{task_id}") != 1 {
		return fmt.Errorf("video upstream query path template must contain exactly one {task_id}")
	}
	if strings.ContainsAny(queryTemplate, "?#") {
		return fmt.Errorf("video upstream query path template must not include query or fragment")
	}
	return nil
}

type AssetUpstreamProfile string

const (
	AssetUpstreamProfileNone       AssetUpstreamProfile = "none"
	AssetUpstreamProfileArk        AssetUpstreamProfile = "ark_assets"
	AssetUpstreamProfileRelay      AssetUpstreamProfile = "relay_assets"
	AssetUpstreamProfileJoyCreator AssetUpstreamProfile = "joycreator_assets"
	AssetUpstreamProfileOfficial   AssetUpstreamProfile = "official_action_assets"
)

func (p AssetUpstreamProfile) IsValid() bool {
	switch p {
	case "", AssetUpstreamProfileNone, AssetUpstreamProfileArk, AssetUpstreamProfileRelay, AssetUpstreamProfileJoyCreator, AssetUpstreamProfileOfficial:
		return true
	default:
		return false
	}
}

func (p AssetUpstreamProfile) IsRoutable() bool {
	return p == AssetUpstreamProfileArk || p == AssetUpstreamProfileRelay || p == AssetUpstreamProfileOfficial
}

func ValidateAssetUpstreamProfile(p AssetUpstreamProfile) error {
	if !p.IsValid() {
		return fmt.Errorf("unsupported asset upstream profile %q", p)
	}
	return nil
}
