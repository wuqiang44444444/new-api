package service

import (
	"context"
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/dto"
	"github.com/QuantumNous/new-api/logger"
	"github.com/QuantumNous/new-api/model"
	assetadapter "github.com/QuantumNous/new-api/relay/channel/task/doubao/assets"
	"github.com/QuantumNous/new-api/setting/asset_setting"
	"gorm.io/gorm"
)

const (
	assetBindingTargetJoyCreator = "joycreator_library"
)

func AssetBindingResponse(asset *model.Asset, binding *model.AssetBinding) dto.AssetBindingResponse {
	return dto.AssetBindingResponse{ID: binding.PublicID, AssetID: asset.PublicID, Target: binding.BindingTarget, Model: binding.RequestedModel, Status: binding.Status, ErrorCode: binding.ErrorCode, Error: publicAssetBindingError(binding), CreatedAt: binding.CreatedAt, UpdatedAt: binding.UpdatedAt}
}

func selectAssetChannel(userID int, userGroup, usingGroup string, asset *model.Asset, modelName, target string) (*model.Channel, dto.AssetUpstreamProfile, error) {
	candidateGroups := assetCandidateGroups(userGroup, usingGroup)
	if asset.AssetKind == model.AssetKindRealPerson {
		if asset.AuthorizationID == nil {
			return nil, "", fmt.Errorf("%w: real-person authorization is missing", ErrRealPersonAuthorizationNotReady)
		}
		var authorization model.RealPersonAuthorization
		if err := model.DB.First(&authorization, "id = ? AND user_id = ?", *asset.AuthorizationID, userID).Error; err != nil {
			return nil, "", err
		}
		if authorization.Status != model.RealPersonAuthorizationAuthorized {
			return nil, "", fmt.Errorf("%w: real-person authorization is not active", ErrRealPersonAuthorizationNotReady)
		}
		if modelName != authorization.RequestedModel {
			return nil, "", fmt.Errorf("%w: real-person assets must use the model recorded by their authorization", ErrAssetBindingInvalidRequest)
		}
		channel, err := model.GetChannelById(authorization.ChannelID, true)
		if err != nil {
			return nil, "", err
		}
		_, fingerprint, err := singleChannelCredential(channel)
		if err != nil || fingerprint != authorization.CredentialFingerprint {
			return nil, "", fmt.Errorf("%w: real-person authorization credential is stale", ErrAssetCredentialChanged)
		}
		return channel, dto.AssetUpstreamProfile(authorization.UpstreamProfile), nil
	}
	if target == assetBindingTargetJoyCreator {
		var channels []model.Channel
		if err := model.DB.Where("type = ? AND status = ?", constant.ChannelTypeDoubaoVideo, common.ChannelStatusEnabled).Order("priority desc").Order("id desc").Find(&channels).Error; err != nil {
			return nil, "", err
		}
		for i := range channels {
			channel := &channels[i]
			if !channelAvailableToAnyGroup(channel, candidateGroups) || channel.GetOtherSettings().AssetUpstreamProfile != dto.AssetUpstreamProfileJoyCreator {
				continue
			}
			if _, _, err := singleChannelCredential(channel); err == nil {
				return channel, dto.AssetUpstreamProfileJoyCreator, nil
			}
		}
		return nil, "", fmt.Errorf("%w: no compatible single-key JoyCreator asset channel is available", ErrAssetBindingRequired)
	}

	abilities := model.GetAllEnableAbilities()
	sort.SliceStable(abilities, func(i, j int) bool {
		left, right := int64(0), int64(0)
		if abilities[i].Priority != nil {
			left = *abilities[i].Priority
		}
		if abilities[j].Priority != nil {
			right = *abilities[j].Priority
		}
		return left > right
	})
	seen := map[int]struct{}{}
	for _, ability := range abilities {
		if !containsAssetGroup(candidateGroups, ability.Group) || (modelName != "" && ability.Model != modelName) {
			continue
		}
		if _, ok := seen[ability.ChannelId]; ok {
			continue
		}
		seen[ability.ChannelId] = struct{}{}
		channel, err := model.GetChannelById(ability.ChannelId, true)
		if err != nil || channel.Status != common.ChannelStatusEnabled || channel.Type != constant.ChannelTypeDoubaoVideo {
			continue
		}
		settings := channel.GetOtherSettings()
		profile := settings.AssetUpstreamProfile
		if target == assetBindingTargetJoyCreator {
			if profile != dto.AssetUpstreamProfileJoyCreator {
				continue
			}
		} else if !profile.IsRoutable() || !assetProfileMatchesVideoProfile(profile, settings.VideoUpstreamProfile) {
			continue
		}
		if _, _, err := singleChannelCredential(channel); err != nil {
			continue
		}
		return channel, profile, nil
	}
	return nil, "", fmt.Errorf("%w: no compatible single-key asset channel is available", ErrAssetBindingRequired)
}

