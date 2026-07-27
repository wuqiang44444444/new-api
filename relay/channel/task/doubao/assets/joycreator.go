package assets

import (
	"context"
	"fmt"
	"net/http"
	"net/url"

	"github.com/QuantumNous/new-api/dto"
)

type JoyCreatorAdapter struct{ client }

func NewJoyCreatorAdapter(baseURL, apiKey string, httpClient HTTPDoer) *JoyCreatorAdapter {
	return &JoyCreatorAdapter{client: newClient(baseURL, apiKey, httpClient)}
}

func (*JoyCreatorAdapter) Profile() dto.AssetUpstreamProfile {
	return dto.AssetUpstreamProfileJoyCreator
}
func (*JoyCreatorAdapter) Supports(kind, mediaType string) bool {
	return kind == "general" && (mediaType == "image" || mediaType == "video" || mediaType == "audio")
}

type joyError struct {
	Code    int    `json:"code"`
	Message string `json:"message"`
}

type joyGroup struct {
	ID        string `json:"id"`
	GroupID   string `json:"groupId"`
	Status    int    `json:"status"`
	GroupName string `json:"groupName"`
}

type joyAsset struct {
	ID           string `json:"id"`
	AssetID      string `json:"assetId"`
	VendorURL    string `json:"vendorUrl"`
	VendorStatus string `json:"vendorStatus"`
	Status       int    `json:"status"`
	ErrorMessage string `json:"errorMsg"`
}

type joyResponse struct {
	RequestID string    `json:"requestId"`
	Error     *joyError `json:"error"`
	Result    struct {
		Group joyGroup `json:"group"`
		Asset joyAsset `json:"asset"`
		ID    any      `json:"id"`
	} `json:"result"`
}

func (a *JoyCreatorAdapter) CreateGroup(ctx context.Context, req GroupRequest) (GroupResult, error) {
	var response joyResponse
	err := a.request(ctx, http.MethodPost, "/joycreator/openApi/v1/asset/group/create", map[string]any{"Name": req.Name, "Description": req.Description, "GroupType": "AIGC"}, &response)
	return normalizeJoyGroup(response, err)
}

func (a *JoyCreatorAdapter) GetGroup(ctx context.Context, resourceID string) (GroupResult, error) {
	var response joyResponse
	err := a.request(ctx, http.MethodPost, "/joycreator/openApi/v1/asset/group/detail/"+url.PathEscape(resourceID), nil, &response)
	return normalizeJoyGroup(response, err)
}

func (a *JoyCreatorAdapter) UpdateGroup(ctx context.Context, resourceID string, req GroupRequest) (GroupResult, error) {
	var response joyResponse
	err := a.request(ctx, http.MethodPost, "/joycreator/openApi/v1/asset/group/"+url.PathEscape(resourceID), map[string]any{"Name": req.Name, "Description": req.Description}, &response)
	if response.Result.Group.ID == "" {
		response.Result.Group.ID = resourceID
	}
	return normalizeJoyGroup(response, err)
}

func (a *JoyCreatorAdapter) DeleteGroup(ctx context.Context, resourceID string) error {
	return fmt.Errorf("JoyCreator group deletion is not documented by the upstream protocol")
}

func (a *JoyCreatorAdapter) CreateAsset(ctx context.Context, req AssetRequest) (AssetResult, error) {
	var response joyResponse
	err := a.request(ctx, http.MethodPost, "/joycreator/openApi/v1/asset/create", map[string]any{"groupId": req.GroupResourceID, "URL": req.URL, "AssetType": normalizedMediaType(req.MediaType), "Name": req.Name}, &response)
	return normalizeJoyAsset(response, err)
}

func (a *JoyCreatorAdapter) GetAsset(ctx context.Context, resourceID string) (AssetResult, error) {
	var response joyResponse
	err := a.request(ctx, http.MethodPost, "/joycreator/openApi/v1/asset/detail/"+url.PathEscape(resourceID), nil, &response)
	return normalizeJoyAsset(response, err)
}

func (a *JoyCreatorAdapter) UpdateAsset(ctx context.Context, resourceID, name string) (AssetResult, error) {
	var response joyResponse
	err := a.request(ctx, http.MethodPost, "/joycreator/openApi/v1/asset/"+url.PathEscape(resourceID), map[string]string{"Name": name}, &response)
	if response.Result.Asset.ID == "" {
		response.Result.Asset.ID = resourceID
	}
	return normalizeJoyAsset(response, err)
}

func (a *JoyCreatorAdapter) DeleteAsset(ctx context.Context, resourceID string) error {
	var response joyResponse
	err := a.request(ctx, http.MethodDelete, "/joycreator/openApi/v1/asset/"+url.PathEscape(resourceID), nil, &response)
	if upstreamNotFound(err) {
		return nil
	}
	if err != nil {
		return err
	}
	if response.Error != nil {
		return fmt.Errorf("JoyCreator error %d (request %s)", response.Error.Code, response.RequestID)
	}
	return nil
}

func normalizeJoyGroup(response joyResponse, requestErr error) (GroupResult, error) {
	if requestErr != nil {
		return GroupResult{}, requestErr
	}
	if response.Error != nil {
		return GroupResult{}, &upstreamApplicationError{provider: "JoyCreator", code: response.Error.Code}
	}
	resourceID := response.Result.Group.ID
	if resourceID == "" && response.Result.ID != nil {
		resourceID = fmt.Sprint(response.Result.ID)
	}
	status := "processing"
	if response.Result.Group.Status == 1 {
		status = "active"
	} else if response.Result.Group.Status == 2 {
		status = "failed"
	}
	return GroupResult{ResourceID: resourceID, BusinessID: response.Result.Group.GroupID, Status: status, RequestID: response.RequestID}, nil
}

func normalizeJoyAsset(response joyResponse, requestErr error) (AssetResult, error) {
	if requestErr != nil {
		return AssetResult{}, requestErr
	}
	if response.Error != nil {
		return AssetResult{}, &upstreamApplicationError{provider: "JoyCreator", code: response.Error.Code}
	}
	asset := response.Result.Asset
	if asset.ID == "" && response.Result.ID != nil {
		asset.ID = fmt.Sprint(response.Result.ID)
	}
	result := AssetResult{ResourceID: asset.ID, BusinessID: asset.AssetID, RequestID: response.RequestID, ErrorMessage: asset.ErrorMessage}
	switch {
	case asset.Status == 2 || asset.VendorStatus == "Failed":
		result.Status = "failed"
	case asset.Status == 1 && asset.VendorURL != "" && (asset.VendorStatus == "" || asset.VendorStatus == "Active"):
		result.Status = "active"
	default:
		result.Status = "processing"
	}
	return result, nil
}
