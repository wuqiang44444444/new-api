package service

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/dto"
	"github.com/QuantumNous/new-api/model"
	assetadapter "github.com/QuantumNous/new-api/relay/channel/task/doubao/assets"
	"github.com/QuantumNous/new-api/setting/asset_setting"
	"gorm.io/gorm"
)

const remoteAssetCreateEndpoint = "/v1/assets"

func CreateRemoteAsset(ctx context.Context, userID, tokenID int, userGroup, usingGroup, idempotencyKey string, req dto.CreateAssetRequest) (*model.Asset, error) {
	if normalizedTarget, ok := dto.NormalizeAssetTarget(req.Target); ok {
		req.Target = normalizedTarget
	}
	return createRemoteAsset(ctx, userID, tokenID, userGroup, usingGroup, idempotencyKey, remoteAssetCreateEndpoint, req, req, nil)
}

type assetMigrationMetadata struct {
	SupersedesAssetID *int64
	BatchID           string
	Reason            string
}

func createRemoteAsset(ctx context.Context, userID, tokenID int, userGroup, usingGroup, idempotencyKey, endpoint string, idempotencyRequest any, req dto.CreateAssetRequest, migration *assetMigrationMetadata) (*model.Asset, error) {
	req.Name = strings.TrimSpace(req.Name)
	req.AssetKind = strings.TrimSpace(req.AssetKind)
	req.MediaType = strings.TrimSpace(req.MediaType)
	req.AuthorizationID = strings.TrimSpace(req.AuthorizationID)
	req.Model = strings.TrimSpace(req.Model)
	req.Target = strings.TrimSpace(req.Target)
	req.Source.Type = strings.TrimSpace(req.Source.Type)
	if normalizedTarget, ok := dto.NormalizeAssetTarget(req.Target); ok {
		req.Target = normalizedTarget
	} else {
		return nil, fmt.Errorf("%w: unsupported binding target", ErrUnsupportedAssetBindingTarget)
	}
	if req.Source.Type != "url" {
		return nil, ErrAssetURLRequired
	}
	remoteURL, err := normalizeRemoteAssetURL(req.Source.URL)
	if err != nil {
		return nil, err
	}
	req.Source.URL = remoteURL
	switch request := idempotencyRequest.(type) {
	case dto.CreateAssetRequest:
		request = req
		idempotencyRequest = request
	case dto.MigrateAssetRequest:
		request.Name = req.Name
		request.AuthorizationID = req.AuthorizationID
		request.Model = req.Model
		request.Target = req.Target
		request.Source = req.Source
		idempotencyRequest = request
	}

	rawIdempotencyKey := strings.TrimSpace(idempotencyKey)
	var idempotency *model.AssetCreateIdempotency
	if rawIdempotencyKey != "" {
		idempotency, err = remoteAssetIdempotency(userID, tokenID, rawIdempotencyKey, endpoint, idempotencyRequest)
		if err != nil {
			return nil, err
		}
		existingAsset, _, replay, err := model.LoadRemoteAssetCreateReplay(idempotency)
		if errors.Is(err, model.ErrAssetIdempotencyConflict) {
			return nil, ErrIdempotencyConflict
		}
		if err == nil && replay {
			return existingAsset, nil
		}
		if err != nil && !errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, err
		}
	}

	config := asset_setting.Current()
	if !config.Enabled {
		return nil, fmt.Errorf("%w: remote asset creation is disabled", ErrAssetLibraryUnavailable)
	}
	if req.Name == "" || len([]rune(req.Name)) > 64 {
		return nil, fmt.Errorf("%w: asset name must contain 1 to 64 characters", ErrInvalidAssetRequest)
	}
	if !model.ValidateAssetKind(req.AssetKind) || !model.ValidateAssetMediaType(req.MediaType) {
		return nil, fmt.Errorf("%w: unsupported asset kind or media type", ErrInvalidAssetRequest)
	}
	if req.Model != "" && req.Target != "" {
		return nil, fmt.Errorf("%w: model and target are mutually exclusive", ErrAssetBindingInvalidRequest)
	}
	if req.AssetKind == model.AssetKindRealPerson {
		if req.MediaType != "image" {
			return nil, fmt.Errorf("%w: real-person remote assets must be images", ErrAssetBindingInvalidRequest)
		}
		if !config.RealPersonEnabled {
			return nil, fmt.Errorf("%w: real-person asset service is disabled", ErrAssetLibraryUnavailable)
		}
	}
	publication, sourceMinimumTTL, err := resolveAssetLinkPublication(req.Model, req.AssetKind, req.MediaType)
	if err != nil {
		return nil, err
	}

	asset := &model.Asset{
		UserID: userID, CreatedByTokenID: tokenID, AppID: tokenID, Name: req.Name,
		AssetKind: req.AssetKind, MediaType: req.MediaType,
		RequestedModel: req.Model,
		Status:         model.AssetStatusCreating,
	}
	if publication != nil {
		asset.LinkContractNamespace = publication.ContractNamespace
		asset.LinkRouteFamily = string(publication.RouteFamily)
		asset.PublishedLinkContractSKU = publication.LinkSKU
		asset.LinkPublicationVersion = publication.PublicationVersion
	}
	if migration != nil {
		asset.SupersedesAssetID = migration.SupersedesAssetID
		asset.MigrationBatchID = migration.BatchID
		asset.MigrationReason = migration.Reason
	}
	if req.AssetKind == model.AssetKindRealPerson {
		authorization, err := model.GetRealPersonAuthorizationForApp(userID, tokenID, req.AuthorizationID)
		if err != nil {
			return nil, err
		}
		if authorization != nil && authorization.ErrorCode == "real_person_verification_rejected" {
			return nil, ErrRealPersonVerificationRejected
		}
		if authorization == nil || authorization.Status != model.RealPersonAuthorizationAuthorized {
			return nil, ErrRealPersonAuthorizationNotReady
		}
		asset.AuthorizationID = &authorization.ID
		asset.EndUserSubjectHash = authorization.EndUserSubjectHash
		if asset.EndUserSubjectHash == "" {
			return nil, ErrRealPersonAuthorizationNotReady
		}
	}
	remoteURL, err = validateRemoteAssetURL(remoteURL, config.RemoteURLMaxLength)
	if err != nil {
		return nil, err
	}
	if (req.Model == "" && req.Target == "") || (publication != nil && sourceMinimumTTL > 0 && req.AssetKind != model.AssetKindRealPerson) {
		if err := validateRemoteAssetTTL(req.Source.ExpiresAt, sourceMinimumTTL, time.Now()); err != nil {
			return nil, err
		}
		asset.Status = model.AssetStatusReady
		asset, _, replay, err := model.CreateRemoteAssetWithQuota(asset, remoteURL, req.Source.ExpiresAt, nil, idempotency, config.MaxAssetsPerUser, config.CreateUnknownTTLSeconds)
		if errors.Is(err, model.ErrAssetIdempotencyConflict) {
			return nil, ErrIdempotencyConflict
		}
		if errors.Is(err, model.ErrAssetAuthorizationNotAuthorized) {
			return nil, ErrRealPersonAuthorizationNotReady
		}
		if err != nil {
			return nil, err
		}
		if replay {
			return asset, nil
		}
		return asset, nil
	}

	channel, profile, err := selectAssetChannel(userID, userGroup, usingGroup, asset, req.Model, req.Target)
	if err != nil {
		return nil, err
	}
	minimumTTL := channel.GetOtherSettings().AssetMinURLTTLSeconds
	if minimumTTL <= 0 {
		return nil, fmt.Errorf("%w: selected Provider has no verified URL fetch window", ErrAssetUpstreamUnavailable)
	}
	if err := validateRemoteAssetTTL(req.Source.ExpiresAt, minimumTTL, time.Now()); err != nil {
		return nil, err
	}
	key, fingerprint, err := singleChannelCredential(channel)
	if err != nil {
		return nil, err
	}
	adapter, err := assetAdapterForChannel(channel, profile, key)
	if err != nil {
		return nil, fmt.Errorf("%w: selected Provider adapter is unavailable", ErrAssetUpstreamUnavailable)
	}
	if !adapter.Supports(asset.AssetKind, asset.MediaType) {
		return nil, fmt.Errorf("%w: selected asset protocol does not support this asset", ErrUnsupportedAssetType)
	}

	implementation, ok := model.ResolveChannelLinkImplementation(channel)
	if publication != nil {
		if !ok || model.ValidateChannelLinkExecution(channel, publication.CustomerModel, publication.RouteFamily, publication.LinkSKU) != nil {
			return nil, fmt.Errorf("%w: selected Provider does not implement the published Link contract", ErrAssetUpstreamUnavailable)
		}
	} else if model.IsRegisteredLinkSKU(req.Model) && !ok {
		return nil, fmt.Errorf("%w: selected Provider has no registered Link implementation", ErrAssetUpstreamUnavailable)
	}
	binding := &model.AssetBinding{
		UserID: userID, ChannelID: channel.Id, CredentialFingerprint: fingerprint,
		UpstreamProfile: string(profile), ProviderProject: channel.GetOtherSettings().AssetProviderProject,
		Region: channel.GetOtherSettings().AssetRegion, RequestedModel: req.Model, BindingTarget: req.Target,
		Status: model.AssetBindingStatusCreating,
	}
	if publication != nil {
		binding.LinkContractNamespace = publication.ContractNamespace
		binding.LinkRouteFamily = string(publication.RouteFamily)
		binding.PublishedLinkContractSKU = publication.LinkSKU
		binding.LinkPublicationVersion = publication.PublicationVersion
	}
	if ok {
		binding.LinkImplementationID = implementation.ID
		binding.LinkImplementationVer = implementation.Version
		binding.LinkImplementationHash = implementation.ContentHash
	}
	if err := preflightAutomaticAssetGroupCreate(asset, binding, adapter); err != nil {
		return nil, err
	}
	asset, binding, replay, err := model.CreateRemoteAssetWithQuota(asset, remoteURL, req.Source.ExpiresAt, binding, idempotency, config.MaxAssetsPerUser, config.CreateUnknownTTLSeconds)
	if errors.Is(err, model.ErrAssetIdempotencyConflict) {
		return nil, ErrIdempotencyConflict
	}
	if errors.Is(err, model.ErrAssetAuthorizationNotAuthorized) {
		if asset.AuthorizationID != nil {
			var authorization model.RealPersonAuthorization
			if lookupErr := model.DB.Select("error_code").First(&authorization, "id = ?", *asset.AuthorizationID).Error; lookupErr == nil && authorization.ErrorCode == "real_person_verification_rejected" {
				return nil, ErrRealPersonVerificationRejected
			}
		}
		return nil, ErrRealPersonAuthorizationNotReady
	}
	if errors.Is(err, model.ErrAssetChannelCredentialChanged) {
		return nil, ErrAssetCredentialChanged
	}
	if errors.Is(err, model.ErrAssetChannelUnavailable) {
		return nil, ErrAssetUpstreamUnavailable
	}
	if err != nil {
		return nil, err
	}
	if replay {
		return asset, nil
	}

	groupID, err := ensureAssetGroup(ctx, asset, binding, adapter)
	if err != nil {
		if errors.Is(err, errAssetGroupCreateOutcomeUnknown) {
			if persistErr := markRemoteCreateUnknown(asset, binding, config.CreateUnknownTTLSeconds); persistErr != nil {
				return nil, persistErr
			}
			return asset, nil
		}
		if persistErr := markRemoteCreateFailed(asset, binding, "asset_upstream_error"); persistErr != nil {
			return nil, persistErr
		}
		return nil, fmt.Errorf("%w: Provider group is unavailable", ErrAssetUpstreamError)
	}
	proceed, err := remoteCreateMayProceed(asset, binding)
	if err != nil {
		return nil, err
	}
	if !proceed {
		return asset, nil
	}
	result, createErr := adapter.CreateAsset(ctx, assetadapter.AssetRequest{GroupResourceID: groupID, URL: remoteURL, Name: asset.Name, MediaType: asset.MediaType})
	if createErr != nil {
		if assetadapter.IsDefinitiveUpstreamRejection(createErr) {
			if err := markRemoteCreateFailed(asset, binding, "asset_upstream_error"); err != nil {
				return nil, err
			}
			return nil, ErrAssetUpstreamError
		}
		if err := markRemoteCreateUnknown(asset, binding, config.CreateUnknownTTLSeconds); err != nil {
			return nil, err
		}
		return asset, nil
	}
	if err := saveRemoteCreateResult(asset, binding, result); err != nil {
		if errors.Is(err, model.ErrAssetOwnershipConflict) {
			if persistErr := markRemoteCreateOwnershipConflict(asset, binding, result); persistErr != nil {
				return nil, persistErr
			}
			return nil, ErrAssetUpstreamError
		}
		return nil, err
	}
	if asset.Status == model.AssetStatusFailed {
		return nil, ErrAssetUpstreamError
	}
	return asset, nil
}