func channelAvailableToAnyGroup(channel *model.Channel, candidateGroups []string) bool {
	for _, group := range channel.GetGroups() {
		if containsAssetGroup(candidateGroups, group) {
			return true
		}
	}
	return false
}

func assetCandidateGroups(userGroup, usingGroup string) []string {
	if usingGroup == "auto" {
		if groups := GetUserAutoGroup(userGroup); len(groups) > 0 {
			return groups
		}
	}
	return []string{usingGroup}
}

func containsAssetGroup(groups []string, expected string) bool {
	for _, group := range groups {
		if group == expected {
			return true
		}
	}
	return false
}

func assetProfileMatchesVideoProfile(assetProfile dto.AssetUpstreamProfile, videoProfile dto.VideoUpstreamProfile) bool {
	return assetProfile == dto.AssetUpstreamProfileArk && videoProfile == dto.VideoUpstreamProfileThirdPartyReverseProxy ||
		assetProfile == dto.AssetUpstreamProfileRelay && videoProfile == dto.VideoUpstreamProfileThirdPartyRelay ||
		assetProfile == dto.AssetUpstreamProfileOfficial && (videoProfile == "" || videoProfile == dto.VideoUpstreamProfileOfficial)
}

func singleChannelCredential(channel *model.Channel) (string, string, error) {
	key, fingerprint, err := model.ResolveAssetChannelCredential(channel)
	if err != nil {
		return "", "", fmt.Errorf("%w: %v", ErrAssetBindingRequired, err)
	}
	return key, fingerprint, nil
}

func assetAdapterForChannel(channel *model.Channel, profile dto.AssetUpstreamProfile, key string) (assetadapter.Adapter, error) {
	httpClient, err := GetHttpClientWithProxy(channel.GetSetting().Proxy)
	if err != nil {
		return nil, err
	}
	switch profile {
	case dto.AssetUpstreamProfileArk:
		return assetadapter.NewArkAdapter(channel.GetBaseURL(), key, httpClient), nil
	case dto.AssetUpstreamProfileRelay:
		return assetadapter.NewRelayAdapter(channel.GetBaseURL(), key, httpClient), nil
	case dto.AssetUpstreamProfileJoyCreator:
		return assetadapter.NewJoyCreatorAdapter(channel.GetBaseURL(), key, httpClient), nil
	case dto.AssetUpstreamProfileOfficial:
		return assetadapter.NewOfficialActionAdapter(
			model.OfficialAssetActionBaseURL(channel.GetOtherSettings().AssetRegion),
			key,
			channel.GetOtherSettings().AssetRegion,
			channel.GetOtherSettings().AssetProviderProject,
			httpClient,
		)
	default:
		return nil, fmt.Errorf("%w: unsupported asset upstream profile", ErrUnsupportedAssetType)
	}
}

func publicAssetBindingError(binding *model.AssetBinding) string {
	if binding.ErrorCode == "" {
		return ""
	}
	return "upstream asset operation failed"
}

