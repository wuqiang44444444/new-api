package model

import (
	"fmt"
	"regexp"
	"strings"

	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/dto"
)

var officialAssetRegionPattern = regexp.MustCompile(`^[a-z]{2}(?:-[a-z]+)+-[0-9]+$`)

func validateChannelAssetSettings(channel *Channel, settings *dto.ChannelOtherSettings) error {
	if channel.Type != constant.ChannelTypeDoubaoVideo {
		if settings.AssetUpstreamProfile != "" && settings.AssetUpstreamProfile != dto.AssetUpstreamProfileNone {
			return fmt.Errorf("asset upstream profile is only supported by DoubaoVideo channels")
		}
		return nil
	}
	if err := dto.ValidateAssetUpstreamProfile(settings.AssetUpstreamProfile); err != nil {
		return err
	}
	assetProfile := settings.AssetUpstreamProfile
	if assetProfile == "" || assetProfile == dto.AssetUpstreamProfileNone {
		return nil
	}
	if settings.AssetMinURLTTLSeconds <= 0 {
		return fmt.Errorf("asset upstream profile requires a positive remote URL minimum TTL")
	}

	isMultiKey := channel.ChannelInfo.IsMultiKey
	keys := channel.GetKeys()
	if channel.Id > 0 {
		if existing, err := GetChannelById(channel.Id, true); err == nil {
			isMultiKey = existing.ChannelInfo.IsMultiKey
			if strings.TrimSpace(channel.Key) == "" {
				keys = existing.GetKeys()
			}
		}
	}
	if isMultiKey || len(keys) != 1 {
		return fmt.Errorf("asset upstream profile requires a single-key channel")
	}
	if assetProfile == dto.AssetUpstreamProfileArk && settings.VideoUpstreamProfile != dto.VideoUpstreamProfileThirdPartyReverseProxy {
		return fmt.Errorf("ark_assets requires third_party_reverse_proxy video profile")
	}
	if assetProfile == dto.AssetUpstreamProfileRelay && settings.VideoUpstreamProfile != dto.VideoUpstreamProfileThirdPartyRelay {
		return fmt.Errorf("relay_assets requires third_party_relay video profile")
	}
	if assetProfile == dto.AssetUpstreamProfileOfficial {
		if settings.VideoUpstreamProfile != "" && settings.VideoUpstreamProfile != dto.VideoUpstreamProfileOfficial {
			return fmt.Errorf("official_action_assets requires the official video profile")
		}
		if strings.TrimSpace(settings.AssetProviderProject) == "" || strings.TrimSpace(settings.AssetRegion) == "" {
			return fmt.Errorf("official_action_assets requires ProviderProject and Region")
		}
		if !officialAssetRegionPattern.MatchString(strings.TrimSpace(settings.AssetRegion)) {
			return fmt.Errorf("official_action_assets Region is invalid")
		}
	}
	return nil
}
