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

const assetVerificationScope = "asset-group-verification:"

func CreateRemoteAsset(ctx context.Context, userID, tokenID int, group string, req dto.CreateAssetRequest) (*model.Asset, error) {
	config := asset_setting.Current()
	if !config.Enabled {
		return nil, ErrAssetLibraryUnavailable
	}
	req.Name = strings.TrimSpace(req.Name)
	req.AssetKind = strings.TrimSpace(req.AssetKind)
	req.MediaType = strings.TrimSpace(req.MediaType)
	req.Model = strings.TrimSpace(req.Model)
	req.AssetGroupID = strings.TrimSpace(req.AssetGroupID)
	if req.Name == "" || len([]rune(req.Name)) > 64 || req.Model == "" ||
		!model.ValidateAssetKind(req.AssetKind) || !model.ValidateAssetMediaType(req.MediaType) {
		return nil, ErrInvalidAssetRequest
	}
	if req.AssetKind == model.AssetKindRealPerson && req.MediaType != "image" {
		return nil, ErrUnsupportedAssetType
	}
	if req.AssetKind == model.AssetKindRealPerson && req.AssetGroupID == "" {
		return nil, ErrAssetReferenceUnresolvable
	}
	if strings.TrimSpace(req.Source.Type) != "url" {
		return nil, ErrAssetURLRequired
	}
	remoteURL, err := validateRemoteAssetURL(req.Source.URL, config.RemoteURLMaxLength)
	if err != nil {
		return nil, err
	}
	channel, err := model.GetEnabledSeedanceChannel(group, req.Model, 0)
	if err != nil || channel == nil {
		return nil, ErrAssetUpstreamUnavailable
	}
	adapter, credentialFingerprint, err := seedanceAssetAdapter(channel)
	if err != nil {
		return nil, err
	}
	settings := channel.GetOtherSettings()
	if settings.AssetMinURLTTLSeconds <= 0 {
		return nil, ErrAssetUpstreamUnavailable
	}
	if err := validateRemoteAssetTTL(req.Source.ExpiresAt, settings.AssetMinURLTTLSeconds, time.Now()); err != nil {
		return nil, err
	}
	if !adapter.Supports(req.AssetKind, req.MediaType) {
		return nil, ErrUnsupportedAssetType
	}
	var count int64
	if err := model.DB.Model(&model.Asset{}).Where("user_id = ? AND deleted_at = ?", userID, 0).Count(&count).Error; err != nil {
		return nil, err
	}
	if count >= config.MaxAssetsPerUser {
		return nil, model.ErrAssetCountLimit
	}

	var groupRow *model.AssetGroup
	groupResourceID := ""
	if req.AssetGroupID != "" {
		groupRow, err = model.GetAssetGroupByPublicIDForApp(userID, tokenID, req.AssetGroupID)
		if err != nil {
			return nil, ErrAssetReferenceUnresolvable
		}
		if err := validateAssetGroupBinding(groupRow, req, channel, credentialFingerprint, settings); err != nil {
			return nil, err
		}
		groupResourceID = groupRow.UpstreamResourceID
	}
	result, err := adapter.CreateAsset(ctx, assetadapter.AssetRequest{
		GroupResourceID: groupResourceID,
		URL:             remoteURL,
		Name:            req.Name,
		MediaType:       req.MediaType,
	})
	if err != nil || strings.TrimSpace(result.ResourceID) == "" {
		common.SysError("Seedance asset creation failed")
		return nil, ErrAssetUpstreamError
	}
	asset := &model.Asset{
		UserID: userID, CreatedByTokenID: tokenID, AppID: tokenID,
		Name: req.Name, AssetKind: req.AssetKind, MediaType: req.MediaType, RequestedModel: req.Model,
		ChannelID: channel.Id, CredentialFingerprint: credentialFingerprint,
		UpstreamProtocol: string(settings.AssetUpstreamProtocol), ProviderProject: settings.AssetProviderProject,
		Region: settings.AssetRegion, UpstreamResourceID: result.ResourceID,
		UpstreamBusinessID: result.BusinessID, UpstreamReferenceType: result.ReferenceType,
		UpstreamReferenceValue: result.ReferenceValue, Status: normalizeAssetStatus(result.Status),
		ErrorCode: publicAssetErrorCode(result.ErrorCode), ErrorMessage: publicAssetErrorMessage(result.ErrorCode),
	}
	if groupRow != nil {
		asset.AssetGroupID = &groupRow.ID
	}
	err = model.DB.Create(asset).Error
	if err != nil {
		common.SysError("Seedance asset local persistence failed after Provider creation")
		return nil, err
	}
	return asset, nil
}

