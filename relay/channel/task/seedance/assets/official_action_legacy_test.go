package assets

import (
	"bytes"
	"context"
	"fmt"
	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/dto"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"
)

func (*OfficialActionAdapter) Profile() dto.AssetUpstreamProfile {
	return dto.AssetUpstreamProfileOfficial
}

func (*OfficialActionAdapter) Supports(kind, mediaType string) bool {
	if kind == "real_person" {
		return mediaType == "image"
	}
	return kind == "general" && (mediaType == "image" || mediaType == "video" || mediaType == "audio")
}

func (a *OfficialActionAdapter) CheckConnectivity(ctx context.Context) error {
	_, _, err := a.ListAssets(ctx, AssetListRequest{GroupType: "AIGC", Page: 1, PageSize: 1})
	return err
}

func (a *OfficialActionAdapter) CreateAsset(ctx context.Context, request AssetRequest) (AssetResult, error) {
	var response officialAsset
	err := a.doAction(ctx, "CreateAsset", map[string]any{
		"GroupId":     request.GroupResourceID,
		"URL":         request.URL,
		"AssetType":   normalizedMediaType(request.MediaType),
		"Name":        request.Name,
		"ProjectName": a.providerProject,
	}, &response)
	return normalizeOfficialAsset(response), err
}

func (a *OfficialActionAdapter) GetAsset(ctx context.Context, resourceID string) (AssetResult, error) {
	var response officialAsset
	err := a.doAction(ctx, "GetAsset", map[string]any{"Id": resourceID, "ProjectName": a.providerProject}, &response)
	return normalizeOfficialAsset(response), err
}

func (a *OfficialActionAdapter) UpdateAsset(ctx context.Context, resourceID, name string) (AssetResult, error) {
	var response officialAsset
	err := a.doAction(ctx, "UpdateAsset", map[string]any{"Id": resourceID, "Name": name, "ProjectName": a.providerProject}, &response)
	if response.ID == "" {
		response.ID = resourceID
	}
	return normalizeOfficialAsset(response), err
}

func (a *OfficialActionAdapter) DeleteAsset(ctx context.Context, resourceID string) error {
	return a.doAction(ctx, "DeleteAsset", map[string]any{"Id": resourceID, "ProjectName": a.providerProject}, nil)
}

func (a *OfficialActionAdapter) CreateGroup(ctx context.Context, request GroupRequest) (GroupResult, error) {
	groupType := strings.TrimSpace(request.GroupType)
	if groupType == "" {
		groupType = "AIGC"
	}
	var response officialGroup
	err := a.doAction(ctx, "CreateAssetGroup", map[string]any{
		"Name":        request.Name,
		"Description": request.Description,
		"GroupType":   groupType,
		"ProjectName": a.providerProject,
	}, &response)
	return normalizeOfficialGroup(response), err
}

func (a *OfficialActionAdapter) GetGroup(ctx context.Context, resourceID string) (GroupResult, error) {
	var response officialGroup
	err := a.doAction(ctx, "GetAssetGroup", map[string]any{"Id": resourceID, "ProjectName": a.providerProject}, &response)
	return normalizeOfficialGroup(response), err
}

func (a *OfficialActionAdapter) ListAssets(ctx context.Context, request AssetListRequest) ([]AssetResult, int, error) {
	groupType := strings.TrimSpace(request.GroupType)
	if groupType == "" {
		return nil, 0, fmt.Errorf("official Action ListAssets requires GroupType")
	}
	filter := map[string]any{"GroupType": groupType}
	if len(request.GroupIDs) > 0 {
		filter["GroupIds"] = request.GroupIDs
	}
	if len(request.Statuses) > 0 {
		filter["Statuses"] = request.Statuses
	}
	if strings.TrimSpace(request.Name) != "" {
		filter["Name"] = strings.TrimSpace(request.Name)
	}
	body := officialListBody(filter, request.Page, request.PageSize, a.providerProject)
	var response struct {
		Items      []officialAsset `json:"Items"`
		TotalCount int             `json:"TotalCount"`
	}
	if err := a.doAction(ctx, "ListAssets", body, &response); err != nil {
		return nil, 0, err
	}
	items := make([]AssetResult, 0, len(response.Items))
	for _, item := range response.Items {
		items = append(items, normalizeOfficialAsset(item))
	}
	return items, response.TotalCount, nil
}

func (a *OfficialActionAdapter) ListGroups(ctx context.Context, request GroupListRequest) ([]GroupResult, int, error) {
	groupType := strings.TrimSpace(request.GroupType)
	if groupType == "" {
		return nil, 0, fmt.Errorf("official Action ListAssetGroups requires GroupType")
	}
	filter := map[string]any{"GroupType": groupType}
	if len(request.GroupIDs) > 0 {
		filter["GroupIds"] = request.GroupIDs
	}
	if strings.TrimSpace(request.Name) != "" {
		filter["Name"] = strings.TrimSpace(request.Name)
	}
	body := officialListBody(filter, request.Page, request.PageSize, a.providerProject)
	var response struct {
		Items      []officialGroup `json:"Items"`
		TotalCount int             `json:"TotalCount"`
	}
	if err := a.doAction(ctx, "ListAssetGroups", body, &response); err != nil {
		return nil, 0, err
	}
	items := make([]GroupResult, 0, len(response.Items))
	for _, item := range response.Items {
		items = append(items, normalizeOfficialGroup(item))
	}
	return items, response.TotalCount, nil
}

