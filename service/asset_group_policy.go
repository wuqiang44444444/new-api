package service

import (
	"strings"

	"github.com/QuantumNous/new-api/dto"
	"github.com/QuantumNous/new-api/model"
)

// resolveAssetGroupID applies the unified northbound group contract after the
// customer model has selected its single Seedance Channel.
func resolveAssetGroupID(channel *model.Channel, assetKind, providedGroupID string) (string, error) {
	if assetKind == model.AssetKindRealPerson {
		if strings.TrimSpace(providedGroupID) == "" {
			return "", ErrInvalidAssetRequest
		}
		return providedGroupID, nil
	}
	if assetKind != model.AssetKindGeneral || channel == nil {
		return "", ErrInvalidAssetRequest
	}

	switch channel.GetOtherSettings().AssetUpstreamProtocol.GeneralAssetGroupPolicy() {
	case dto.GeneralAssetGroupPolicyNone:
		return "", nil
	case dto.GeneralAssetGroupPolicyHosted:
		// 托管组是平台事实：显式组 ID 由托管 adapter 校验归属；未传组时
		// 直接创建无组素材，不回退任何 Provider 默认组。
		return strings.TrimSpace(providedGroupID), nil
	case dto.GeneralAssetGroupPolicyDefaultFallback:
		if strings.TrimSpace(providedGroupID) != "" {
			return providedGroupID, nil
		}
		record, err := model.GetChannelDefaultAssetGroup(channel.Id)
		if err != nil {
			return "", err
		}
		if record == nil || strings.TrimSpace(record.ProviderGroupID) == "" {
			return "", ErrDefaultAssetGroupNotConfigured
		}
		return strings.TrimSpace(record.ProviderGroupID), nil
	default:
		return "", ErrAssetUpstreamUnavailable
	}
}