func remoteAssetIdempotency(userID, appID int, rawKey, endpoint string, request any) (*model.AssetCreateIdempotency, error) {
	if rawKey == "" {
		return nil, nil
	}
	if len(rawKey) > 191 {
		return nil, fmt.Errorf("%w: idempotency key is too long", ErrInvalidAssetRequest)
	}
	requestBytes, err := common.Marshal(request)
	if err != nil {
		return nil, err
	}
	return &model.AssetCreateIdempotency{
		UserID: userID, AppID: appID, Endpoint: endpoint,
		KeyHash:     common.GenerateHMAC(fmt.Sprintf("asset-create-key/v2\n%d\n%d\n%s\n%s", userID, appID, endpoint, rawKey)),
		RequestHMAC: common.GenerateHMACWithKey([]byte(common.CryptoSecret), string(requestBytes)),
		Status:      model.AssetCreateIdempotencyCreating,
		ExpiresAt:   common.GetTimestamp() + 24*60*60,
	}, nil
}

func saveRemoteCreateResult(asset *model.Asset, binding *model.AssetBinding, result assetadapter.AssetResult) error {
	status := model.AssetBindingStatusProcessing
	assetStatus := model.AssetStatusProcessing
	errorCode := result.ErrorCode
	errorMessage := publicAssetUpstreamError(result.ErrorMessage)
	if result.Status == "active" {
		managementOnly := dto.AssetUpstreamProfile(binding.UpstreamProfile) == dto.AssetUpstreamProfileJoyCreator
		if result.ResourceID == "" || (!managementOnly && (result.ReferenceType != "asset_uri_id" || result.ReferenceValue == "")) {
			status = model.AssetBindingStatusFailed
			assetStatus = model.AssetStatusFailed
			errorCode = "invalid_upstream_asset"
			errorMessage = "upstream asset response is incomplete"
		} else {
			status = model.AssetBindingStatusActive
			assetStatus = model.AssetStatusReady
		}
	} else if result.Status == "failed" {
		status = model.AssetBindingStatusFailed
		assetStatus = model.AssetStatusFailed
	} else if result.ResourceID == "" {
		status = model.AssetBindingStatusFailed
		assetStatus = model.AssetStatusFailed
		errorCode = "invalid_upstream_asset"
		errorMessage = "upstream asset response is incomplete"
	}
	now := common.GetTimestamp()
	var savedAsset *model.Asset
	var savedBinding *model.AssetBinding
	err := runAssetStateTransaction(func() error {
		return model.DB.Transaction(func(tx *gorm.DB) error {
			currentAsset, currentBinding, authorizationActive, err := lockRemoteCreateState(tx, asset, binding)
			if err != nil {
				return err
			}
			if err := model.ClaimAssetOwnership(tx, currentBinding, result.ResourceID); err != nil {
				return err
			}
			if !remoteCreateStateOpen(currentAsset, currentBinding, authorizationActive) {
				alreadyComplete := (currentAsset.Status == model.AssetStatusReady || currentAsset.Status == model.AssetStatusProcessing) &&
					(currentBinding.Status == model.AssetBindingStatusActive || currentBinding.Status == model.AssetBindingStatusProcessing)
				if alreadyComplete || result.ResourceID == "" {
					if err := finishRemoteCreateWatchdogTx(tx, currentBinding.ID); err != nil {
						return err
					}
					savedAsset, savedBinding = currentAsset, currentBinding
					return nil
				}

				if err := tx.Model(currentBinding).Updates(map[string]any{
					"upstream_resource_id": result.ResourceID, "upstream_business_id": result.BusinessID,
					"upstream_request_id": result.RequestID, "upstream_reference_type": result.ReferenceType,
					"upstream_reference_value": result.ReferenceValue, "status": model.AssetBindingStatusDeleting,
					"error_code": "", "error_message": "", "updated_at": now,
				}).Error; err != nil {
					return err
				}
				if err := tx.Model(currentAsset).Updates(map[string]any{
					"status": model.AssetStatusDeleting, "deleted_at": int64(0),
					"error_code": "", "error_message": "", "updated_at": now,
				}).Error; err != nil {
					return err
				}
				cleanup := &model.AssetOperationJob{
					IdempotencyKey: fmt.Sprintf("delete-binding:%d", currentBinding.ID), Kind: "delete_binding",
					AssetID: &currentAsset.ID, BindingID: &currentBinding.ID,
				}
				if _, err := model.EnsureAssetOperationJob(tx, cleanup, true); err != nil {
					return err
				}
				if err := updateAssetCreateIdempotency(tx, currentAsset.ID, model.AssetStatusFailed); err != nil {
					return err
				}
				if err := finishRemoteCreateWatchdogTx(tx, currentBinding.ID); err != nil {
					return err
				}
				currentAsset.Status = model.AssetStatusDeleting
				currentAsset.DeletedAt = 0
				currentBinding.Status = model.AssetBindingStatusDeleting
				currentBinding.UpstreamResourceID = result.ResourceID
				currentBinding.UpstreamBusinessID = result.BusinessID
				currentBinding.UpstreamRequestID = result.RequestID
				currentBinding.UpstreamReferenceType = result.ReferenceType
				currentBinding.UpstreamReferenceValue = result.ReferenceValue
				savedAsset, savedBinding = currentAsset, currentBinding
				return nil
			}

			if err := tx.Model(currentBinding).Updates(map[string]any{
				"upstream_resource_id": result.ResourceID, "upstream_business_id": result.BusinessID,
				"upstream_request_id": result.RequestID, "upstream_reference_type": result.ReferenceType,
				"upstream_reference_value": result.ReferenceValue,
				"status":                   status, "error_code": errorCode, "error_message": errorMessage, "updated_at": now,
			}).Error; err != nil {
				return err
			}
			if err := tx.Model(currentAsset).Updates(map[string]any{"status": assetStatus, "error_code": errorCode, "error_message": errorMessage, "updated_at": now}).Error; err != nil {
				return err
			}
			if err := updateAssetCreateIdempotency(tx, currentAsset.ID, assetStatus); err != nil {
				return err
			}
			if err := finishRemoteCreateWatchdogTx(tx, currentBinding.ID); err != nil {
				return err
			}
			if status == model.AssetBindingStatusProcessing {
				if _, err := model.EnsureAssetOperationJob(tx, &model.AssetOperationJob{IdempotencyKey: fmt.Sprintf("poll-binding:%d", currentBinding.ID), Kind: "poll_binding", AssetID: &currentAsset.ID, BindingID: &currentBinding.ID}, false); err != nil {
					return err
				}
			}
			currentAsset.Status = assetStatus
			currentAsset.ErrorCode = errorCode
			currentAsset.ErrorMessage = errorMessage
			currentBinding.Status = status
			currentBinding.ErrorCode = errorCode
			currentBinding.ErrorMessage = errorMessage
			currentBinding.UpstreamResourceID = result.ResourceID
			currentBinding.UpstreamBusinessID = result.BusinessID
			currentBinding.UpstreamRequestID = result.RequestID
			currentBinding.UpstreamReferenceType = result.ReferenceType
			currentBinding.UpstreamReferenceValue = result.ReferenceValue
			savedAsset, savedBinding = currentAsset, currentBinding
			return nil
		})
	})
	if err == nil && savedAsset != nil && savedBinding != nil {
		*asset = *savedAsset
		*binding = *savedBinding
	}
	return err
}

