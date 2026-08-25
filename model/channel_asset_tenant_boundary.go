package model

import (
	"errors"
	"strings"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/dto"
	"gorm.io/gorm"
)

var (
	ErrAssetTenantBoundaryImmutable   = errors.New("asset tenant boundary fields cannot be changed; create a new channel")
	ErrAssetTenantRotationUnconfirmed = errors.New("confirm that credential rotation does not change the asset tenant")
)

func parsedChannelOtherSettings(channel *Channel) (dto.ChannelOtherSettings, error) {
	var settings dto.ChannelOtherSettings
	if channel == nil || strings.TrimSpace(channel.OtherSettings) == "" {
		return settings, nil
	}
	if err := common.UnmarshalJsonStr(channel.OtherSettings, &settings); err != nil {
		return dto.ChannelOtherSettings{}, err
	}
	return settings, nil
}

func validateChannelAssetTenantMutation(
	tx *gorm.DB,
	current *Channel,
	updated *Channel,
	credential *ChannelAssetCredential,
	assetTenantUnchanged bool,
) error {
	if current == nil || updated == nil {
		return nil
	}
	if current.Type != constant.ChannelTypeSeedanceLink && updated.Type != constant.ChannelTypeSeedanceLink {
		return nil
	}
	oldSettings, err := parsedChannelOtherSettings(current)
	if err != nil {
		return err
	}
	newSettings, err := parsedChannelOtherSettings(updated)
	if err != nil {
		return err
	}

	_, identityErr := getChannelAssetScopeIdentity(tx, current.Id)
	identityExists := identityErr == nil
	if identityErr != nil && !errors.Is(identityErr, errChannelAssetScopeIdentityMissing) {
		return identityErr
	}
	boundaryEstablished := identityExists || (current.Type == constant.ChannelTypeSeedanceLink &&
		oldSettings.AssetUpstreamProtocol != "" && oldSettings.AssetUpstreamProtocol != dto.AssetUpstreamProtocolNone)
	if !boundaryEstablished {
		return nil
	}

	if current.Type != updated.Type ||
		normalizedAssetBoundaryBaseURL(current) != normalizedAssetBoundaryBaseURL(updated) ||
		oldSettings.VideoUpstreamProtocol != newSettings.VideoUpstreamProtocol ||
		oldSettings.AssetUpstreamProtocol != newSettings.AssetUpstreamProtocol ||
		strings.TrimSpace(oldSettings.AssetProviderProject) != strings.TrimSpace(newSettings.AssetProviderProject) ||
		strings.TrimSpace(oldSettings.AssetRegion) != strings.TrimSpace(newSettings.AssetRegion) {
		return ErrAssetTenantBoundaryImmutable
	}

	credentialChanged := credential != nil ||
		(strings.TrimSpace(updated.Key) != "" && updated.Key != current.Key)
	if credentialChanged && !assetTenantUnchanged {
		return ErrAssetTenantRotationUnconfirmed
	}
	return nil
}

func normalizedAssetBoundaryBaseURL(channel *Channel) string {
	if channel == nil {
		return ""
	}
	return strings.TrimRight(strings.TrimSpace(channel.GetBaseURL()), "/")
}