func (a *OfficialActionAdapter) CreateVerificationSession(ctx context.Context, request VerificationRequest) (VerificationResult, error) {
	var response struct {
		BytedToken  string `json:"BytedToken"`
		H5Link      string `json:"H5Link"`
		CallbackURL string `json:"CallbackURL"`
	}
	err := a.doAction(ctx, "CreateVisualValidateSession", map[string]any{
		"CallbackURL": request.RedirectURL,
		"ProjectName": a.providerProject,
	}, &response)
	return VerificationResult{
		SessionID: response.BytedToken,
		H5URL:     response.H5Link,
		Status:    "verifying",
		ExpiresAt: a.now().UTC().Add(30 * time.Minute).Unix(),
	}, err
}

func (a *OfficialActionAdapter) GetVerificationSession(ctx context.Context, handle string) (VerificationResult, error) {
	return a.GetVerificationResult(ctx, handle)
}

func (a *OfficialActionAdapter) GetVerificationResult(ctx context.Context, handle string) (VerificationResult, error) {
	var response struct {
		GroupID string `json:"GroupId"`
	}
	err := a.doAction(ctx, "GetVisualValidateResult", map[string]any{
		"BytedToken":  handle,
		"ProjectName": a.providerProject,
	}, &response)
	status := "verifying"
	if response.GroupID != "" {
		status = "active"
	}
	return VerificationResult{GroupID: response.GroupID, Status: status}, err
}

type officialAsset struct {
	ID           string `json:"Id"`
	Status       string `json:"Status"`
	URL          string `json:"URL"`
	ErrorCode    string `json:"ErrorCode"`
	ErrorMessage string `json:"ErrorMessage"`
}

type officialGroup struct {
	ID     string `json:"Id"`
	Name   string `json:"Name"`
	Status string `json:"Status"`
}

func normalizeOfficialAsset(response officialAsset) AssetResult {
	result := AssetResult{
		ResourceID:   response.ID,
		BusinessID:   response.ID,
		ErrorCode:    response.ErrorCode,
		ErrorMessage: response.ErrorMessage,
	}
	switch strings.ToLower(strings.TrimSpace(response.Status)) {
	case "active":
		result.Status = "active"
		result.ReferenceType = "asset_uri_id"
		result.ReferenceValue = response.ID
	case "failed":
		result.Status = "failed"
	case "", "processing":
		result.Status = "processing"
	default:
		result.Status = strings.ToLower(response.Status)
	}
	return result
}

func normalizeOfficialGroup(response officialGroup) GroupResult {
	status := strings.ToLower(strings.TrimSpace(response.Status))
	if status == "" {
		status = "active"
	}
	return GroupResult{ResourceID: response.ID, BusinessID: response.ID, Name: response.Name, Status: status}
}

func officialListBody(filter map[string]any, page, pageSize int, providerProject string) map[string]any {
	if page <= 0 {
		page = 1
	}
	if pageSize <= 0 {
		pageSize = 100
	}
	return map[string]any{
		"Filter":      filter,
		"PageNumber":  page,
		"PageSize":    pageSize,
		"ProjectName": providerProject,
	}
}

func (a *OfficialActionAdapter) doAction(ctx context.Context, action string, body any, result any) error {
	payload, err := common.Marshal(body)
	if err != nil {
		return err
	}
	endpoint, err := url.Parse(a.baseURL)
	if err != nil {
		return err
	}
	query := endpoint.Query()
	query.Set("Action", action)
	query.Set("Version", officialActionVersion)
	endpoint.RawQuery = query.Encode()
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint.String(), bytes.NewReader(payload))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json")
	a.sign(req, payload, a.now().UTC())
	resp, err := a.http.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	responseBody, err := io.ReadAll(io.LimitReader(resp.Body, 4<<20))
	if err != nil {
		return err
	}
	if resp.StatusCode < http.StatusOK || resp.StatusCode >= http.StatusMultipleChoices {
		return officialActionHTTPError(resp.StatusCode, responseBody)
	}
	var raw map[string]any
	if err := common.Unmarshal(responseBody, &raw); err != nil {
		return err
	}
	if metadata, ok := raw["ResponseMetadata"].(map[string]any); ok {
		if upstreamError, ok := metadata["Error"].(map[string]any); ok && len(upstreamError) > 0 {
			code := 50000
			if numeric, ok := upstreamError["CodeN"].(float64); ok {
				code = int(numeric)
			}
			return &upstreamApplicationError{provider: "official Action", code: code}
		}
	}
	if result == nil {
		return nil
	}
	if nested, ok := raw["Result"]; ok {
		nestedBytes, marshalErr := common.Marshal(nested)
		if marshalErr != nil {
			return marshalErr
		}
		return common.Unmarshal(nestedBytes, result)
	}
	return common.Unmarshal(responseBody, result)
}
