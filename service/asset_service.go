package service

import (
	"context"
	"errors"
	"net/url"
	"strings"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/dto"
	"github.com/QuantumNous/new-api/model"
	assetadapter "github.com/QuantumNous/new-api/relay/channel/task/seedance/assets"
	"github.com/QuantumNous/new-api/setting/asset_setting"
)

func CreateRemoteAsset(ctx context.Context, group string, req dto.CreateAssetRequest) (dto.AssetResponse, error) {
	req.Name = strings.TrimSpace(req.Name)
	req.AssetKind = strings.TrimSpace(req.AssetKind)
	req.MediaType = strings.TrimSpace(req.MediaType)
	req.Model = strings.TrimSpace(req.Model)
	req.AssetGroupID = strings.TrimSpace(req.AssetGroupID)
	if req.Name == "" || len([]rune(req.Name)) > dto.PublicAssetNameMaxCharacters || req.Model == "" ||
		!model.ValidateAssetKind(req.AssetKind) || !model.ValidateAssetMediaType(req.MediaType) {
		return dto.AssetResponse{}, ErrInvalidAssetRequest
	}
	if req.AssetKind == model.AssetKindRealPerson && req.MediaType != "image" {
		return dto.AssetResponse{}, ErrUnsupportedAssetType
	}
	if req.AssetKind == model.AssetKindRealPerson && req.AssetGroupID == "" {
		return dto.AssetResponse{}, ErrInvalidAssetRequest
	}
	if strings.TrimSpace(req.Source.Type) != "url" {
		return dto.AssetResponse{}, ErrAssetURLRequired
	}

	config := asset_setting.Current()
	if !config.Enabled {
		return dto.AssetResponse{}, ErrAssetLibraryUnavailable
	}
	remoteURL, err := validateRemoteAssetURL(req.Source.URL, config.RemoteURLMaxLength)
	if err != nil {
		return dto.AssetResponse{}, err
	}
	channel, adapter, err := assetAdapterForModel(group, req.Model)
	if err != nil {
		return dto.AssetResponse{}, err
	}
	settings := channel.GetOtherSettings()
	if settings.AssetMinURLTTLSeconds <= 0 {
		return dto.AssetResponse{}, ErrAssetUpstreamUnavailable
	}
	if err := validateRemoteAssetTTL(req.Source.ExpiresAt, settings.AssetMinURLTTLSeconds, time.Now()); err != nil {
		return dto.AssetResponse{}, err
	}
	if !adapter.Supports(req.AssetKind, req.MediaType) {
		return dto.AssetResponse{}, ErrUnsupportedAssetType
	}

	assetRequest := assetadapter.AssetRequest{
		GroupResourceID: req.AssetGroupID,
		URL:             remoteURL,
		Name:            req.Name,
		MediaType:       req.MediaType,
	}
	if settings.AssetUpstreamProtocol == dto.AssetUpstreamProtocolFunCloudMaterial {
		source, sourceErr := openFunCloudAssetSource(ctx, remoteURL, req.MediaType)
		if sourceErr != nil {
			return dto.AssetResponse{}, sourceErr
		}
		defer source.Body.Close()
		assetRequest.Source = source.Body
		assetRequest.SourceType = source.ContentType
		assetRequest.SourceMaxBytes = funCloudMaterialMaxBytes
		assetRequest.SourceFilename = source.Filename
	}
	result, err := adapter.CreateAsset(ctx, assetRequest)
	if err != nil {
		return dto.AssetResponse{}, normalizeAssetAdapterError(err)
	}
	if strings.TrimSpace(result.ResourceID) == "" {
		common.SysError("Seedance asset creation returned no resource id")
		return dto.AssetResponse{}, ErrAssetUpstreamError
	}
	return assetResponse(req.Model, result.ResourceID, result), nil
}

func GetRemoteAsset(ctx context.Context, group, modelName, resourceID string) (dto.AssetResponse, error) {
	modelName, resourceID, err := validateAssetLookup(modelName, resourceID)
	if err != nil {
		return dto.AssetResponse{}, err
	}
	_, adapter, err := assetAdapterForModel(group, modelName)
	if err != nil {
		return dto.AssetResponse{}, err
	}
	result, err := adapter.GetAsset(ctx, resourceID)
	if err != nil {
		return dto.AssetResponse{}, normalizeAssetAdapterError(err)
	}
	return assetResponse(modelName, resourceID, result), nil
}