type assetOperationHandler struct{}

func (assetOperationHandler) Type() string  { return model.SystemTaskTypeAssetOperation }
func (assetOperationHandler) Enabled() bool { return true }
func (assetOperationHandler) Interval() time.Duration {
	return time.Duration(asset_setting.Current().PollIntervalSeconds) * time.Second
}
func (assetOperationHandler) NewPayload() any { return nil }

func (assetOperationHandler) Run(ctx context.Context, task *model.SystemTask, runnerID string) {
	if asset_setting.Current().Enabled {
		if err := ensureOfficialAssetReconciliationJobs(); err != nil {
			logger.LogWarn(ctx, "failed to schedule official asset reconciliation: "+err.Error())
		}
	}
	processed := 0
	for processed < 100 {
		if err := ctx.Err(); err != nil {
			failSystemTask(task, runnerID, err)
			return
		}
		config := asset_setting.Current()
		var allowedKinds []string
		if !config.Enabled {
			allowedKinds = []string{"poll_binding", "delete_binding", "delete_group", "poll_verification", "resolve_unknown_create", "resolve_unknown_group_create"}
		}
		job, err := model.ClaimNextAssetOperationJob(runnerID, config.JobLeaseSeconds, allowedKinds)
		if err != nil {
			failSystemTask(task, runnerID, err)
			return
		}
		if job == nil {
			break
		}
		if err := processAssetOperation(ctx, job); err != nil {
			logger.LogWarn(ctx, fmt.Sprintf("asset operation failed: job=%d kind=%s err=%v", job.ID, job.Kind, err))
			if retryErr := model.RetryAssetOperationJob(job, err); retryErr == nil && job.AttemptCount+1 >= job.MaxAttempts {
				markAssetOperationDead(job)
			}
		}
		processed++
	}
	if err := model.FinishSystemTask(task.TaskID, runnerID, model.SystemTaskStatusSucceeded, map[string]int{"processed": processed}, ""); err != nil {
		logSystemTaskLockError(ctx, task, err)
	}
}

