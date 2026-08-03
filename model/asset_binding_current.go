package model

import (
	"errors"
	"strings"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/dto"
	"gorm.io/gorm"
)

func AssetBindingIsCurrent(binding *AssetBinding) (bool, error) {
	if binding == nil || binding.Status != AssetBindingStatusActive ||
		binding.UpstreamReferenceType != "asset_uri_id" || strings.TrimSpace(binding.UpstreamReferenceValue) == "" {
		return false, nil
	}
	channel, err := GetChannelById(binding.ChannelID, true)
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return false, nil
		}
		return false, err
	}
	return assetBindingIsCurrentForChannel(binding, channel)
}

func assetBindingIsCurrentForChannel(binding *AssetBinding, channel *Channel) (bool, error) {
	if binding == nil || channel == nil || binding.Status != AssetBindingStatusActive ||
		binding.UpstreamReferenceType != "asset_uri_id" || strings.TrimSpace(binding.UpstreamReferenceValue) == "" {
		return false, nil
	}
	profile := dto.AssetUpstreamProfile(binding.UpstreamProfile)
	if !profile.IsRoutable() {
		return false, nil
	}
	if channel.Status != common.ChannelStatusEnabled || channel.Type != constant.ChannelTypeDoubaoVideo || channel.ChannelInfo.IsMultiKey {
		return false, nil
	}
	settings := channel.GetOtherSettings()
	implementation, ok := ResolveChannelLinkImplementation(channel)
	if !ok || binding.LinkImplementationID != implementation.ID || binding.LinkImplementationVer != implementation.Version || binding.LinkImplementationHash != implementation.ContentHash {
		return false, nil
	}
	if IsRegisteredLinkSKU(binding.RequestedModel) && ValidateChannelLinkImplementationForSKU(channel, binding.RequestedModel) != nil {
		return false, nil
	}
	if settings.AssetUpstreamProfile != profile {
		return false, nil
	}
	if profile == dto.AssetUpstreamProfileArk && settings.VideoUpstreamProfile != dto.VideoUpstreamProfileThirdPartyReverseProxy ||
		profile == dto.AssetUpstreamProfileRelay && settings.VideoUpstreamProfile != dto.VideoUpstreamProfileThirdPartyRelay ||
		profile == dto.AssetUpstreamProfileOfficial && settings.VideoUpstreamProfile != "" && settings.VideoUpstreamProfile != dto.VideoUpstreamProfileOfficial {
		return false, nil
	}
	_, fingerprint, err := ResolveAssetChannelCredential(channel)
	if err != nil {
		return false, nil
	}
	return fingerprint == binding.CredentialFingerprint, nil
}
