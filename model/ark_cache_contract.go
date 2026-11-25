package model

import (
	"errors"
	"slices"
	"strings"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/dto"
	"github.com/QuantumNous/new-api/setting"
	"github.com/QuantumNous/new-api/setting/ratio_setting"
	"gorm.io/gorm"
)

// FindArkCacheChannel reads the private execution source. A channel with any
// ordinary model or Ability does not qualify as an isolated cache channel.
func FindArkCacheChannel(tx *gorm.DB, id int) (*Channel, error) {
	var channel Channel
	if err := tx.Take(&channel, id).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, nil
		}
		return nil, err
	}
	if strings.TrimSpace(channel.Models) != "" || !arkCacheTransportCompatible(&channel) {
		return nil, nil
	}
	var abilities []Ability
	if err := tx.Select("channel_id").Where("channel_id = ? AND model <> ''", id).Limit(1).Find(&abilities).Error; err != nil || len(abilities) != 0 {
		return nil, err
	}
	return &channel, nil
}

func arkCacheTransportCompatible(channel *Channel) bool {
	if channel.Type != constant.ChannelTypeVolcEngine || channel.ChannelInfo.IsMultiKey ||
		strings.TrimSpace(channel.Key) == "" || strings.TrimRight(channel.GetBaseURL(), "/") != setting.ArkCacheBaseURL {
		return false
	}
	var execution dto.ChannelSettings
	var storage dto.ChannelOtherSettings
	if channel.Setting != nil && *channel.Setting != "" && common.UnmarshalJsonStr(*channel.Setting, &execution) != nil ||
		channel.OtherSettings != "" && common.UnmarshalJsonStr(channel.OtherSettings, &storage) != nil || execution.Proxy != "" || storage.DisableStore {
		return false
	}
	for _, override := range []*string{channel.HeaderOverride, channel.ParamOverride} {
		var fields map[string]any
		if override != nil && *override != "" && (common.UnmarshalJsonStr(*override, &fields) != nil || len(fields) != 0) {
			return false
		}
	}
	return true
}

func ArkCacheContractRouteAllowed(pilot *setting.ArkPrefixCachePilot, channel *Channel, rule ContractEntityRule) bool {
	return pilot != nil && (pilot.Mode == "observe" || pilot.ChargesSessionStorage()) && channel != nil && channel.Id == pilot.ChannelID &&
		channel.Status == common.ChannelStatusEnabled &&
		rule.ChannelId == pilot.ChannelID && rule.PublicModel == pilot.Model &&
		ratio_setting.ContainsGroupRatio(rule.RouteGroup) && slices.Contains(channel.GetGroups(), rule.RouteGroup) &&
		(!pilot.NativeChannel || IsChannelEnabledForGroupModel(rule.RouteGroup, rule.PublicModel, channel.Id))
}