func markRemoteCreateFailed(asset *model.Asset, binding *model.AssetBinding, code string) error {
	now := common.GetTimestamp()
	var savedAsset *model.Asset
	var savedBinding *model.AssetBinding
	err := model.DB.Transaction(func(tx *gorm.DB) error {
		currentAsset, currentBinding, authorizationActive, err := lockRemoteCreateState(tx, asset, binding)
		if err != nil {
			return err
		}
		if !remoteCreateStateOpen(currentAsset, currentBinding, authorizationActive) {
			if err := finishRemoteCreateWatchdogTx(tx, currentBinding.ID); err != nil {
				return err
			}
			savedAsset, savedBinding = currentAsset, currentBinding
			return nil
		}
		if err := tx.Model(currentBinding).Updates(map[string]any{"status": model.AssetBindingStatusFailed, "error_code": code, "error_message": "upstream asset operation failed", "updated_at": now}).Error; err != nil {
			return err
		}
		if err := tx.Model(currentAsset).Updates(map[string]any{"status": model.AssetStatusFailed, "error_code": code, "error_message": "upstream asset operation failed", "updated_at": now}).Error; err != nil {
			return err
		}
		if err := updateAssetCreateIdempotency(tx, currentAsset.ID, model.AssetStatusFailed); err != nil {
			return err
		}
		if err := finishRemoteCreateWatchdogTx(tx, currentBinding.ID); err != nil {
			return err
		}
		currentAsset.Status = model.AssetStatusFailed
		currentAsset.ErrorCode = code
		currentAsset.ErrorMessage = "upstream asset operation failed"
		currentBinding.Status = model.AssetBindingStatusFailed
		currentBinding.ErrorCode = code
		currentBinding.ErrorMessage = "upstream asset operation failed"
		savedAsset, savedBinding = currentAsset, currentBinding
		return nil
	})
	if err == nil && savedAsset != nil && savedBinding != nil {
		*asset = *savedAsset
		*binding = *savedBinding
	}
	return err
}