func markAssetOperationDead(job *model.AssetOperationJob) {
	now := common.GetTimestamp()
	switch job.Kind {
	case "poll_binding":
		if job.BindingID != nil {
			updated := model.DB.Model(&model.AssetBinding{}).Where("id = ? AND status = ?", *job.BindingID, model.AssetBindingStatusProcessing).Updates(map[string]any{"status": model.AssetBindingStatusFailed, "error_code": "operation_exhausted", "error_message": "upstream asset operation exhausted retries", "updated_at": now})
			if updated.Error != nil {
				common.SysError(fmt.Sprintf("failed to persist exhausted asset polling: job_id=%d err=%v", job.ID, updated.Error))
				break
			}
			var binding model.AssetBinding
			if updated.RowsAffected == 1 {
				if err := model.DB.Select("asset_id").First(&binding, "id = ?", *job.BindingID).Error; err != nil {
					common.SysError(fmt.Sprintf("failed to load exhausted asset binding: job_id=%d err=%v", job.ID, err))
				} else if err := syncRemoteAssetStatus(binding.AssetID, model.AssetBindingStatusFailed, "operation_exhausted", "upstream asset operation exhausted retries"); err != nil {
					common.SysError(fmt.Sprintf("failed to persist exhausted asset status: job_id=%d err=%v", job.ID, err))
				}
			}
		}
	case "update_binding":
		if job.BindingID != nil {
			if err := model.DB.Model(&model.AssetBinding{}).Where("id = ? AND status IN ?", *job.BindingID, []string{model.AssetBindingStatusActive, model.AssetBindingStatusProcessing}).Updates(map[string]any{"error_code": "update_exhausted", "error_message": "upstream asset update exhausted retries", "updated_at": now}).Error; err != nil {
				common.SysError(fmt.Sprintf("failed to persist exhausted asset update: job_id=%d err=%v", job.ID, err))
			}
		}
	case "delete_binding":
		if job.BindingID != nil && job.AssetID != nil {
			binding := &model.AssetBinding{ID: *job.BindingID, AssetID: *job.AssetID}
			asset := &model.Asset{ID: *job.AssetID}
			if err := markAssetBindingDeletionFailed(binding, asset, "delete_exhausted", "upstream asset deletion exhausted retries"); err != nil {
				common.SysError(fmt.Sprintf("failed to persist exhausted asset deletion: job_id=%d err=%v", job.ID, err))
			}
		}
	case "delete_group":
		if job.GroupBindingID != nil {
			if err := model.DB.Model(&model.AssetGroupBinding{}).
				Where("id = ? AND status IN ?", *job.GroupBindingID, []string{model.AssetBindingStatusDeleting, model.AssetBindingStatusDeletionFailed}).
				Updates(map[string]any{"status": model.AssetBindingStatusDeletionFailed, "error_code": "delete_exhausted", "error_message": "upstream group deletion exhausted retries", "updated_at": now}).Error; err != nil {
				common.SysError(fmt.Sprintf("failed to persist exhausted asset group deletion: job_id=%d err=%v", job.ID, err))
			}
		}
	case "poll_verification":
		if job.AuthorizationID != nil {
			if err := model.DB.Model(&model.RealPersonAuthorization{}).Where("id = ? AND status IN ?", *job.AuthorizationID, []string{model.RealPersonAuthorizationAwaitingVerification, model.RealPersonAuthorizationVerifying}).Updates(map[string]any{"status": model.RealPersonAuthorizationFailed, "error_code": "real_person_verification_failed", "updated_at": now}).Error; err != nil {
				common.SysError(fmt.Sprintf("failed to persist exhausted verification poll: job_id=%d err=%v", job.ID, err))
			}
		}
	case "resolve_unknown_create":
		if err := expireDeadUnknownRemoteCreate(job); err != nil {
			common.SysError(fmt.Sprintf("failed to close dead remote asset create watchdog: job_id=%d err=%v", job.ID, err))
		}
	case "resolve_unknown_group_create":
		if err := expireDeadUnknownAssetGroupCreate(job); err != nil {
			common.SysError(fmt.Sprintf("failed to close dead automatic asset group create watchdog: job_id=%d err=%v", job.ID, err))
		}
	case "reconcile_official_assets":
		channelID := 0
		if job.ChannelID != nil {
			channelID = *job.ChannelID
		}
		common.SysError(fmt.Sprintf("official asset reconciliation exhausted retries: job_id=%d channel_id=%d", job.ID, channelID))
	}
}

func init() {
	RegisterSystemTaskHandler(assetOperationHandler{})
}

func processAssetOperation(ctx context.Context, job *model.AssetOperationJob) error {
	switch job.Kind {
	case "poll_binding", "update_binding", "delete_binding":
		return processAssetBindingOperation(ctx, job)
	case "delete_group":
		return processAssetGroupDeletion(ctx, job)
	case "poll_verification":
		return processRealPersonVerificationPoll(ctx, job)
	case "resolve_unknown_create":
		return resolveUnknownRemoteCreate(job)
	case "resolve_unknown_group_create":
		return resolveUnknownAssetGroupCreate(job)
	case "reconcile_official_assets":
		return processOfficialAssetReconciliation(ctx, job)
	default:
		return fmt.Errorf("unknown asset operation %q", job.Kind)
	}
}

