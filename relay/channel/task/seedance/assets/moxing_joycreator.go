package assets

import (
	"context"
	"fmt"
	"net/http"
	"net/url"
	"strings"

	"github.com/QuantumNous/new-api/dto"
)

const moxingJoyCreatorAssetRoot = "/joycreator/openApi/v1/asset"

type MoxingJoyCreatorAdapter struct{ client }

func NewMoxingJoyCreatorAdapter(baseURL, apiKey string, httpClient HTTPDoer) *MoxingJoyCreatorAdapter {
	return &MoxingJoyCreatorAdapter{client: newClient(baseURL, apiKey, httpClient)}
}

func (*MoxingJoyCreatorAdapter) Profile() dto.AssetUpstreamProfile {
	return dto.AssetUpstreamProfileMoxingJoyCreator
}

func (*MoxingJoyCreatorAdapter) Supports(kind, mediaType string) bool {
	return kind == "general" && (mediaType == "image" || mediaType == "video" || mediaType == "audio")
}

type joyCreatorError struct {
	Code    int    `json:"code"`
	Message string `json:"message"`
}

type joyCreatorGroup struct {
	ID       string `json:"id"`
	GroupID  string `json:"groupId"`
	Status   int    `json:"status"`
	ErrorMsg string `json:"errorMsg"`
}

type joyCreatorAsset struct {
	ID           string `json:"id"`
	AssetID      string `json:"assetId"`
	VendorURL    string `json:"vendorUrl"`
	VendorStatus string `json:"vendorStatus"`
	Status       int    `json:"status"`
	ErrorMsg     string `json:"errorMsg"`
}

type joyCreatorEnvelope struct {
	RequestID string           `json:"requestId"`
	Error     *joyCreatorError `json:"error"`
	Result    struct {
		Group joyCreatorGroup `json:"group"`
		Asset joyCreatorAsset `json:"asset"`
	} `json:"result"`
}

func (a *MoxingJoyCreatorAdapter) requestJoyCreator(ctx context.Context, method, path string, body any) (joyCreatorEnvelope, error) {
	var response joyCreatorEnvelope
	if err := a.request(ctx, method, path, body, &response); err != nil {
		return response, err
	}
	if response.Error != nil {
		return response, &upstreamApplicationError{provider: "Moxing JoyCreator", code: response.Error.Code}
	}
	return response, nil
}

func (a *MoxingJoyCreatorAdapter) CreateGroup(ctx context.Context, req GroupRequest) (GroupResult, error) {
	response, err := a.requestJoyCreator(ctx, http.MethodPost, moxingJoyCreatorAssetRoot+"/group/create", map[string]any{
		"Name": req.Name, "Description": req.Description, "GroupType": "AIGC",
	})
	return normalizeJoyCreatorGroup(response), err
}

func (a *MoxingJoyCreatorAdapter) GetGroup(ctx context.Context, resourceID string) (GroupResult, error) {
	response, err := a.requestJoyCreator(ctx, http.MethodPost, moxingJoyCreatorAssetRoot+"/group/detail/"+url.PathEscape(resourceID), nil)
	return normalizeJoyCreatorGroup(response), err
}

func (*MoxingJoyCreatorAdapter) DeleteGroup(context.Context, string) error {
	return ErrGroupDeletionUnsupported
}

func (a *MoxingJoyCreatorAdapter) CreateAsset(ctx context.Context, req AssetRequest) (AssetResult, error) {
	response, err := a.requestJoyCreator(ctx, http.MethodPost, moxingJoyCreatorAssetRoot+"/create", map[string]any{
		"groupId": req.GroupResourceID, "URL": req.URL, "AssetType": normalizedMediaType(req.MediaType), "Name": req.Name,
	})
	return normalizeJoyCreatorAsset(response), err
}

func (a *MoxingJoyCreatorAdapter) GetAsset(ctx context.Context, resourceID string) (AssetResult, error) {
	response, err := a.requestJoyCreator(ctx, http.MethodPost, moxingJoyCreatorAssetRoot+"/detail/"+url.PathEscape(resourceID), nil)
	return normalizeJoyCreatorAsset(response), err
}

func (a *MoxingJoyCreatorAdapter) UpdateAsset(ctx context.Context, resourceID, name string) (AssetResult, error) {
	response, err := a.requestJoyCreator(ctx, http.MethodPost, moxingJoyCreatorAssetRoot+"/"+url.PathEscape(resourceID), map[string]string{"Name": name})
	if response.Result.Asset.ID == "" {
		response.Result.Asset.ID = resourceID
	}
	return normalizeJoyCreatorAsset(response), err
}

func (a *MoxingJoyCreatorAdapter) DeleteAsset(ctx context.Context, resourceID string) error {
	_, err := a.requestJoyCreator(ctx, http.MethodDelete, moxingJoyCreatorAssetRoot+"/"+url.PathEscape(resourceID), nil)
	if upstreamNotFound(err) {
		return nil
	}
	return err
}

func normalizeJoyCreatorGroup(response joyCreatorEnvelope) GroupResult {
	group := response.Result.Group
	status := "processing"
	switch group.Status {
	case 1:
		status = "active"
	case 2:
		status = "failed"
	}
	return GroupResult{ResourceID: group.ID, BusinessID: group.GroupID, Status: status, RequestID: response.RequestID}
}

func normalizeJoyCreatorAsset(response joyCreatorEnvelope) AssetResult {
	asset := response.Result.Asset
	result := AssetResult{
		ResourceID: asset.ID, BusinessID: asset.AssetID, ErrorMessage: strings.TrimSpace(asset.ErrorMsg), RequestID: response.RequestID,
	}
	switch {
	case asset.Status == 2 || strings.EqualFold(asset.VendorStatus, "Failed"):
		result.Status = "failed"
		result.ErrorCode = "upstream_asset_failed"
	case asset.Status == 1 && strings.EqualFold(asset.VendorStatus, "Active"):
		result.Status = "active"
		result.ReferenceType = "asset_uri_id"
		result.ReferenceValue = strings.TrimSpace(asset.AssetID)
		if result.ReferenceValue == "" {
			result.ReferenceValue = strings.TrimSpace(asset.ID)
		}
	default:
		result.Status = "processing"
	}
	if result.Status == "active" && result.ReferenceValue == "" {
		result.Status = "failed"
		result.ErrorCode = "upstream_asset_failed"
		result.ErrorMessage = fmt.Sprintf("Moxing JoyCreator returned Active without an asset id (request %s)", response.RequestID)
	}
	return result
}
