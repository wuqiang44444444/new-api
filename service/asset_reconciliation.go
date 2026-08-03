package service

import (
	"context"
	"fmt"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/dto"
	"github.com/QuantumNous/new-api/logger"
	"github.com/QuantumNous/new-api/model"
	assetadapter "github.com/QuantumNous/new-api/relay/channel/task/doubao/assets"
	"gorm.io/gorm"
)

const officialAssetReconciliationIntervalSeconds int64 = 60 * 60

var officialAssetReconciliationGroupTypes = [...]string{"AIGC", "LivenessFace"}

func ensureOfficialAssetReconciliationJobs() error {
	var channels []model.Channel
	if err := model.DB.Where("type = ? AND status = ?", constant.ChannelTypeDoubaoVideo, common.ChannelStatusEnabled).Find(&channels).Error; err != nil {
		return err
	}
	for i := range channels {
		channel := &channels[i]
		if channel.GetOtherSettings().AssetUpstreamProfile != dto.AssetUpstreamProfileOfficial {
			continue
		}
		channelID := channel.Id
		_, err := model.EnsureAssetOperationJob(model.DB, &model.AssetOperationJob{
			IdempotencyKey: fmt.Sprintf("reconcile-official-assets:%d", channelID),
			Kind:           "reconcile_official_assets",
			ChannelID:      &channelID,
		}, false)
		if err != nil {
			return err
		}
	}
	return nil
}

func processOfficialAssetReconciliation(ctx context.Context, job *model.AssetOperationJob) error {
	if job.ChannelID == nil {
		return fmt.Errorf("channel id is required")
	}
	channel, err := model.GetChannelById(*job.ChannelID, true)
	if err != nil {
		return err
	}
	settings := channel.GetOtherSettings()
	if settings.AssetUpstreamProfile != dto.AssetUpstreamProfileOfficial {
		return model.FinishAssetOperationJob(job.ID, job.LockedBy)
	}
	key, fingerprint, err := singleChannelCredential(channel)
	if err != nil {
		return err
	}
	adapter, err := assetAdapterForChannel(channel, dto.AssetUpstreamProfileOfficial, key)
	if err != nil {
		return err
	}
	reconciler, ok := adapter.(assetadapter.ReconciliationAdapter)
	if !ok {
		return fmt.Errorf("official asset adapter does not support reconciliation")
	}

	upstreamAssets, err := listAllUpstreamAssets(ctx, reconciler)
	if err != nil {
		return err
	}
	upstreamGroups, err := listAllUpstreamGroups(ctx, reconciler)
	if err != nil {
		return err
	}
	findings, orphanCount, missingCount, err := compareOfficialAssetInventory(
		channel.Id,
		fingerprint,
		string(settings.AssetUpstreamProfile),
		settings.AssetProviderProject,
		settings.AssetRegion,
		upstreamAssets,
		upstreamGroups,
	)
	if err != nil {
		return err
	}
	if err := model.DB.Transaction(func(tx *gorm.DB) error {
		return model.SaveAssetReconciliationFindings(tx, channel.Id, fingerprint, findings)
	}); err != nil {
		return err
	}
	if orphanCount > 0 || missingCount > 0 {
		logger.LogWarn(ctx, fmt.Sprintf(
			"official asset reconciliation detected inventory differences: channel_id=%d orphan_count=%d missing_count=%d",
			channel.Id,
			orphanCount,
			missingCount,
		))
	}
	return model.ScheduleNextAssetOperationJob(job.ID, job.LockedBy, officialAssetReconciliationIntervalSeconds)
}

func listAllUpstreamAssets(ctx context.Context, reconciler assetadapter.ReconciliationAdapter) (map[string]struct{}, error) {
	result := map[string]struct{}{}
	for _, groupType := range officialAssetReconciliationGroupTypes {
		groupTypeResult := map[string]struct{}{}
		complete := false
		for page := 1; page <= 1000; page++ {
			items, total, err := reconciler.ListAssets(ctx, assetadapter.AssetListRequest{GroupType: groupType, Page: page, PageSize: 100})
			if err != nil {
				return nil, err
			}
			for _, item := range items {
				if item.ResourceID != "" {
					groupTypeResult[item.ResourceID] = struct{}{}
					result[item.ResourceID] = struct{}{}
				}
			}
			if len(items) == 0 || (total > 0 && len(groupTypeResult) >= total) || len(items) < 100 {
				complete = true
				break
			}
		}
		if !complete {
			return nil, fmt.Errorf("official asset reconciliation exceeded the page limit for group type %s", groupType)
		}
	}
	return result, nil
}