func processRealPersonVerificationPoll(ctx context.Context, job *model.AssetOperationJob) error {
	if job.AuthorizationID == nil {
		return fmt.Errorf("authorization id is required")
	}
	var authorization model.RealPersonAuthorization
	if err := model.DB.First(&authorization, "id = ?", *job.AuthorizationID).Error; err != nil {
		return err
	}
	if realPersonVerificationTerminal(authorization.Status) {
		return model.FinishAssetOperationJob(job.ID, job.LockedBy)
	}
	if err := RefreshRealPersonVerification(ctx, &authorization); err != nil {
		return err
	}
	if realPersonVerificationTerminal(authorization.Status) {
		return model.FinishAssetOperationJob(job.ID, job.LockedBy)
	}
	if job.AttemptCount+1 >= job.MaxAttempts {
		return expireRealPersonVerificationPoll(job, &authorization)
	}
	return model.RescheduleAssetOperationJob(job.ID, job.LockedBy, "poll_verification", asset_setting.Current().PollIntervalSeconds)
}

func processAssetBindingOperation(ctx context.Context, job *model.AssetOperationJob) error {
	if job.BindingID == nil {
		return fmt.Errorf("binding id is required")
	}
	var binding model.AssetBinding
	if err := model.DB.First(&binding, "id = ?", *job.BindingID).Error; err != nil {
		return err
	}
	var asset model.Asset
	if err := model.DB.First(&asset, "id = ?", binding.AssetID).Error; err != nil {
		return err
	}
	if job.Kind == "delete_binding" && binding.UpstreamResourceID == "" {
		unresolved, err := model.AssetOperationJobUnresolved(remoteCreateWatchdogKey(binding.ID))
		if err != nil {
			return err
		}
		if unresolved {
			return fmt.Errorf("remote asset creation is still unresolved")
		}
		return completeAssetBindingDeletion(job, &binding, &asset)
	}
	channel, err := model.GetChannelById(binding.ChannelID, true)
	if err != nil {
		return err
	}
	key, fingerprint, err := singleChannelCredential(channel)
	if err != nil || fingerprint != binding.CredentialFingerprint {
		if err == nil {
			err = fmt.Errorf("channel credential changed")
		}
		if job.Kind == "delete_binding" {
			if persistErr := markAssetBindingDeletionFailed(&binding, &asset, "stale_credential", "channel credential changed"); persistErr != nil {
				return persistErr
			}
			return err
		}
		updated := model.DB.Model(&model.AssetBinding{}).Where("id = ? AND status IN ?", binding.ID, []string{model.AssetBindingStatusProcessing, model.AssetBindingStatusActive}).Updates(map[string]any{"status": model.AssetBindingStatusStaleCredential, "error_code": "stale_credential", "error_message": "channel credential changed", "updated_at": common.GetTimestamp()})
		if updated.Error != nil {
			return updated.Error
		}
		if updated.RowsAffected == 0 {
			return model.FinishAssetOperationJob(job.ID, job.LockedBy)
		}
		if syncErr := syncRemoteAssetStatus(binding.AssetID, model.AssetBindingStatusStaleCredential, "stale_credential", "channel credential changed"); syncErr != nil {
			return syncErr
		}
		return err
	}
	adapter, err := assetAdapterForChannel(channel, dto.AssetUpstreamProfile(binding.UpstreamProfile), key)
	if err != nil {
		return err
	}

	switch job.Kind {
	case "poll_binding":
		result, err := adapter.GetAsset(ctx, binding.UpstreamResourceID)
		if err != nil {
			return err
		}
		return saveAssetBindingResult(job, &asset, &binding, result)
	case "update_binding":
		result, err := adapter.UpdateAsset(ctx, binding.UpstreamResourceID, asset.Name)
		if err != nil {
			return err
		}
		if result.ResourceID == "" {
			result.ResourceID = binding.UpstreamResourceID
		}
		return saveAssetBindingResult(job, &asset, &binding, result)
	case "delete_binding":
		if err := adapter.DeleteAsset(ctx, binding.UpstreamResourceID); err != nil {
			if persistErr := markAssetBindingDeletionFailed(&binding, &asset, "upstream_delete_failed", "upstream deletion failed"); persistErr != nil {
				return persistErr
			}
			return err
		}
		return completeAssetBindingDeletion(job, &binding, &asset)
	}
	return nil
}

