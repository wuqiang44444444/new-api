package assets

import (
	"context"
	"net/http"
	"net/url"
	"strings"

	"github.com/QuantumNous/new-api/dto"
)

type ArkAdapter struct{ client }

func NewArkAdapter(baseURL, apiKey string, httpClient HTTPDoer) *ArkAdapter {
	return &ArkAdapter{client: newClient(baseURL, apiKey, httpClient)}
}

func (*ArkAdapter) Profile() dto.AssetUpstreamProfile { return dto.AssetUpstreamProfileArk }
func (*ArkAdapter) Supports(kind, mediaType string) bool {
	if kind == "real_person" {
		return mediaType == "image"
	}
	return kind == "general" && (mediaType == "image" || mediaType == "video" || mediaType == "audio")
}

func (a *ArkAdapter) CheckConnectivity(ctx context.Context) error {
	return a.request(ctx, http.MethodPost, "/v1/ark/assets/list", map[string]any{
		"PageNumber": 1,
		"PageSize":   1,
	}, nil)
}

type arkGroupResponse struct {
	ID           string `json:"Id"`
	Status       string `json:"Status"`
	ErrorCode    string `json:"ErrorCode"`
	ErrorMessage string `json:"ErrorMessage"`
}

type arkAssetResponse struct {
	ID           string `json:"Id"`
	Status       string `json:"Status"`
	AssetURL     string `json:"URL"`
	ErrorCode    string `json:"ErrorCode"`
	ErrorMessage string `json:"ErrorMessage"`
}

type arkVerificationResponse struct {
	SessionID string `json:"session_id"`
	GroupID   string `json:"group_id"`
	H5URL     string `json:"h5_link"`
	Status    string `json:"status"`
	ExpiresAt int64  `json:"expires_at"`
}

func (a *ArkAdapter) CreateGroup(ctx context.Context, req GroupRequest) (GroupResult, error) {
	var response arkGroupResponse
	err := a.request(ctx, http.MethodPost, "/v1/ark/assets/groups", map[string]any{"Name": req.Name, "Description": req.Description, "GroupType": "AIGC"}, &response)
	return normalizeArkGroup(response), err
}

func (a *ArkAdapter) GetGroup(ctx context.Context, resourceID string) (GroupResult, error) {
	var response arkGroupResponse
	err := a.request(ctx, http.MethodGet, "/v1/ark/assets/groups/"+url.PathEscape(resourceID), nil, &response)
	return normalizeArkGroup(response), err
}

func (a *ArkAdapter) UpdateGroup(ctx context.Context, resourceID string, req GroupRequest) (GroupResult, error) {
	var response arkGroupResponse
	err := a.request(ctx, http.MethodPost, "/v1/ark/assets/groups/"+url.PathEscape(resourceID), map[string]any{"Name": req.Name, "Description": req.Description}, &response)
	if response.ID == "" {
		response.ID = resourceID
	}
	return normalizeArkGroup(response), err
}

func (a *ArkAdapter) DeleteGroup(ctx context.Context, resourceID string) error {
	return ErrGroupDeletionUnsupported
}

func (a *ArkAdapter) CreateAsset(ctx context.Context, req AssetRequest) (AssetResult, error) {
	var response arkAssetResponse
	err := a.request(ctx, http.MethodPost, "/v1/ark/assets", map[string]any{"GroupId": req.GroupResourceID, "URL": req.URL, "AssetType": normalizedMediaType(req.MediaType), "Name": req.Name}, &response)
	return normalizeArkAsset(response), err
}

func (a *ArkAdapter) GetAsset(ctx context.Context, resourceID string) (AssetResult, error) {
	var response arkAssetResponse
	err := a.request(ctx, http.MethodGet, "/v1/ark/assets/"+url.PathEscape(resourceID), nil, &response)
	return normalizeArkAsset(response), err
}

func (a *ArkAdapter) UpdateAsset(ctx context.Context, resourceID, name string) (AssetResult, error) {
	var response arkAssetResponse
	err := a.request(ctx, http.MethodPost, "/v1/ark/assets/"+url.PathEscape(resourceID)+"/update", map[string]string{"Name": name}, &response)
	if response.ID == "" {
		response.ID = resourceID
	}
	return normalizeArkAsset(response), err
}

func (a *ArkAdapter) DeleteAsset(ctx context.Context, resourceID string) error {
	err := a.request(ctx, http.MethodPost, "/v1/ark/assets/"+url.PathEscape(resourceID)+"/delete", nil, nil)
	if upstreamNotFound(err) {
		return nil
	}
	return err
}

func (a *ArkAdapter) CreateVerificationSession(ctx context.Context, req VerificationRequest) (VerificationResult, error) {
	var response arkVerificationResponse
	err := a.request(ctx, http.MethodPost, "/v1/ark/assets/visual-validate/session", map[string]string{"client_redirect_url": req.RedirectURL, "project_name": req.ProjectName}, &response)
	return normalizeArkVerification(response), err
}

func (a *ArkAdapter) GetVerificationSession(ctx context.Context, sessionID string) (VerificationResult, error) {
	var response arkVerificationResponse
	err := a.request(ctx, http.MethodGet, "/v1/ark/assets/visual-validate/sessions/"+url.PathEscape(sessionID), nil, &response)
	return normalizeArkVerification(response), err
}

func (a *ArkAdapter) GetVerificationResult(ctx context.Context, sessionID string) (VerificationResult, error) {
	var response arkVerificationResponse
	err := a.request(ctx, http.MethodGet, "/v1/ark/assets/visual-validate/result/"+url.PathEscape(sessionID), nil, &response)
	return normalizeArkVerification(response), err
}

func normalizeArkGroup(response arkGroupResponse) GroupResult {
	status := strings.ToLower(response.Status)
	if response.ErrorCode != "" {
		status = "failed"
	}
	if status == "" {
		status = "active"
	}
	return GroupResult{ResourceID: response.ID, BusinessID: response.ID, Status: status}
}

func normalizeArkAsset(response arkAssetResponse) AssetResult {
	result := AssetResult{ResourceID: response.ID, BusinessID: response.ID, ErrorCode: response.ErrorCode, ErrorMessage: response.ErrorMessage}
	if response.ErrorCode != "" {
		result.Status = "failed"
		return result
	}
	switch strings.ToLower(response.Status) {
	case "active":
		result.Status = "active"
		result.ReferenceType = "asset_uri_id"
		result.ReferenceValue = response.ID
	case "failed":
		result.Status = "failed"
	case "pending":
		result.Status = "pending"
	default:
		result.Status = "processing"
	}
	return result
}

func normalizeArkVerification(response arkVerificationResponse) VerificationResult {
	return VerificationResult{SessionID: response.SessionID, GroupID: response.GroupID, H5URL: response.H5URL, Status: response.Status, ExpiresAt: response.ExpiresAt}
}