func UpdateRemoteAsset(ctx context.Context, group, resourceID string, req dto.UpdateAssetRequest) (dto.AssetResponse, error) {
	req.Model = strings.TrimSpace(req.Model)
	req.Name = strings.TrimSpace(req.Name)
	if req.Name == "" || len([]rune(req.Name)) > dto.PublicAssetNameMaxCharacters {
		return dto.AssetResponse{}, ErrInvalidAssetRequest
	}
	modelName, resourceID, err := validateAssetLookup(req.Model, resourceID)
	if err != nil {
		return dto.AssetResponse{}, err
	}
	_, adapter, err := assetAdapterForModel(group, modelName)
	if err != nil {
		return dto.AssetResponse{}, err
	}
	result, err := adapter.UpdateAsset(ctx, resourceID, req.Name)
	if err != nil {
		return dto.AssetResponse{}, normalizeAssetAdapterError(err)
	}
	return assetResponse(modelName, resourceID, result), nil
}

func DeleteRemoteAsset(ctx context.Context, group, modelName, resourceID string) error {
	modelName, resourceID, err := validateAssetLookup(modelName, resourceID)
	if err != nil {
		return err
	}
	_, adapter, err := assetAdapterForModel(group, modelName)
	if err != nil {
		return err
	}
	return normalizeAssetAdapterError(adapter.DeleteAsset(ctx, resourceID))
}

func CreateAssetGroup(ctx context.Context, group string, req dto.CreateAssetGroupRequest) (dto.AssetGroupResponse, error) {
	req.Name = strings.TrimSpace(req.Name)
	req.Description = strings.TrimSpace(req.Description)
	req.GroupKind = strings.TrimSpace(req.GroupKind)
	req.Model = strings.TrimSpace(req.Model)
	req.RedirectURL = strings.TrimSpace(req.RedirectURL)
	if req.Name == "" || len([]rune(req.Name)) > dto.PublicAssetNameMaxCharacters || len([]rune(req.Description)) > 300 ||
		req.Model == "" || (req.GroupKind != model.AssetKindGeneral && req.GroupKind != model.AssetKindRealPerson) {
		return dto.AssetGroupResponse{}, ErrInvalidAssetRequest
	}
	if req.RedirectURL != "" {
		if _, err := validProviderURL(req.RedirectURL); err != nil {
			return dto.AssetGroupResponse{}, ErrInvalidAssetRequest
		}
	}
	channel, adapter, err := assetAdapterForModel(group, req.Model)
	if err != nil {
		return dto.AssetGroupResponse{}, err
	}

	if req.GroupKind == model.AssetKindRealPerson {
		verification, ok := adapter.(assetadapter.VerificationAdapter)
		if !ok {
			return dto.AssetGroupResponse{}, ErrUnsupportedAssetOperation
		}
		result, err := verification.CreateVerificationSession(ctx, assetadapter.VerificationRequest{
			RedirectURL: req.RedirectURL,
			ProjectName: channel.GetOtherSettings().AssetProviderProject,
		})
		if err != nil {
			return dto.AssetGroupResponse{}, normalizeAssetAdapterError(err)
		}
		result.SessionID = strings.TrimSpace(result.SessionID)
		result.H5URL = strings.TrimSpace(result.H5URL)
		if result.SessionID == "" || result.H5URL == "" {
			return dto.AssetGroupResponse{}, ErrAssetUpstreamError
		}
		if _, err := validProviderURL(result.H5URL); err != nil {
			return dto.AssetGroupResponse{}, ErrAssetUpstreamError
		}
		return dto.AssetGroupResponse{
			Object:          "asset_group_verification",
			ID:              result.SessionID,
			Model:           req.Model,
			GroupID:         strings.TrimSpace(result.GroupID),
			Status:          normalizeAssetStatus(result.Status),
			VerificationURL: result.H5URL,
			ExpiresAt:       result.ExpiresAt,
		}, nil
	}

	groupAdapter, ok := adapter.(assetadapter.GroupAdapter)
	if !ok {
		return dto.AssetGroupResponse{}, ErrUnsupportedAssetOperation
	}
	result, err := groupAdapter.CreateGroup(ctx, assetadapter.GroupRequest{
		Name: req.Name, Description: req.Description, GroupType: "AIGC",
	})
	if err != nil {
		return dto.AssetGroupResponse{}, normalizeAssetAdapterError(err)
	}
	if strings.TrimSpace(result.ResourceID) == "" {
		return dto.AssetGroupResponse{}, ErrAssetUpstreamError
	}
	return assetGroupResponse(req.Model, result.ResourceID, result), nil
}