func ensureAssetGroup(ctx context.Context, asset *model.Asset, binding *model.AssetBinding, adapter assetadapter.Adapter) (string, error) {
	groupAdapter, ok := adapter.(assetadapter.GroupAdapter)
	if !ok {
		return "", nil
	}
	if binding.UpstreamGroupBindingID != nil {
		var managedGroup model.AssetGroupBinding
		if err := model.DB.First(&managedGroup, "id = ? AND user_id = ? AND channel_id = ? AND credential_fingerprint = ?", *binding.UpstreamGroupBindingID, asset.UserID, binding.ChannelID, binding.CredentialFingerprint).Error; err != nil {
			return "", err
		}
		if managedGroup.Status != model.AssetBindingStatusActive || managedGroup.UpstreamResourceID == "" {
			return "", fmt.Errorf("managed asset group is not active")
		}
		return managedGroup.UpstreamResourceID, nil
	}

	groupKind := "general_aigc"
	var authorizationID *int64
	if asset.AssetKind == model.AssetKindRealPerson {
		groupKind = "real_person"
		authorizationID = asset.AuthorizationID
	}
	scopeKey := model.AssetScopeKey(asset.UserID, authorizationID)
	candidate := &model.AssetGroupBinding{
		UserID: asset.UserID, AuthorizationID: authorizationID, ScopeKey: scopeKey,
		ChannelID: binding.ChannelID, CredentialFingerprint: binding.CredentialFingerprint,
		UpstreamProfile: binding.UpstreamProfile, ProviderProject: binding.ProviderProject,
		Region: binding.Region, GroupKind: groupKind,
		Name: "NEWAPI " + groupKind, Description: "Managed asset group", Status: model.AssetBindingStatusPending,
	}
	group, owned, err := prepareAutomaticAssetGroupCreate(asset, candidate)
	if err != nil {
		return "", err
	}
	if group == nil {
		return "", fmt.Errorf("upstream asset group is unavailable")
	}
	if owned {
		return createAutomaticAssetGroup(ctx, groupAdapter, group, binding)
	}
	if group.Status == model.AssetBindingStatusActive && group.UpstreamResourceID != "" {
		binding.UpstreamGroupBindingID = &group.ID
		return group.UpstreamResourceID, model.DB.Model(binding).Update("upstream_group_binding_id", group.ID).Error
	}
	if group.Status == model.AssetBindingStatusDeleting || group.Status == model.AssetBindingStatusDeletionFailed || group.Status == model.AssetBindingStatusDeleted {
		return "", fmt.Errorf("upstream asset group is being deleted")
	}
	if group.UpstreamResourceID == "" {
		if group.Status == model.AssetBindingStatusCreateUnknown {
			return "", errAssetGroupCreateOutcomeUnknown
		}
		if group.Status == model.AssetBindingStatusFailed && group.ErrorCode == assetGroupCreationOutcomeUnknownCode {
			return "", fmt.Errorf("upstream asset group creation outcome could not be confirmed")
		}
		return "", errAssetGroupCreateOutcomeUnknown
	}
	result, err := groupAdapter.GetGroup(ctx, group.UpstreamResourceID)
	if err != nil {
		return "", err
	}
	if result.ResourceID == "" {
		result.ResourceID = group.UpstreamResourceID
	}
	usable, err := saveAutomaticAssetGroupResult(group, result)
	if err != nil {
		return "", err
	}
	if !usable {
		return "", fmt.Errorf("upstream asset group is being deleted")
	}
	if group.Status != model.AssetBindingStatusActive {
		if group.Status == model.AssetBindingStatusFailed {
			return "", fmt.Errorf("upstream asset group failed")
		}
		return "", errAssetGroupCreateOutcomeUnknown
	}
	binding.UpstreamGroupBindingID = &group.ID
	return group.UpstreamResourceID, model.DB.Model(binding).Update("upstream_group_binding_id", group.ID).Error
}

