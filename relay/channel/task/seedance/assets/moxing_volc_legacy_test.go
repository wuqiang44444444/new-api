package assets

import (
	"context"
	"fmt"
	"net/http"
	"net/url"
	"strconv"
	"strings"

	"github.com/QuantumNous/new-api/dto"
)

const moxingVolcAssetRoot = "/v1/volc/assets"

type MoxingVolcAdapter struct{ client }

func NewMoxingVolcAdapter(baseURL, apiKey string, httpClient HTTPDoer) *MoxingVolcAdapter {
	return &MoxingVolcAdapter{client: newClient(baseURL, apiKey, httpClient)}
}

func (*MoxingVolcAdapter) Profile() dto.AssetUpstreamProfile {
	return dto.AssetUpstreamProfileMoxingVolc
}

func (*MoxingVolcAdapter) Supports(kind, mediaType string) bool {
	if kind == "real_person" {
		return mediaType == "image"
	}
	return kind == "general" && (mediaType == "image" || mediaType == "video" || mediaType == "audio")
}

type moxingVolcError struct {
	Code    any    `json:"Code"`
	Message string `json:"Message"`
}

type moxingVolcResult struct {
	ID           string          `json:"Id"`
	GroupID      string          `json:"GroupId"`
	AssetID      string          `json:"AssetId"`
	Status       string          `json:"Status"`
	Error        moxingVolcError `json:"Error"`
	ErrorCode    string          `json:"ErrorCode"`
	ErrorMessage string          `json:"ErrorMessage"`
	SessionID    string          `json:"SessionId"`
	H5Link       string          `json:"H5Link"`
	ExpiresAt    int64           `json:"ExpiresAt"`
}

type moxingVolcEnvelope struct {
	RequestID string           `json:"RequestId"`
	Error     moxingVolcError  `json:"Error"`
	Result    moxingVolcResult `json:"Result"`
}

func (a *MoxingVolcAdapter) requestVolc(ctx context.Context, method, path string, body any) (moxingVolcEnvelope, error) {
	var response moxingVolcEnvelope
	if err := a.request(ctx, method, path, body, &response); err != nil {
		return response, err
	}
	if code := moxingVolcErrorCode(response.Error); code != "" {
		return response, &upstreamApplicationError{provider: "Moxing Volcengine", code: applicationErrorCode(response.Error.Code)}
	}
	return response, nil
}

func applicationErrorCode(value any) int {
	switch typed := value.(type) {
	case float64:
		return int(typed)
	case int:
		return typed
	case string:
		if parsed, err := strconv.Atoi(strings.TrimSpace(typed)); err == nil {
			return parsed
		}
	default:
	}
	return 500
}

func moxingVolcErrorCode(value moxingVolcError) string {
	if value.Code == nil {
		return ""
	}
	code := strings.TrimSpace(fmt.Sprint(value.Code))
	if code == "" || code == "0" {
		return ""
	}
	return code
}

func (a *MoxingVolcAdapter) CheckConnectivity(ctx context.Context) error {
	_, err := a.requestVolc(ctx, http.MethodPost, moxingVolcAssetRoot+"/list", map[string]any{
		"PageNumber": 1, "PageSize": 1, "ProjectName": "default",
	})
	return err
}

func (a *MoxingVolcAdapter) CreateGroup(ctx context.Context, req GroupRequest) (GroupResult, error) {
	response, err := a.requestVolc(ctx, http.MethodPost, moxingVolcAssetRoot+"/groups", map[string]any{
		"Name": req.Name, "Description": req.Description, "GroupType": "AIGC",
	})
	return normalizeMoxingVolcGroup(response), err
}

func (a *MoxingVolcAdapter) GetGroup(ctx context.Context, resourceID string) (GroupResult, error) {
	response, err := a.requestVolc(ctx, http.MethodGet, moxingVolcAssetRoot+"/groups/"+url.PathEscape(resourceID)+"?ProjectName=default", nil)
	return normalizeMoxingVolcGroup(response), err
}