func GetRemoteAssetGroup(ctx context.Context, group, modelName, resourceID string, verificationSession bool) (dto.AssetGroupResponse, error) {
	modelName, resourceID, err := validateAssetLookup(modelName, resourceID)
	if err != nil {
		return dto.AssetGroupResponse{}, err
	}
	_, adapter, err := assetAdapterForModel(group, modelName)
	if err != nil {
		return dto.AssetGroupResponse{}, err
	}
	if verificationSession {
		verification, ok := adapter.(assetadapter.VerificationAdapter)
		if !ok {
			return dto.AssetGroupResponse{}, ErrUnsupportedAssetOperation
		}
		result, err := verification.GetVerificationResult(ctx, resourceID)
		if err != nil {
			return dto.AssetGroupResponse{}, normalizeAssetAdapterError(err)
		}
		return dto.AssetGroupResponse{
			Object:  "asset_group_verification",
			ID:      resourceID,
			Model:   modelName,
			GroupID: strings.TrimSpace(result.GroupID),
			Status:  normalizeAssetStatus(result.Status),
		}, nil
	}
	groupAdapter, ok := adapter.(assetadapter.GroupAdapter)
	if !ok {
		return dto.AssetGroupResponse{}, ErrUnsupportedAssetOperation
	}
	result, err := groupAdapter.GetGroup(ctx, resourceID)
	if err != nil {
		return dto.AssetGroupResponse{}, normalizeAssetAdapterError(err)
	}
	return assetGroupResponse(modelName, resourceID, result), nil
}

func DeleteRemoteAssetGroup(ctx context.Context, group, modelName, resourceID string) error {
	modelName, resourceID, err := validateAssetLookup(modelName, resourceID)
	if err != nil {
		return err
	}
	_, adapter, err := assetAdapterForModel(group, modelName)
	if err != nil {
		return err
	}
	groupAdapter, ok := adapter.(assetadapter.GroupAdapter)
	if !ok {
		return ErrUnsupportedAssetOperation
	}
	return normalizeAssetAdapterError(groupAdapter.DeleteGroup(ctx, resourceID))
}

func CheckAssetChannelConnectivity(ctx context.Context, channel *model.Channel) error {
	adapter, _, err := seedanceAssetAdapter(channel)
	if err != nil {
		return err
	}
	connectivity, ok := adapter.(assetadapter.ConnectivityAdapter)
	if !ok {
		return ErrUnsupportedAssetType
	}
	return connectivity.CheckConnectivity(ctx)
}

func assetAdapterForModel(group, modelName string) (*model.Channel, assetadapter.Adapter, error) {
	if !asset_setting.Current().Enabled {
		return nil, nil, ErrAssetLibraryUnavailable
	}
	modelName = strings.TrimSpace(modelName)
	if modelName == "" {
		return nil, nil, ErrInvalidAssetRequest
	}
	channel, err := model.GetEnabledSeedanceChannel(group, modelName, 0)
	if err != nil {
		return nil, nil, ErrAssetUpstreamUnavailable
	}
	if channel == nil {
		catalog, catalogErr := model.GetConfiguredSeedancePublicModels()
		if catalogErr != nil {
			return nil, nil, ErrAssetUpstreamUnavailable
		}
		for _, item := range catalog {
			if item.ModelName == modelName {
				return nil, nil, ErrAssetUpstreamUnavailable
			}
		}
		return nil, nil, ErrAssetModelNotFound
	}
	adapter, _, err := seedanceAssetAdapter(channel)
	if err != nil {
		return nil, nil, err
	}
	return channel, adapter, nil
}