func saveAssetBindingResult(job *model.AssetOperationJob, asset *model.Asset, binding *model.AssetBinding, result assetadapter.AssetResult) error {
	var savedAsset *model.Asset
	var savedBinding *model.AssetBinding
	err := model.DB.Transaction(func(tx *gorm.DB) error {
		currentAsset, currentBinding, authorizationActive, err := lockRemoteCreateState(tx, asset, binding)
		if err != nil {
			return err
		}
		pollable := job.Kind == "poll_binding" && currentAsset.Status == model.AssetStatusProcessing && currentBinding.Status == model.AssetBindingStatusProcessing
		updatable := job.Kind == "update_binding" &&
			(currentAsset.Status == model.AssetStatusReady || currentAsset.Status == model.AssetStatusProcessing) &&
			(currentBinding.Status == model.AssetBindingStatusActive || currentBinding.Status == model.AssetBindingStatusProcessing)
		if !authorizationActive || (!pollable && !updatable) {
			if err := model.FinishAssetOperationJobTx(tx, job.ID, job.LockedBy); err != nil {
				return err
			}
			savedAsset, savedBinding = currentAsset, currentBinding
			return nil
		}

		resourceID := result.ResourceID
		if resourceID == "" {
			resourceID = currentBinding.UpstreamResourceID
		}
		businessID := result.BusinessID
		if businessID == "" {
			businessID = currentBinding.UpstreamBusinessID
		}
		requestID := result.RequestID
		if requestID == "" {
			requestID = currentBinding.UpstreamRequestID
		}
		referenceType := result.ReferenceType
		if referenceType == "" {
			referenceType = currentBinding.UpstreamReferenceType
		}
		referenceValue := result.ReferenceValue
		if referenceValue == "" {
			referenceValue = currentBinding.UpstreamReferenceValue
		}

		status := model.AssetBindingStatusProcessing
		assetStatus := model.AssetStatusProcessing
		if result.Status == "active" {
			managementOnly := dto.AssetUpstreamProfile(currentBinding.UpstreamProfile) == dto.AssetUpstreamProfileJoyCreator
			if resourceID == "" || (!managementOnly && (referenceType != "asset_uri_id" || referenceValue == "")) {
				status = model.AssetBindingStatusFailed
				assetStatus = model.AssetStatusFailed
				result.ErrorCode = "invalid_upstream_asset"
				result.ErrorMessage = "upstream asset response is incomplete"
			} else {
				status = model.AssetBindingStatusActive
				assetStatus = model.AssetStatusReady
			}
		} else if result.Status == "failed" {
			status = model.AssetBindingStatusFailed
			assetStatus = model.AssetStatusFailed
		}
		if status == model.AssetBindingStatusProcessing && resourceID == "" {
			status = model.AssetBindingStatusFailed
			assetStatus = model.AssetStatusFailed
			result.ErrorCode = "invalid_upstream_asset"
			result.ErrorMessage = "upstream asset response is incomplete"
		} else if status == model.AssetBindingStatusProcessing && job.AttemptCount+1 >= job.MaxAttempts {
			status = model.AssetBindingStatusFailed
			assetStatus = model.AssetStatusFailed
			result.ErrorCode = "poll_exhausted"
			result.ErrorMessage = "upstream processing timeout"
		}
		errorMessage := publicAssetUpstreamError(result.ErrorMessage)
		now := common.GetTimestamp()
		if err := tx.Model(currentBinding).Updates(map[string]any{
			"upstream_resource_id": resourceID, "upstream_business_id": businessID,
			"upstream_request_id": requestID, "upstream_reference_type": referenceType,
			"upstream_reference_value": referenceValue, "status": status, "updated_at": now,
			"error_code": result.ErrorCode, "error_message": errorMessage,
		}).Error; err != nil {
			return err
		}
		if err := tx.Model(currentAsset).Updates(map[string]any{
			"status": assetStatus, "error_code": result.ErrorCode, "error_message": errorMessage, "updated_at": now,
		}).Error; err != nil {
			return err
		}
		if err := updateAssetCreateIdempotency(tx, currentAsset.ID, assetStatus); err != nil {
			return err
		}
		if status == model.AssetBindingStatusProcessing {
			if err := model.RescheduleAssetOperationJobTx(tx, job.ID, job.LockedBy, "poll_binding", asset_setting.Current().PollIntervalSeconds); err != nil {
				return err
			}
		} else if err := model.FinishAssetOperationJobTx(tx, job.ID, job.LockedBy); err != nil {
			return err
		}

		currentAsset.Status = assetStatus
		currentAsset.ErrorCode = result.ErrorCode
		currentAsset.ErrorMessage = errorMessage
		currentBinding.UpstreamResourceID = resourceID
		currentBinding.UpstreamBusinessID = businessID
		currentBinding.UpstreamRequestID = requestID
		currentBinding.UpstreamReferenceType = referenceType
		currentBinding.UpstreamReferenceValue = referenceValue
		currentBinding.Status = status
		currentBinding.ErrorCode = result.ErrorCode
		currentBinding.ErrorMessage = errorMessage
		savedAsset, savedBinding = currentAsset, currentBinding
		return nil
	})
	if err == nil && savedAsset != nil && savedBinding != nil {
		*asset = *savedAsset
		*binding = *savedBinding
	}
	return err
}

