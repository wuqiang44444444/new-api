package model

import (
	"slices"

	"github.com/QuantumNous/new-api/common"
	"gorm.io/gorm"
)

func ProjectAssetStatuses(assets []Asset, now int64) error {
	if len(assets) == 0 {
		return nil
	}
	assetIDs := make([]int64, 0, len(assets))
	authorizationIDs := make([]int64, 0)
	for i := range assets {
		assetIDs = append(assetIDs, assets[i].ID)
		if assets[i].AuthorizationID != nil {
			authorizationIDs = append(authorizationIDs, *assets[i].AuthorizationID)
		}
	}
	sources, err := LoadAssetSources(assetIDs)
	if err != nil {
		return err
	}
	var bindings []AssetBinding
	if err := DB.Where("asset_id IN ? AND status <> ?", assetIDs, AssetBindingStatusDeleted).Find(&bindings).Error; err != nil {
		return err
	}
	channelIDs := make([]int, 0, len(bindings))
	seenChannelIDs := make(map[int]struct{}, len(bindings))
	for i := range bindings {
		if _, exists := seenChannelIDs[bindings[i].ChannelID]; exists {
			continue
		}
		seenChannelIDs[bindings[i].ChannelID] = struct{}{}
		channelIDs = append(channelIDs, bindings[i].ChannelID)
	}
	channelsByID := make(map[int]*Channel, len(channelIDs))
	if len(channelIDs) > 0 {
		channels, err := GetChannelsByIds(channelIDs)
		if err != nil {
			return err
		}
		for _, channel := range channels {
			channelsByID[channel.Id] = channel
		}
	}
	authorizations := make(map[int64]RealPersonAuthorization, len(authorizationIDs))
	if len(authorizationIDs) > 0 {
		var rows []RealPersonAuthorization
		if err := DB.Where("id IN ?", authorizationIDs).Find(&rows).Error; err != nil {
			return err
		}
		for _, authorization := range rows {
			authorizations[authorization.ID] = authorization
		}
	}
	activeBinding := make(map[int64]bool, len(assets))
	pendingBinding := make(map[int64]bool, len(assets))
	for i := range bindings {
		binding := &bindings[i]
		if slices.Contains([]string{AssetBindingStatusPending, AssetBindingStatusCreating, AssetBindingStatusCreateUnknown, AssetBindingStatusProcessing}, binding.Status) {
			pendingBinding[binding.AssetID] = true
		}
		current, err := assetBindingIsCurrentForChannel(binding, channelsByID[binding.ChannelID])
		if err != nil {
			return err
		}
		if current {
			activeBinding[binding.AssetID] = true
		}
	}
	for i := range assets {
		asset := &assets[i]
		if slices.Contains([]string{AssetStatusDeleting, AssetStatusDeletionFailed, AssetStatusDeleted}, asset.Status) {
			continue
		}
		authorizationActive := asset.AssetKind != AssetKindRealPerson
		if asset.AssetKind == AssetKindRealPerson && asset.AuthorizationID != nil {
			authorization, exists := authorizations[*asset.AuthorizationID]
			authorizationActive = exists && authorization.UserID == asset.UserID && authorization.AppID == asset.AppID &&
				authorization.EndUserSubjectHash != "" && authorization.EndUserSubjectHash == asset.EndUserSubjectHash &&
				authorization.Status == RealPersonAuthorizationAuthorized && authorization.RevokedAt == 0
		}
		if !authorizationActive {
			asset.Status = AssetStatusFailed
			asset.ErrorCode = "real_person_authorization_inactive"
			asset.ErrorMessage = "real-person authorization is inactive"
			continue
		}
		source, hasSource := sources[asset.ID]
		sourceAvailable := asset.AssetKind != AssetKindRealPerson && hasSource && (source.ExpiresAt == 0 || source.ExpiresAt > now)
		if sourceAvailable || activeBinding[asset.ID] {
			asset.Status = AssetStatusReady
			asset.ErrorCode = ""
			asset.ErrorMessage = ""
			continue
		}
		if pendingBinding[asset.ID] {
			asset.Status = AssetStatusProcessing
			asset.ErrorCode = ""
			asset.ErrorMessage = ""
			continue
		}
		if asset.Status == AssetStatusCreating || asset.Status == AssetStatusCreateUnknown {
			continue
		}
		asset.Status = AssetStatusFailed
		if hasSource && source.ExpiresAt != 0 && source.ExpiresAt <= now {
			asset.ErrorCode = "asset_source_expired"
			asset.ErrorMessage = "asset source expired"
		} else {
			asset.ErrorCode = "asset_unresolvable"
			asset.ErrorMessage = "asset has no current resolution path"
		}
	}
	return nil
}

func listAssetsWithProjectedStatus(query *gorm.DB, offset, limit int, filter AssetListFilter) ([]Asset, int64, error) {
	if filter.AssetKind != "" {
		query = query.Where("asset_kind = ?", filter.AssetKind)
	}
	if filter.MediaType != "" {
		query = query.Where("media_type = ?", filter.MediaType)
	}
	if filter.Name != "" {
		query = query.Where("name LIKE ?", "%"+filter.Name+"%")
	}
	if filter.Status == "" {
		var total int64
		if err := query.Count(&total).Error; err != nil {
			return nil, 0, err
		}
		var assets []Asset
		if err := query.Order("id desc").Offset(offset).Limit(limit).Find(&assets).Error; err != nil {
			return nil, 0, err
		}
		if err := ProjectAssetStatuses(assets, common.GetTimestamp()); err != nil {
			return nil, 0, err
		}
		return assets, total, nil
	}
	var assets []Asset
	if err := query.Order("id desc").Find(&assets).Error; err != nil {
		return nil, 0, err
	}
	if err := ProjectAssetStatuses(assets, common.GetTimestamp()); err != nil {
		return nil, 0, err
	}
	filtered := assets[:0]
	for i := range assets {
		if assets[i].Status == filter.Status {
			filtered = append(filtered, assets[i])
		}
	}
	total := int64(len(filtered))
	if offset >= len(filtered) {
		return []Asset{}, total, nil
	}
	end := offset + limit
	if end > len(filtered) {
		end = len(filtered)
	}
	return filtered[offset:end], total, nil
}