func listAllUpstreamGroups(ctx context.Context, reconciler assetadapter.ReconciliationAdapter) (map[string]struct{}, error) {
	result := map[string]struct{}{}
	for _, groupType := range officialAssetReconciliationGroupTypes {
		groupTypeResult := map[string]struct{}{}
		complete := false
		for page := 1; page <= 1000; page++ {
			items, total, err := reconciler.ListGroups(ctx, assetadapter.GroupListRequest{GroupType: groupType, Page: page, PageSize: 100})
			if err != nil {
				return nil, err
			}
			for _, item := range items {
				if item.ResourceID != "" {
					groupTypeResult[item.ResourceID] = struct{}{}
					result[item.ResourceID] = struct{}{}
				}
			}
			if len(items) == 0 || (total > 0 && len(groupTypeResult) >= total) || len(items) < 100 {
				complete = true
				break
			}
		}
		if !complete {
			return nil, fmt.Errorf("official asset group reconciliation exceeded the page limit for group type %s", groupType)
		}
	}
	return result, nil
}

func compareOfficialAssetInventory(
	channelID int,
	fingerprint, profile, project, region string,
	upstreamAssets, upstreamGroups map[string]struct{},
) ([]model.AssetReconciliationFinding, int, int, error) {
	var bindings []model.AssetBinding
	if err := model.DB.Where(
		"channel_id = ? AND credential_fingerprint = ? AND upstream_profile = ? AND provider_project = ? AND region = ? AND upstream_resource_id <> ? AND status <> ?",
		channelID, fingerprint, profile, project, region, "", model.AssetBindingStatusDeleted,
	).Find(&bindings).Error; err != nil {
		return nil, 0, 0, err
	}
	var groups []model.AssetGroupBinding
	if err := model.DB.Where(
		"channel_id = ? AND credential_fingerprint = ? AND upstream_profile = ? AND provider_project = ? AND region = ? AND upstream_resource_id <> ? AND status <> ?",
		channelID, fingerprint, profile, project, region, "", model.AssetBindingStatusDeleted,
	).Find(&groups).Error; err != nil {
		return nil, 0, 0, err
	}

	boundAssets := make(map[string]struct{}, len(bindings))
	for i := range bindings {
		boundAssets[bindings[i].UpstreamResourceID] = struct{}{}
	}
	boundGroups := make(map[string]struct{}, len(groups))
	for i := range groups {
		boundGroups[groups[i].UpstreamResourceID] = struct{}{}
	}

	findings := make([]model.AssetReconciliationFinding, 0)
	orphanCount, missingCount := 0, 0
	for resourceID := range upstreamAssets {
		if _, exists := boundAssets[resourceID]; exists {
			continue
		}
		findings = append(findings, model.NewAssetReconciliationFinding(
			channelID, fingerprint, profile, project, region, "asset", resourceID, model.AssetReconciliationOrphanUpstream,
		))
		orphanCount++
	}
	for resourceID := range upstreamGroups {
		if _, exists := boundGroups[resourceID]; exists {
			continue
		}
		findings = append(findings, model.NewAssetReconciliationFinding(
			channelID, fingerprint, profile, project, region, "group", resourceID, model.AssetReconciliationOrphanUpstream,
		))
		orphanCount++
	}
	for resourceID := range boundAssets {
		if _, exists := upstreamAssets[resourceID]; exists {
			continue
		}
		findings = append(findings, model.NewAssetReconciliationFinding(
			channelID, fingerprint, profile, project, region, "asset", resourceID, model.AssetReconciliationMissingUpstream,
		))
		missingCount++
	}
	for resourceID := range boundGroups {
		if _, exists := upstreamGroups[resourceID]; exists {
			continue
		}
		findings = append(findings, model.NewAssetReconciliationFinding(
			channelID, fingerprint, profile, project, region, "group", resourceID, model.AssetReconciliationMissingUpstream,
		))
		missingCount++
	}
	return findings, orphanCount, missingCount, nil
}
