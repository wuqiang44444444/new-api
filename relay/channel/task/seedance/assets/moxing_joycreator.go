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

type joyCreatorAssetAdapter struct {
	client
	root         string
	profile      dto.AssetUpstreamProfile
	providerName string
}

type MoxingJoyCreatorAdapter struct {
	*joyCreatorAssetAdapter
}

func NewMoxingJoyCreatorAdapter(baseURL, apiKey string, httpClient HTTPDoer) *MoxingJoyCreatorAdapter {
	return &MoxingJoyCreatorAdapter{joyCreatorAssetAdapter: newJoyCreatorAssetAdapter(
		baseURL,
		apiKey,
		httpClient,
		moxingJoyCreatorAssetRoot,
		dto.AssetUpstreamProfileMoxingJoyCreator,
		"Moxing JoyCreator",
	)}
}

func newJoyCreatorAssetAdapter(
	baseURL string,
	apiKey string,
	httpClient HTTPDoer,
	root string,
	profile dto.AssetUpstreamProfile,
	providerName string,
) *joyCreatorAssetAdapter {
	return &joyCreatorAssetAdapter{
		client:       newClient(baseURL, apiKey, httpClient),
		root:         root,
		profile:      profile,
		providerName: providerName,
	}
}

func (a *joyCreatorAssetAdapter) Profile() dto.AssetUpstreamProfile {
	return a.profile
}

func (*joyCreatorAssetAdapter) Supports(kind, mediaType string) bool {
	return kind == "general" && (mediaType == "image" || mediaType == "video" || mediaType == "audio")
}

func (a *joyCreatorAssetAdapter) RequiresAssetGroup(kind, mediaType string) bool {
	return a.Supports(kind, mediaType)
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
		Group        joyCreatorGroup `json:"group"`
		Asset        joyCreatorAsset `json:"asset"`
		ID           string          `json:"id"`
		GroupID      string          `json:"groupId"`
		AssetID      string          `json:"assetId"`
		VendorURL    string          `json:"vendorUrl"`
		VendorStatus string          `json:"vendorStatus"`
		Status       int             `json:"status"`
		ErrorMsg     string          `json:"errorMsg"`
	} `json:"result"`
}

func (a *joyCreatorAssetAdapter) requestJoyCreator(ctx context.Context, method, path string, body any) (joyCreatorEnvelope, error) {
	var response joyCreatorEnvelope
	if err := a.request(ctx, method, path, body, &response); err != nil {
		return response, err
	}
	if response.Error != nil {
		return response, &upstreamApplicationError{provider: a.providerName, code: response.Error.Code}
	}
	return response, nil
}

func (a *joyCreatorAssetAdapter) CreateGroup(ctx context.Context, req GroupRequest) (GroupResult, error) {
	response, err := a.requestJoyCreator(ctx, http.MethodPost, a.root+"/group/create", map[string]any{
		"Name": req.Name, "Description": req.Description, "GroupType": "AIGC",
	})
	return normalizeJoyCreatorGroup(response), err
}

func (a *joyCreatorAssetAdapter) GetGroup(ctx context.Context, resourceID string) (GroupResult, error) {
	response, err := a.requestJoyCreator(ctx, http.MethodPost, a.root+"/group/detail/"+url.PathEscape(resourceID), nil)
	return normalizeJoyCreatorGroup(response), err
}

func (a *joyCreatorAssetAdapter) CreateAsset(ctx context.Context, req AssetRequest) (AssetResult, error) {
	response, err := a.requestJoyCreator(ctx, http.MethodPost, a.root+"/create", map[string]any{
		"groupId": req.GroupResourceID, "URL": req.URL, "AssetType": normalizedMediaType(req.MediaType), "Name": req.Name,
	})
	return normalizeJoyCreatorAsset(response), err
}

func (a *joyCreatorAssetAdapter) GetAsset(ctx context.Context, resourceID string) (AssetResult, error) {
	response, err := a.requestJoyCreator(ctx, http.MethodPost, a.root+"/detail/"+url.PathEscape(resourceID), nil)
	return normalizeJoyCreatorAsset(response), err
}

func (a *joyCreatorAssetAdapter) UpdateAsset(ctx context.Context, resourceID, name string) (AssetResult, error) {
	response, err := a.requestJoyCreator(ctx, http.MethodPost, a.root+"/"+url.PathEscape(resourceID), map[string]string{"Name": name})
	if response.Result.Asset.ID == "" {
		response.Result.Asset.ID = resourceID
	}
	return normalizeJoyCreatorAsset(response), err
}

func (a *joyCreatorAssetAdapter) DeleteAsset(ctx context.Context, resourceID string) error {
	_, err := a.requestJoyCreator(ctx, http.MethodDelete, a.root+"/"+url.PathEscape(resourceID), nil)
	if upstreamNotFound(err) {
		return nil
	}
	return err
}

func normalizeJoyCreatorGroup(response joyCreatorEnvelope) GroupResult {
	group := response.Result.Group
	directResult := group.ID == "" && response.Result.ID != ""
	if directResult {
		group.ID = response.Result.ID
		group.GroupID = response.Result.GroupID
		group.Status = response.Result.Status
		group.ErrorMsg = response.Result.ErrorMsg
	}
	status := "processing"
	switch group.Status {
	case 1:
		status = "active"
	case 2:
		status = "failed"
	default:
		if directResult {
			status = "active"
		}
	}
	return GroupResult{ResourceID: group.ID, BusinessID: group.GroupID, Status: status, RequestID: response.RequestID}
}

func normalizeJoyCreatorAsset(response joyCreatorEnvelope) AssetResult {
	asset := response.Result.Asset
	if asset.ID == "" && response.Result.ID != "" {
		asset.ID = response.Result.ID
		asset.AssetID = response.Result.AssetID
		asset.VendorURL = response.Result.VendorURL
		asset.VendorStatus = response.Result.VendorStatus
		asset.Status = response.Result.Status
		asset.ErrorMsg = response.Result.ErrorMsg
	}
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