func seedanceAssetAdapter(channel *model.Channel) (assetadapter.Adapter, string, error) {
	if channel == nil || channel.Type != constant.ChannelTypeSeedanceLink {
		return nil, "", ErrAssetUpstreamUnavailable
	}
	settings := channel.GetOtherSettings()
	if settings.AssetUpstreamProtocol == dto.AssetUpstreamProtocolNone {
		return nil, "", ErrAssetLibraryUnsupported
	}
	if !settings.AssetUpstreamProtocol.IsValid() {
		return nil, "", ErrAssetLibraryUnavailable
	}
	key, fingerprint, err := model.ResolveAssetChannelCredential(channel)
	if err != nil {
		return nil, "", ErrAssetUpstreamUnavailable
	}
	httpClient, err := GetHttpClientWithProxy(channel.GetSetting().Proxy)
	if err != nil {
		return nil, "", err
	}
	var adapter assetadapter.Adapter
	switch settings.AssetUpstreamProtocol {
	case dto.AssetUpstreamProtocolVolcengineAction:
		adapter, err = assetadapter.NewVolcengineActionAdapter(key, settings.AssetProviderProject, httpClient)
	case dto.AssetUpstreamProtocolBytePlusAction:
		adapter, err = assetadapter.NewBytePlusActionAdapter(key, settings.AssetRegion, settings.AssetProviderProject, httpClient)
	case dto.AssetUpstreamProtocolArkAssetsV1:
		adapter = assetadapter.NewArkAdapter(channel.GetBaseURL(), key, httpClient)
	case dto.AssetUpstreamProtocolTokenSaveAssetsV1:
		adapter = assetadapter.NewRelayAdapter(channel.GetBaseURL(), key, httpClient)
	case dto.AssetUpstreamProtocolMoxingJoyCreatorV1:
		adapter = assetadapter.NewMoxingJoyCreatorAdapter(channel.GetBaseURL(), key, httpClient)
	case dto.AssetUpstreamProtocolMoxingVolcAssetsV1:
		adapter = assetadapter.NewMoxingVolcAdapter(channel.GetBaseURL(), key, httpClient)
	case dto.AssetUpstreamProtocolFunCloudMaterial:
		adapter = assetadapter.NewFunCloudMaterialAdapter(channel.GetBaseURL(), key, httpClient)
	default:
		return nil, "", ErrAssetLibraryUnavailable
	}
	if err != nil {
		return nil, "", ErrAssetUpstreamUnavailable
	}
	return adapter, fingerprint, nil
}

func validateAssetLookup(modelName, resourceID string) (string, string, error) {
	modelName = strings.TrimSpace(modelName)
	resourceID = strings.TrimSpace(resourceID)
	if modelName == "" || resourceID == "" {
		return "", "", ErrInvalidAssetRequest
	}
	return modelName, resourceID, nil
}

func assetResponse(modelName, fallbackID string, result assetadapter.AssetResult) dto.AssetResponse {
	resourceID := strings.TrimSpace(result.ResourceID)
	if resourceID == "" {
		resourceID = fallbackID
	}
	response := dto.AssetResponse{
		Object:    "asset",
		ID:        resourceID,
		Model:     modelName,
		Status:    normalizeAssetStatus(result.Status),
		ErrorCode: publicAssetErrorCode(result.ErrorCode),
	}
	if value := strings.TrimSpace(result.ReferenceValue); value != "" {
		response.Reference = "asset://" + strings.TrimPrefix(value, "asset://")
	}
	if response.ErrorCode != "" {
		response.Error = "asset operation failed"
	}
	return response
}

func assetGroupResponse(modelName, fallbackID string, result assetadapter.GroupResult) dto.AssetGroupResponse {
	resourceID := strings.TrimSpace(result.ResourceID)
	if resourceID == "" {
		resourceID = fallbackID
	}
	return dto.AssetGroupResponse{
		Object: "asset_group",
		ID:     resourceID,
		Model:  modelName,
		Status: normalizeAssetStatus(result.Status),
	}
}

func normalizeAssetAdapterError(err error) error {
	if err == nil {
		return nil
	}
	if errors.Is(err, assetadapter.ErrAssetOperationUnsupported) || errors.Is(err, assetadapter.ErrGroupDeletionUnsupported) {
		return ErrUnsupportedAssetOperation
	}
	if errors.Is(err, assetadapter.ErrGroupNotEmpty) {
		return ErrAssetGroupNotEmpty
	}
	if assetadapter.IsUpstreamNotFound(err) {
		return ErrAssetNotFound
	}
	if diagnostic, ok := assetadapter.SafeUpstreamDiagnostic(err); ok {
		common.SysError("Seedance asset upstream operation failed: " + diagnostic)
	} else {
		common.SysError("Seedance asset upstream operation failed")
	}
	return ErrAssetUpstreamError
}

func normalizeAssetStatus(status string) string {
	switch strings.ToLower(strings.TrimSpace(status)) {
	case "active", "ready", "success", "succeeded", "approved", "verified", "completed":
		return "ready"
	case "failed", "rejected", "expired", "cancelled", "canceled":
		return "failed"
	default:
		return "processing"
	}
}

func publicAssetErrorCode(providerCode string) string {
	if strings.TrimSpace(providerCode) == "" {
		return ""
	}
	return "upstream_asset_failed"
}

func validProviderURL(value string) (*url.URL, error) {
	parsed, err := url.Parse(strings.TrimSpace(value))
	if err != nil || parsed.Scheme != "https" || parsed.Host == "" || parsed.User != nil {
		return nil, errors.New("provider verification URL is invalid")
	}
	return parsed, nil
}
