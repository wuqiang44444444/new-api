package service

import (
	"context"
	"strings"
	"time"

	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/dto"
	"github.com/QuantumNous/new-api/model"
	assetadapter "github.com/QuantumNous/new-api/relay/channel/task/seedance/assets"
)

const DefaultAssetGroupName = "aigctokenaigeneral"

const (
	DefaultAssetGroupActionCreated = "created"
	DefaultAssetGroupActionReused  = "reused"
	defaultAssetGroupPageSize      = 100
	defaultAssetGroupMaxPages      = 100
)

type ChannelDefaultAssetGroupStatus struct {
	Supported  bool   `json:"supported"`
	Configured bool   `json:"configured"`
	Name       string `json:"name"`
}

type ChannelDefaultAssetGroupResult struct {
	ChannelDefaultAssetGroupStatus
	Action string `json:"action"`
}

func GetChannelDefaultAssetGroupStatus(channel *model.Channel) (ChannelDefaultAssetGroupStatus, error) {
	status := ChannelDefaultAssetGroupStatus{Name: DefaultAssetGroupName}
	if channel == nil || channel.Type != constant.ChannelTypeSeedanceLink {
		return status, ErrAssetUpstreamUnavailable
	}
	// 仅明确的"无素材协议"直接表达不适用。其余协议必须经同一次固定声明与
	// adapter 判定；声明缺失时明确失败，不伪装为"配置正常但不需要默认组"。
	if settings := channel.GetOtherSettings(); settings.AssetUpstreamProtocol == "" || settings.AssetUpstreamProtocol == dto.AssetUpstreamProtocolNone {
		return status, nil
	}
	adapter, err := seedanceAssetAdapter(channel, 0, "")
	if err != nil {
		return status, err
	}
	policy, err := seedanceAssetGroupPolicy(adapter)
	if err != nil {
		return status, err
	}
	if policy != dto.GeneralAssetGroupPolicyDefaultFallback {
		return status, nil
	}
	if _, ok := adapter.(assetadapter.GroupAdapter); !ok {
		return status, nil
	}
	status.Supported = true
	record, err := model.GetChannelDefaultAssetGroup(channel.Id)
	if err != nil {
		return status, err
	}
	status.Configured = record != nil && strings.TrimSpace(record.ProviderGroupID) != ""
	return status, nil
}

func CreateOrReuseChannelDefaultAssetGroup(ctx context.Context, channel *model.Channel) (ChannelDefaultAssetGroupResult, error) {
	status := ChannelDefaultAssetGroupStatus{Name: DefaultAssetGroupName}
	adapter, err := seedanceAssetAdapter(channel, 0, "")
	if err != nil {
		return ChannelDefaultAssetGroupResult{ChannelDefaultAssetGroupStatus: status}, err
	}
	policy, err := seedanceAssetGroupPolicy(adapter)
	if err != nil {
		return ChannelDefaultAssetGroupResult{ChannelDefaultAssetGroupStatus: status}, err
	}
	if policy != dto.GeneralAssetGroupPolicyDefaultFallback {
		return ChannelDefaultAssetGroupResult{ChannelDefaultAssetGroupStatus: status}, ErrUnsupportedAssetOperation
	}
	groupAdapter, ok := adapter.(assetadapter.GroupAdapter)
	if !ok {
		return ChannelDefaultAssetGroupResult{ChannelDefaultAssetGroupStatus: status}, ErrUnsupportedAssetOperation
	}
	status.Supported = true
	providerGroupID, action, err := createOrReuseDefaultAssetGroup(ctx, channel, groupAdapter)
	if err != nil {
		return ChannelDefaultAssetGroupResult{ChannelDefaultAssetGroupStatus: status}, err
	}
	if err := model.SaveChannelDefaultAssetGroup(channel.Id, providerGroupID); err != nil {
		return ChannelDefaultAssetGroupResult{ChannelDefaultAssetGroupStatus: status}, err
	}
	status.Configured = true
	return ChannelDefaultAssetGroupResult{ChannelDefaultAssetGroupStatus: status, Action: action}, nil
}

func createOrReuseDefaultAssetGroup(ctx context.Context, channel *model.Channel, adapter assetadapter.GroupAdapter) (string, string, error) {
	startedAt := time.Now()
	search, canSearch := adapter.(assetadapter.GroupSearchAdapter)
	if declared, ok := adapter.(interface{ CanSearchGroups() bool }); ok {
		canSearch = canSearch && declared.CanSearchGroups()
	}
	if canSearch {
		seen := 0
		for page := 1; page <= defaultAssetGroupMaxPages; page++ {
			items, total, err := search.ListGroups(ctx, assetadapter.GroupListRequest{
				GroupType: "AIGC",
				Name:      DefaultAssetGroupName,
				Page:      page,
				PageSize:  defaultAssetGroupPageSize,
			})
			if err != nil {
				return "", "", normalizeAssetAdapterError(ctx, "default_asset_group", "", channel, time.Since(startedAt), err)
			}
			for _, item := range items {
				name := strings.TrimSpace(item.Name)
				if name == "" {
					return "", "", ErrAssetUpstreamError
				}
				if name == DefaultAssetGroupName {
					if strings.TrimSpace(item.ResourceID) == "" {
						return "", "", ErrAssetUpstreamError
					}
					return item.ResourceID, DefaultAssetGroupActionReused, nil
				}
			}
			seen += len(items)
			if total < seen {
				return "", "", ErrAssetUpstreamError
			}
			if seen >= total {
				break
			}
			if len(items) == 0 || len(items) < defaultAssetGroupPageSize {
				return "", "", ErrAssetUpstreamError
			}
			if page == defaultAssetGroupMaxPages {
				return "", "", ErrAssetUpstreamError
			}
		}
	}

	result, err := adapter.CreateGroup(ctx, assetadapter.GroupRequest{
		Name:      DefaultAssetGroupName,
		GroupType: "AIGC",
	})
	if err != nil {
		return "", "", normalizeAssetAdapterError(ctx, "default_asset_group", "", channel, time.Since(startedAt), err)
	}
	if strings.TrimSpace(result.ResourceID) == "" {
		return "", "", ErrAssetUpstreamError
	}
	return result.ResourceID, DefaultAssetGroupActionCreated, nil
}
