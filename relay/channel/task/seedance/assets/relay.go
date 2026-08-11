// Package assets implements Seedance asset-library protocols.
package assets

import (
	"context"
	"net/http"
	"net/url"

	"github.com/QuantumNous/new-api/dto"
)

type RelayAdapter struct{ client }

func NewRelayAdapter(baseURL, apiKey string, httpClient HTTPDoer) *RelayAdapter {
	return &RelayAdapter{client: newClient(baseURL, apiKey, httpClient)}
}

func (*RelayAdapter) Profile() dto.AssetUpstreamProfile { return dto.AssetUpstreamProfileRelay }
func (*RelayAdapter) Supports(kind, mediaType string) bool {
	return kind == "general" && (mediaType == "image" || mediaType == "video" || mediaType == "audio")
}

func (a *RelayAdapter) CheckConnectivity(ctx context.Context) error {
	return a.request(ctx, http.MethodPost, "/assets/list", map[string]any{
		"page_number": 1,
		"page_size":   1,
	}, nil)
}

type relayAssetResponse struct {
	UUID         string `json:"uuid"`
	UpstreamID   string `json:"upstream_id"`
	AssetURL     string `json:"asset_url"`
	Status       string `json:"status"`
	ErrorCode    string `json:"error_code"`
	ErrorMessage string `json:"error_message"`
}

func (a *RelayAdapter) CreateAsset(ctx context.Context, req AssetRequest) (AssetResult, error) {
	var response relayAssetResponse
	err := a.request(ctx, http.MethodPost, "/assets", map[string]any{"url": req.URL, "name": req.Name, "asset_type": normalizedMediaType(req.MediaType)}, &response)
	return normalizeRelayAsset(response), err
}

func (a *RelayAdapter) GetAsset(ctx context.Context, resourceID string) (AssetResult, error) {
	var response relayAssetResponse
	err := a.request(ctx, http.MethodGet, "/assets/"+url.PathEscape(resourceID), nil, &response)
	return normalizeRelayAsset(response), err
}

func (a *RelayAdapter) UpdateAsset(ctx context.Context, resourceID, name string) (AssetResult, error) {
	var response relayAssetResponse
	err := a.request(ctx, http.MethodPost, "/assets/"+url.PathEscape(resourceID)+"/update", map[string]string{"name": name}, &response)
	if response.UUID == "" {
		response.UUID = resourceID
	}
	return normalizeRelayAsset(response), err
}

func (a *RelayAdapter) DeleteAsset(ctx context.Context, resourceID string) error {
	err := a.request(ctx, http.MethodPost, "/assets/"+url.PathEscape(resourceID)+"/delete", nil, nil)
	if upstreamNotFound(err) {
		return nil
	}
	return err
}

func normalizeRelayAsset(response relayAssetResponse) AssetResult {
	result := AssetResult{ResourceID: response.UUID, BusinessID: response.UpstreamID, ErrorCode: response.ErrorCode, ErrorMessage: response.ErrorMessage}
	if response.ErrorCode != "" {
		result.Status = "failed"
		return result
	}
	switch response.Status {
	case "Active":
		result.Status = "active"
		result.ReferenceType = "asset_uri_id"
		if response.UpstreamID != "" {
			result.ReferenceValue = response.UpstreamID
		} else {
			result.ReferenceValue = response.UUID
		}
	case "Failed":
		result.Status = "failed"
	case "Pending":
		result.Status = "pending"
	default:
		result.Status = "processing"
	}
	return result
}