func markRemoteCreateOwnershipConflict(asset *model.Asset, binding *model.AssetBinding, result assetadapter.AssetResult) error {
	now := common.GetTimestamp()
	var savedAsset *model.Asset
	var savedBinding *model.AssetBinding
	err := model.DB.Transaction(func(tx *gorm.DB) error {
		currentAsset, currentBinding, authorizationActive, err := lockRemoteCreateState(tx, asset, binding)
		if err != nil {
			return err
		}
		if !remoteCreateStateOpen(currentAsset, currentBinding, authorizationActive) {
			savedAsset, savedBinding = currentAsset, currentBinding
			return finishRemoteCreateWatchdogTx(tx, currentBinding.ID)
		}
		if err := tx.Model(currentBinding).Updates(map[string]any{
			"status":                   model.AssetBindingStatusFailed,
			"error_code":               "asset_ownership_conflict",
			"error_message":            "upstream asset requires manual ownership review",
			"upstream_resource_id":     result.ResourceID,
			"upstream_business_id":     result.BusinessID,
			"upstream_request_id":      result.RequestID,
			"upstream_reference_type":  "",
			"upstream_reference_value": "",
			"updated_at":               now,
		}).Error; err != nil {
			return err
		}
		if err := tx.Model(currentAsset).Updates(map[string]any{
			"status":        model.AssetStatusFailed,
			"error_code":    "asset_ownership_conflict",
			"error_message": "upstream asset requires manual ownership review",
			"updated_at":    now,
		}).Error; err != nil {
			return err
		}
		if err := updateAssetCreateIdempotency(tx, currentAsset.ID, model.AssetStatusFailed); err != nil {
			return err
		}
		if err := finishRemoteCreateWatchdogTx(tx, currentBinding.ID); err != nil {
			return err
		}
		currentAsset.Status = model.AssetStatusFailed
		currentAsset.ErrorCode = "asset_ownership_conflict"
		currentAsset.ErrorMessage = "upstream asset requires manual ownership review"
		currentBinding.Status = model.AssetBindingStatusFailed
		currentBinding.ErrorCode = "asset_ownership_conflict"
		currentBinding.ErrorMessage = "upstream asset requires manual ownership review"
		currentBinding.UpstreamResourceID = result.ResourceID
		currentBinding.UpstreamBusinessID = result.BusinessID
		currentBinding.UpstreamRequestID = result.RequestID
		currentBinding.UpstreamReferenceType = ""
		currentBinding.UpstreamReferenceValue = ""
		savedAsset, savedBinding = currentAsset, currentBinding
		return nil
	})
	if err == nil && savedAsset != nil && savedBinding != nil {
		*asset = *savedAsset
		*binding = *savedBinding
	}
	return err
}