func processAssetGroupDeletion(ctx context.Context, job *model.AssetOperationJob) error {
	if job.GroupBindingID == nil {
		return fmt.Errorf("group binding id is required")
	}
	var group model.AssetGroupBinding
	if err := model.DB.First(&group, "id = ?", *job.GroupBindingID).Error; err != nil {
		return err
	}
	if group.Status == model.AssetBindingStatusDeleted {
		return model.FinishAssetOperationJob(job.ID, job.LockedBy)
	}
	if group.UpstreamResourceID == "" {
		unresolved, err := model.AssetOperationJobUnresolved(automaticAssetGroupWatchdogKey(group.ID))
		if err != nil {
			return err
		}
		if unresolved {
			return fmt.Errorf("automatic asset group creation is still unresolved")
		}
		return completeAssetGroupDeletion(job, &group)
	}
	channel, err := model.GetChannelById(group.ChannelID, true)
	if err != nil {
		return err
	}
	key, fingerprint, err := singleChannelCredential(channel)
	if err != nil || fingerprint != group.CredentialFingerprint {
		return fmt.Errorf("group channel credential changed")
	}
	adapter, err := assetAdapterForChannel(channel, dto.AssetUpstreamProfile(group.UpstreamProfile), key)
	if err != nil {
		return err
	}
	groupAdapter, ok := adapter.(assetadapter.GroupAdapter)
	if !ok {
		return fmt.Errorf("asset protocol does not support group deletion")
	}
	if err := groupAdapter.DeleteGroup(ctx, group.UpstreamResourceID); err != nil {
		persistErr := model.DB.Model(&model.AssetGroupBinding{}).
			Where("id = ? AND status IN ? AND upstream_resource_id = ?", group.ID, []string{model.AssetBindingStatusDeleting, model.AssetBindingStatusDeletionFailed}, group.UpstreamResourceID).
			Updates(map[string]any{"status": model.AssetBindingStatusDeletionFailed, "error_code": "upstream_group_delete_failed", "error_message": "upstream group deletion failed", "updated_at": common.GetTimestamp()}).Error
		if persistErr != nil {
			common.SysError(fmt.Sprintf("failed to persist asset group deletion failure: job_id=%d group_id=%d err=%v", job.ID, group.ID, persistErr))
			return persistErr
		}
		return err
	}
	return completeAssetGroupDeletion(job, &group)
}

func publicAssetUpstreamError(message string) string {
	if strings.TrimSpace(message) == "" {
		return ""
	}
	return "upstream asset operation failed"
}