func (a *MoxingVolcAdapter) CreateAsset(ctx context.Context, req AssetRequest) (AssetResult, error) {
	response, err := a.requestVolc(ctx, http.MethodPost, moxingVolcAssetRoot, map[string]any{
		"GroupId": req.GroupResourceID, "URL": req.URL, "Name": req.Name,
		"AssetType": normalizedMediaType(req.MediaType), "ProjectName": "default",
	})
	return normalizeMoxingVolcAsset(response), err
}

func (a *MoxingVolcAdapter) GetAsset(ctx context.Context, resourceID string) (AssetResult, error) {
	response, err := a.requestVolc(ctx, http.MethodGet, moxingVolcAssetRoot+"/"+url.PathEscape(resourceID)+"?ProjectName=default", nil)
	return normalizeMoxingVolcAsset(response), err
}

func (a *MoxingVolcAdapter) UpdateAsset(ctx context.Context, resourceID, name string) (AssetResult, error) {
	response, err := a.requestVolc(ctx, http.MethodPost, moxingVolcAssetRoot+"/"+url.PathEscape(resourceID)+"/update", map[string]string{
		"Name": name, "ProjectName": "default",
	})
	if response.Result.ID == "" {
		response.Result.ID = resourceID
	}
	return normalizeMoxingVolcAsset(response), err
}

func (a *MoxingVolcAdapter) DeleteAsset(ctx context.Context, resourceID string) error {
	_, err := a.requestVolc(ctx, http.MethodPost, moxingVolcAssetRoot+"/"+url.PathEscape(resourceID)+"/delete", map[string]string{"ProjectName": "default"})
	if upstreamNotFound(err) {
		return nil
	}
	return err
}

func (a *MoxingVolcAdapter) CreateVerificationSession(ctx context.Context, req VerificationRequest) (VerificationResult, error) {
	response, err := a.requestVolc(ctx, http.MethodPost, moxingVolcAssetRoot+"/visual-validate/sessions", map[string]string{"ProjectName": "default"})
	return normalizeMoxingVolcVerification(response), err
}

func (a *MoxingVolcAdapter) GetVerificationSession(ctx context.Context, sessionID string) (VerificationResult, error) {
	response, err := a.requestVolc(ctx, http.MethodGet, moxingVolcAssetRoot+"/visual-validate/sessions/"+url.PathEscape(sessionID), nil)
	return normalizeMoxingVolcVerification(response), err
}

func (a *MoxingVolcAdapter) GetVerificationResult(ctx context.Context, sessionID string) (VerificationResult, error) {
	response, err := a.requestVolc(ctx, http.MethodGet, moxingVolcAssetRoot+"/visual-validate/results/"+url.PathEscape(sessionID), nil)
	return normalizeMoxingVolcVerification(response), err
}

func normalizeMoxingVolcGroup(response moxingVolcEnvelope) GroupResult {
	group := response.Result
	status := strings.ToLower(strings.TrimSpace(group.Status))
	if status == "" {
		status = "active"
	}
	return GroupResult{ResourceID: group.ID, BusinessID: group.GroupID, Status: status, RequestID: response.RequestID}
}

func normalizeMoxingVolcAsset(response moxingVolcEnvelope) AssetResult {
	asset := response.Result
	errorCode := strings.TrimSpace(asset.ErrorCode)
	if errorCode == "" {
		errorCode = moxingVolcErrorCode(asset.Error)
	}
	errorMessage := strings.TrimSpace(asset.ErrorMessage)
	if errorMessage == "" {
		errorMessage = strings.TrimSpace(asset.Error.Message)
	}
	result := AssetResult{ResourceID: asset.ID, BusinessID: asset.AssetID, ErrorCode: errorCode, ErrorMessage: errorMessage, RequestID: response.RequestID}
	switch strings.ToLower(strings.TrimSpace(asset.Status)) {
	case "active":
		result.Status = "active"
		result.ReferenceType = "asset_uri_id"
		result.ReferenceValue = asset.ID
	case "failed":
		result.Status = "failed"
	default:
		result.Status = "processing"
	}
	if result.ErrorCode != "" {
		result.Status = "failed"
	}
	return result
}

func normalizeMoxingVolcVerification(response moxingVolcEnvelope) VerificationResult {
	result := response.Result
	return VerificationResult{SessionID: result.SessionID, GroupID: result.GroupID, H5URL: result.H5Link, Status: result.Status, ExpiresAt: result.ExpiresAt}
}