func markRemoteCreateUnknown(asset *model.Asset, binding *model.AssetBinding, ttlSeconds int64) error {
	now := common.GetTimestamp()
	var savedAsset *model.Asset
	var savedBinding *model.AssetBinding
	err := model.DB.Transaction(func(tx *gorm.DB) error {
		currentAsset, currentBinding, authorizationActive, err := lockRemoteCreateState(tx, asset, binding)
		if err != nil {
			return err
		}
		if !remoteCreateStateOpen(currentAsset, currentBinding, authorizationActive) {
			if err := finishRemoteCreateWatchdogTx(tx, currentBinding.ID); err != nil {
				return err
			}
			savedAsset, savedBinding = currentAsset, currentBinding
			return nil
		}
		if err := tx.Model(currentBinding).Updates(map[string]any{"status": model.AssetBindingStatusCreateUnknown, "error_code": "", "error_message": "", "updated_at": now}).Error; err != nil {
			return err
		}
		if err := tx.Model(currentAsset).Updates(map[string]any{"status": model.AssetStatusCreateUnknown, "error_code": "", "error_message": "", "updated_at": now}).Error; err != nil {
			return err
		}
		if err := updateAssetCreateIdempotency(tx, currentAsset.ID, model.AssetStatusCreateUnknown); err != nil {
			return err
		}
		job := &model.AssetOperationJob{IdempotencyKey: remoteCreateWatchdogKey(currentBinding.ID), Kind: "resolve_unknown_create", AssetID: &currentAsset.ID, BindingID: &currentBinding.ID, NextAttemptAt: now + ttlSeconds}
		if _, err := model.EnsureAssetOperationJob(tx, job, false); err != nil {
			return err
		}
		currentAsset.Status = model.AssetStatusCreateUnknown
		currentAsset.ErrorCode = ""
		currentAsset.ErrorMessage = ""
		currentBinding.Status = model.AssetBindingStatusCreateUnknown
		currentBinding.ErrorCode = ""
		currentBinding.ErrorMessage = ""
		savedAsset, savedBinding = currentAsset, currentBinding
		return nil
	})
	if err == nil && savedAsset != nil && savedBinding != nil {
		*asset = *savedAsset
		*binding = *savedBinding
	}
	return err
}

func updateAssetCreateIdempotency(tx *gorm.DB, assetID int64, status string) error {
	switch status {
	case model.AssetStatusReady:
		status = model.AssetCreateIdempotencyComplete
	case model.AssetStatusProcessing:
		status = model.AssetCreateIdempotencyProcessing
	case model.AssetStatusCreateUnknown:
		status = model.AssetCreateIdempotencyCreateUnknown
	case model.AssetStatusFailed:
		status = model.AssetCreateIdempotencyFailed
	}
	return tx.Model(&model.AssetCreateIdempotency{}).Where("asset_id = ?", assetID).Updates(map[string]any{"status": status, "updated_at": common.GetTimestamp()}).Error
}