func validateAssetGroupBinding(
	group *model.AssetGroup,
	request dto.CreateAssetRequest,
	channel *model.Channel,
	credentialFingerprint string,
	settings dto.ChannelOtherSettings,
) error {
	if group == nil || group.Status != model.AssetStatusReady || group.GroupKind != request.AssetKind {
		return ErrAssetReferenceUnresolvable
	}
	if channel == nil || group.RequestedModel != request.Model || group.ChannelID != channel.Id {
		return ErrAssetChannelMismatch
	}
	if group.CredentialFingerprint != credentialFingerprint ||
		group.UpstreamProtocol != string(settings.AssetUpstreamProtocol) ||
		group.ProviderProject != settings.AssetProviderProject || group.Region != settings.AssetRegion {
		return ErrAssetScopeConflict
	}
	return nil
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

func CreateAssetGroup(ctx context.Context, userID, tokenID int, group string, req dto.CreateAssetGroupRequest) (*model.AssetGroup, string, error) {
	if !asset_setting.Current().Enabled {
		return nil, "", ErrAssetLibraryUnavailable
	}
	req.Name = strings.TrimSpace(req.Name)
	req.Description = strings.TrimSpace(req.Description)
	req.GroupKind = strings.TrimSpace(req.GroupKind)
	req.Model = strings.TrimSpace(req.Model)
	req.RedirectURL = strings.TrimSpace(req.RedirectURL)
	if req.Name == "" || len([]rune(req.Name)) > 64 || len([]rune(req.Description)) > 300 ||
		req.Model == "" || (req.GroupKind != model.AssetKindGeneral && req.GroupKind != model.AssetKindRealPerson) {
		return nil, "", ErrInvalidAssetRequest
	}
	if req.RedirectURL != "" {
		if _, err := validProviderURL(req.RedirectURL); err != nil {
			return nil, "", ErrInvalidAssetRequest
		}
	}
	channel, err := model.GetEnabledSeedanceChannel(group, req.Model, 0)
	if err != nil || channel == nil {
		return nil, "", ErrAssetUpstreamUnavailable
	}
	adapter, credentialFingerprint, err := seedanceAssetAdapter(channel)
	if err != nil {
		return nil, "", err
	}
	settings := channel.GetOtherSettings()
	row := &model.AssetGroup{
		UserID: userID, CreatedByTokenID: tokenID, AppID: tokenID,
		Name: req.Name, Description: req.Description, GroupKind: req.GroupKind, RequestedModel: req.Model,
		ChannelID: channel.Id, CredentialFingerprint: credentialFingerprint,
		UpstreamProtocol: string(settings.AssetUpstreamProtocol), ProviderProject: settings.AssetProviderProject,
		Region: settings.AssetRegion,
	}
	if req.GroupKind == model.AssetKindRealPerson {
		if err := row.BeforeCreate(nil); err != nil {
			return nil, "", err
		}
		verification, ok := adapter.(assetadapter.VerificationAdapter)
		if !ok {
			return nil, "", ErrUnsupportedAssetType
		}
		verificationResult, verifyErr := verification.CreateVerificationSession(ctx, assetadapter.VerificationRequest{
			RedirectURL: req.RedirectURL, ProjectName: settings.AssetProviderProject,
		})
		verificationURL := strings.TrimSpace(verificationResult.H5URL)
		if verifyErr != nil || verificationResult.SessionID == "" || verificationURL == "" {
			common.SysError("Seedance real-person verification session creation failed")
			return nil, "", ErrAssetUpstreamError
		}
		if _, err := validProviderURL(verificationURL); err != nil {
			return nil, "", ErrAssetUpstreamError
		}
		row.VerificationSessionID = verificationResult.SessionID
		row.UpstreamResourceID = strings.TrimSpace(verificationResult.GroupID)
		row.UpstreamBusinessID = row.UpstreamResourceID
		row.VerificationURLExpiresAt = verificationResult.ExpiresAt
		row.Status = normalizeAssetStatus(verificationResult.Status)
		ciphertext, err := common.EncryptShortLivedSecretForScope(assetVerificationScope+row.PublicID, verificationURL)
		if err != nil {
			common.SysError("Seedance verification URL encryption failed after Provider session creation")
			return nil, "", err
		}
		row.VerificationURLCiphertext = ciphertext
		if err := model.DB.Create(row).Error; err != nil {
			common.SysError("Seedance asset group local persistence failed after Provider verification session creation")
			return nil, "", err
		}
		return row, verificationURL, nil
	}

	groupAdapter, ok := adapter.(assetadapter.GroupAdapter)
	if !ok {
		return nil, "", ErrUnsupportedAssetType
	}
	result, err := groupAdapter.CreateGroup(ctx, assetadapter.GroupRequest{
		Name: req.Name, Description: req.Description, GroupType: "AIGC",
	})
	if err != nil || strings.TrimSpace(result.ResourceID) == "" {
		common.SysError("Seedance asset group creation failed")
		return nil, "", ErrAssetUpstreamError
	}
	row.UpstreamResourceID = result.ResourceID
	row.UpstreamBusinessID = result.BusinessID
	row.Status = normalizeAssetStatus(result.Status)
	if row.Status == model.AssetStatusProcessing && result.Status == "" {
		row.Status = model.AssetStatusReady
	}
	if err := model.DB.Create(row).Error; err != nil {
		common.SysError("Seedance asset group local persistence failed after Provider creation")
		return nil, "", err
	}
	return row, "", nil
}

func RefreshAsset(ctx context.Context, asset *model.Asset) error {
	if asset == nil {
		return ErrAssetNotFound
	}
	channel, adapter, err := adapterForFrozenAsset(asset.ChannelID, asset.CredentialFingerprint, asset.UpstreamProtocol)
	if err != nil {
		return err
	}
	result, err := adapter.GetAsset(ctx, asset.UpstreamResourceID)
	if err != nil {
		if assetadapter.IsUpstreamNotFound(err) {
			if markErr := markAssetDeleted(asset); markErr != nil {
				return markErr
			}
			return ErrAssetNotFound
		}
		return ErrAssetUpstreamError
	}
	settings := channel.GetOtherSettings()
	asset.Status = normalizeAssetStatus(result.Status)
	asset.UpstreamBusinessID = result.BusinessID
	asset.UpstreamReferenceType = result.ReferenceType
	asset.UpstreamReferenceValue = result.ReferenceValue
	asset.ErrorCode = publicAssetErrorCode(result.ErrorCode)
	asset.ErrorMessage = publicAssetErrorMessage(result.ErrorCode)
	asset.UpstreamProtocol = string(settings.AssetUpstreamProtocol)
	asset.UpdatedAt = common.GetTimestamp()
	return model.DB.Save(asset).Error
}

func RefreshAssetGroup(ctx context.Context, group *model.AssetGroup) (string, error) {
	if group == nil {
		return "", ErrAssetNotFound
	}
	_, adapter, err := adapterForFrozenAsset(group.ChannelID, group.CredentialFingerprint, group.UpstreamProtocol)
	if err != nil {
		return "", err
	}
	if group.GroupKind == model.AssetKindRealPerson && group.VerificationSessionID != "" {
		verification, ok := adapter.(assetadapter.VerificationAdapter)
		if !ok {
			return "", ErrUnsupportedAssetType
		}
		verificationResult, err := verification.GetVerificationResult(ctx, group.VerificationSessionID)
		if err != nil {
			if assetadapter.IsUpstreamNotFound(err) {
				if markErr := markAssetGroupDeleted(group); markErr != nil {
					return "", markErr
				}
				return "", ErrAssetNotFound
			}
			return "", ErrAssetUpstreamError
		}
		group.Status = normalizeAssetStatus(verificationResult.Status)
		if strings.TrimSpace(verificationResult.GroupID) != "" {
			group.UpstreamResourceID = strings.TrimSpace(verificationResult.GroupID)
			group.UpstreamBusinessID = group.UpstreamResourceID
		}
	} else {
		groupAdapter, ok := adapter.(assetadapter.GroupAdapter)
		if !ok {
			return "", ErrUnsupportedAssetType
		}
		result, err := groupAdapter.GetGroup(ctx, group.UpstreamResourceID)
		if err != nil {
			if assetadapter.IsUpstreamNotFound(err) {
				if markErr := markAssetGroupDeleted(group); markErr != nil {
					return "", markErr
				}
				return "", ErrAssetNotFound
			}
			return "", ErrAssetUpstreamError
		}
		group.Status = normalizeAssetStatus(result.Status)
	}
	group.UpdatedAt = common.GetTimestamp()
	if err := model.DB.Save(group).Error; err != nil {
		return "", err
	}
	verificationURL := ""
	if group.VerificationURLCiphertext != "" && (group.VerificationURLExpiresAt == 0 || group.VerificationURLExpiresAt > common.GetTimestamp()) {
		verificationURL, err = common.DecryptShortLivedSecretForScope(assetVerificationScope+group.PublicID, group.VerificationURLCiphertext)
		if err != nil {
			return "", ErrAssetUpstreamError
		}
	}
	return verificationURL, nil
}

func RenameAssetForApp(ctx context.Context, userID, appID int, publicID, name string) (*model.Asset, error) {
	asset, err := model.GetAssetByPublicIDForApp(userID, appID, publicID)
	if err != nil || asset == nil {
		return asset, err
	}
	name = strings.TrimSpace(name)
	if name == "" || len([]rune(name)) > 64 {
		return nil, ErrInvalidAssetRequest
	}
	_, adapter, err := adapterForFrozenAsset(asset.ChannelID, asset.CredentialFingerprint, asset.UpstreamProtocol)
	if err != nil {
		return nil, err
	}
	if _, err := adapter.UpdateAsset(ctx, asset.UpstreamResourceID, name); err != nil {
		return nil, ErrAssetUpstreamError
	}
	asset.Name = name
	asset.UpdatedAt = common.GetTimestamp()
	return asset, model.DB.Save(asset).Error
}

func DeleteAssetForApp(ctx context.Context, userID, appID int, publicID string) error {
	asset, err := model.GetAssetByPublicIDForApp(userID, appID, publicID)
	if err != nil || asset == nil {
		return err
	}
	_, adapter, err := adapterForFrozenAsset(asset.ChannelID, asset.CredentialFingerprint, asset.UpstreamProtocol)
	if err != nil {
		return err
	}
	if err := adapter.DeleteAsset(ctx, asset.UpstreamResourceID); err != nil {
		common.SysError("Seedance asset deletion failed")
		return ErrAssetUpstreamError
	}
	return markAssetDeleted(asset)
}

func DeleteAssetGroupForApp(ctx context.Context, userID, appID int, publicID string) error {
	group, err := model.GetAssetGroupByPublicIDForApp(userID, appID, publicID)
	if err != nil || group == nil {
		return err
	}
	var assetCount int64
	if err := model.DB.Model(&model.Asset{}).Where("asset_group_id = ? AND deleted_at = ?", group.ID, 0).Count(&assetCount).Error; err != nil {
		return err
	}
	if assetCount > 0 {
		return ErrInvalidAssetRequest
	}
	_, adapter, err := adapterForFrozenAsset(group.ChannelID, group.CredentialFingerprint, group.UpstreamProtocol)
	if err != nil {
		return err
	}
	if group.UpstreamResourceID != "" {
		groupAdapter, ok := adapter.(assetadapter.GroupAdapter)
		if !ok {
			return ErrUnsupportedAssetType
		}
		if err := groupAdapter.DeleteGroup(ctx, group.UpstreamResourceID); err != nil {
			return ErrAssetUpstreamError
		}
	}
	return markAssetGroupDeleted(group)
}

func markAssetDeleted(asset *model.Asset) error {
	if asset == nil {
		return ErrAssetNotFound
	}
	now := common.GetTimestamp()
	asset.Status = model.AssetStatusDeleted
	asset.DeletedAt = now
	asset.UpdatedAt = now
	return model.DB.Model(asset).Updates(map[string]any{
		"status": asset.Status, "deleted_at": asset.DeletedAt, "updated_at": asset.UpdatedAt,
	}).Error
}

func markAssetGroupDeleted(group *model.AssetGroup) error {
	if group == nil {
		return ErrAssetNotFound
	}
	now := common.GetTimestamp()
	group.Status = model.AssetStatusDeleted
	group.DeletedAt = now
	group.UpdatedAt = now
	group.VerificationURLCiphertext = ""
	return model.DB.Model(group).Updates(map[string]any{
		"status": group.Status, "deleted_at": group.DeletedAt, "updated_at": group.UpdatedAt,
		"verification_url_ciphertext": "",
	}).Error
}

func AssetResponse(asset *model.Asset) dto.AssetResponse {
	response := dto.AssetResponse{
		ID: asset.PublicID, Name: asset.Name, AssetKind: asset.AssetKind, MediaType: asset.MediaType,
		Model: asset.RequestedModel, Status: asset.Status, ErrorCode: asset.ErrorCode,
		CreatedAt: asset.CreatedAt, UpdatedAt: asset.UpdatedAt,
	}
	if asset.ErrorCode != "" {
		response.Error = "asset operation failed"
	}
	if asset.AssetGroupID != nil {
		var group model.AssetGroup
		if model.DB.Select("public_id").First(&group, *asset.AssetGroupID).Error == nil {
			response.AssetGroupID = group.PublicID
		}
	}
	return response
}

func AssetGroupResponse(group *model.AssetGroup, verificationURL string) dto.AssetGroupResponse {
	response := dto.AssetGroupResponse{
		ID: group.PublicID, Name: group.Name, Description: group.Description, GroupKind: group.GroupKind,
		Model: group.RequestedModel, Status: group.Status, VerificationURL: verificationURL,
		ExpiresAt: group.VerificationURLExpiresAt, ErrorCode: group.ErrorCode,
		CreatedAt: group.CreatedAt, UpdatedAt: group.UpdatedAt,
	}
	if group.ErrorCode != "" {
		response.Error = "asset group operation failed"
	}
	return response
}

func seedanceAssetAdapter(channel *model.Channel) (assetadapter.Adapter, string, error) {
	if channel == nil || channel.Type != constant.ChannelTypeSeedanceLink {
		return nil, "", ErrAssetUpstreamUnavailable
	}
	settings := channel.GetOtherSettings()
	if settings.AssetUpstreamProtocol == dto.AssetUpstreamProtocolNone || !settings.AssetUpstreamProtocol.IsValid() {
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
		adapter, err = assetadapter.NewBytePlusActionAdapter(
			key, settings.AssetRegion, settings.AssetProviderProject, httpClient,
		)
	case dto.AssetUpstreamProtocolArkAssetsV1:
		adapter = assetadapter.NewArkAdapter(channel.GetBaseURL(), key, httpClient)
	case dto.AssetUpstreamProtocolRelayAssetsV1:
		adapter = assetadapter.NewRelayAdapter(channel.GetBaseURL(), key, httpClient)
	default:
		return nil, "", ErrAssetLibraryUnavailable
	}
	if err != nil {
		return nil, "", ErrAssetUpstreamUnavailable
	}
	return adapter, fingerprint, nil
}

func adapterForFrozenAsset(channelID int, fingerprint, protocol string) (*model.Channel, assetadapter.Adapter, error) {
	channel, err := model.GetChannelById(channelID, true)
	if err != nil || channel == nil || channel.Status != common.ChannelStatusEnabled || channel.Type != constant.ChannelTypeSeedanceLink {
		return nil, nil, ErrAssetUpstreamUnavailable
	}
	if string(channel.GetOtherSettings().AssetUpstreamProtocol) != protocol {
		return nil, nil, ErrAssetScopeConflict
	}
	adapter, currentFingerprint, err := seedanceAssetAdapter(channel)
	if err != nil {
		return nil, nil, err
	}
	if currentFingerprint != fingerprint {
		return nil, nil, ErrAssetScopeConflict
	}
	return channel, adapter, nil
}

func normalizeAssetStatus(status string) string {
	switch strings.ToLower(strings.TrimSpace(status)) {
	case "active", "ready", "success", "succeeded", "approved", "verified", "completed":
		return model.AssetStatusReady
	case "failed", "rejected", "expired", "cancelled", "canceled":
		return model.AssetStatusFailed
	default:
		return model.AssetStatusProcessing
	}
}

func publicAssetErrorCode(providerCode string) string {
	if strings.TrimSpace(providerCode) == "" {
		return ""
	}
	return "upstream_asset_failed"
}

func publicAssetErrorMessage(providerCode string) string {
	if strings.TrimSpace(providerCode) == "" {
		return ""
	}
	return "upstream asset operation failed"
}

func validProviderURL(value string) (*url.URL, error) {
	parsed, err := url.Parse(strings.TrimSpace(value))
	if err != nil || parsed.Scheme != "https" || parsed.Host == "" || parsed.User != nil {
		return nil, errors.New("provider verification URL is invalid")
	}
	return parsed, nil
}
