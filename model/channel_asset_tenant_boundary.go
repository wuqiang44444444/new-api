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
	ErrAssetTenantBoundaryImmutable      = errors.New("channel type cannot be changed after an asset tenant is established")
	ErrAssetTenantReplacementUnconfirmed = errors.New("confirm the asset tenant replacement before changing boundary fields")
	ErrAssetTenantRotationUnconfirmed    = errors.New("confirm that credential rotation does not change the asset tenant")
)

type AssetTenantReplacementRequiredError struct {
	ChangedFields []string
}

func (err *AssetTenantReplacementRequiredError) Error() string {
	return ErrAssetTenantReplacementUnconfirmed.Error()
}

func (err *AssetTenantReplacementRequiredError) Unwrap() error {
	return ErrAssetTenantReplacementUnconfirmed
}

func AssetTenantReplacementChangedFields(err error) []string {
	var replacementErr *AssetTenantReplacementRequiredError
	if !errors.As(err, &replacementErr) {
		return nil
	}
	return append([]string(nil), replacementErr.ChangedFields...)
}

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
	assetTenantReplacementConfirmed bool,
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

	boundaryEstablished, err := channelAssetTenantBoundaryEstablished(tx, current, oldSettings)
	if err != nil {
		return err
	}
	if !boundaryEstablished {
		return nil
	}

	if current.Type != updated.Type {
		return ErrAssetTenantBoundaryImmutable
	}
	changedFields := assetTenantBoundaryChangedFields(current, updated, oldSettings, newSettings)
	if len(changedFields) > 0 {
		if !assetTenantReplacementConfirmed {
			return &AssetTenantReplacementRequiredError{ChangedFields: changedFields}
		}
		return nil
	}

	credentialChanged := credential != nil ||
		(strings.TrimSpace(updated.Key) != "" && updated.Key != current.Key)
	if credentialChanged && !assetTenantUnchanged {
		return ErrAssetTenantRotationUnconfirmed
	}
	return nil
}

func ChannelAssetTenantBoundaryChanges(current, updated *Channel) ([]string, error) {
	if current == nil || updated == nil ||
		(current.Type != constant.ChannelTypeSeedanceLink && updated.Type != constant.ChannelTypeSeedanceLink) {
		return nil, nil
	}
	oldSettings, err := parsedChannelOtherSettings(current)
	if err != nil {
		return nil, err
	}
	newSettings, err := parsedChannelOtherSettings(updated)
	if err != nil {
		return nil, err
	}
	return assetTenantBoundaryChangedFields(current, updated, oldSettings, newSettings), nil
}

func ChannelAssetTenantBoundaryEstablished(channel *Channel) (bool, error) {
	settings, err := parsedChannelOtherSettings(channel)
	if err != nil {
		return false, err
	}
	return channelAssetTenantBoundaryEstablished(DB, channel, settings)
}

func channelAssetTenantBoundaryEstablished(
	tx *gorm.DB,
	channel *Channel,
	settings dto.ChannelOtherSettings,
) (bool, error) {
	if channel == nil || channel.Type != constant.ChannelTypeSeedanceLink {
		return false, nil
	}
	_, identityErr := getChannelAssetScopeIdentity(tx, channel.Id)
	if identityErr != nil && !errors.Is(identityErr, errChannelAssetScopeIdentityMissing) {
		return false, identityErr
	}
	return identityErr == nil ||
		(settings.AssetUpstreamProtocol != "" && settings.AssetUpstreamProtocol != dto.AssetUpstreamProtocolNone), nil
}

func assetTenantBoundaryChangedFields(
	current *Channel,
	updated *Channel,
	oldSettings dto.ChannelOtherSettings,
	newSettings dto.ChannelOtherSettings,
) []string {
	changedFields := make([]string, 0, 5)
	if normalizedAssetBoundaryBaseURL(current) != normalizedAssetBoundaryBaseURL(updated) {
		changedFields = append(changedFields, "base_url")
	}
	if oldSettings.VideoUpstreamProtocol != newSettings.VideoUpstreamProtocol {
		changedFields = append(changedFields, "video_upstream_protocol")
	}
	if oldSettings.AssetUpstreamProtocol != newSettings.AssetUpstreamProtocol {
		changedFields = append(changedFields, "asset_upstream_protocol")
	}
	if strings.TrimSpace(oldSettings.AssetProviderProject) != strings.TrimSpace(newSettings.AssetProviderProject) {
		changedFields = append(changedFields, "asset_provider_project")
	}
	if strings.TrimSpace(oldSettings.AssetRegion) != strings.TrimSpace(newSettings.AssetRegion) {
		changedFields = append(changedFields, "asset_region")
	}
	return changedFields
}

func normalizedAssetBoundaryBaseURL(channel *Channel) string {
	if channel == nil {
		return ""
	}
	return strings.TrimRight(strings.TrimSpace(channel.GetBaseURL()), "/")
}
