package assets

import (
	"bytes"
	"context"
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"sort"
	"strings"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/dto"
)

const cmccAICCV2BaseURL = "https://ecloud.10086.cn/api/openapi-maas/exp/aicc/v2"
const cmccAICCMaxResponseBytes = 4 << 20

type CMCCAICCV2Adapter struct {
	baseURL   string
	accessKey string
	secretKey string
	http      HTTPDoer
	now       func() time.Time
	readNonce func([]byte) (int, error)
}

type cmccEnvelope struct {
	RequestID    string          `json:"requestId"`
	State        string          `json:"state"`
	ErrorCode    string          `json:"errorCode"`
	ErrorMessage string          `json:"errorMessage"`
	Body         json.RawMessage `json:"body"`
}

type cmccAsset struct {
	AssetID      string `json:"assetId"`
	GroupID      string `json:"groupId"`
	AssetName    string `json:"assetName"`
	AssetType    string `json:"assetType"`
	Status       string `json:"status"`
	ErrorMessage string `json:"errorMessage"`
}

type cmccGroup struct {
	GroupID     string `json:"groupId"`
	GroupType   string `json:"groupType"`
	GroupName   string `json:"groupName"`
	Description string `json:"description"`
}

func NewCMCCAICCV2Adapter(credential string, httpClient HTTPDoer) (*CMCCAICCV2Adapter, error) {
	parts := strings.SplitN(strings.TrimSpace(credential), "|", 2)
	if len(parts) != 2 || strings.TrimSpace(parts[0]) == "" || strings.TrimSpace(parts[1]) == "" {
		return nil, fmt.Errorf("CMCC AICC credential must use ACCESS_KEY|SECRET_KEY")
	}
	if httpClient == nil {
		httpClient = http.DefaultClient
	}
	return &CMCCAICCV2Adapter{
		baseURL: cmccAICCV2BaseURL, accessKey: strings.TrimSpace(parts[0]),
		secretKey: strings.TrimSpace(parts[1]), http: httpClient, now: time.Now, readNonce: rand.Read,
	}, nil
}

func (*CMCCAICCV2Adapter) Profile() dto.AssetUpstreamProfile {
	return dto.AssetUpstreamProfileCMCCAICCV2
}

func (*CMCCAICCV2Adapter) Supports(kind, mediaType string) bool {
	if kind != "general" && kind != "real_person" {
		return false
	}
	return mediaType == "image" || mediaType == "video" || mediaType == "audio"
}

func (*CMCCAICCV2Adapter) RequiresAssetGroup(kind, mediaType string) bool {
	return (kind == "general" || kind == "real_person") &&
		(mediaType == "image" || mediaType == "video" || mediaType == "audio")
}

func (a *CMCCAICCV2Adapter) CheckConnectivity(ctx context.Context) error {
	var result struct {
		Total int64 `json:"total"`
	}
	return a.request(ctx, http.MethodPost, "/asset/query", map[string]any{
		"pageNo": 1, "pageSize": 1, "groupType": "AIGC",
	}, &result)
}

func (a *CMCCAICCV2Adapter) CreateAsset(ctx context.Context, request AssetRequest) (AssetResult, error) {
	var id string
	err := a.request(ctx, http.MethodPost, "/asset", map[string]any{
		"groupId": request.GroupResourceID, "assetName": request.Name,
		"assetUrl": request.URL, "assetType": normalizedMediaType(request.MediaType),
	}, &id)
	return AssetResult{ResourceID: id, BusinessID: id, Status: "processing"}, err
}

func (a *CMCCAICCV2Adapter) GetAsset(ctx context.Context, resourceID string) (AssetResult, error) {
	var result cmccAsset
	err := a.request(ctx, http.MethodGet, "/asset/"+url.PathEscape(resourceID), nil, &result)
	return normalizeCMCCAsset(result), err
}

func (a *CMCCAICCV2Adapter) UpdateAsset(ctx context.Context, resourceID, name string) (AssetResult, error) {
	var result cmccAsset
	err := a.request(ctx, http.MethodPut, "/asset/"+url.PathEscape(resourceID), map[string]any{"assetName": name}, &result)
	if result.AssetID == "" {
		result.AssetID = resourceID
	}
	return normalizeCMCCAsset(result), err
}

func (a *CMCCAICCV2Adapter) DeleteAsset(ctx context.Context, resourceID string) error {
	return a.delete(ctx, "/asset/"+url.PathEscape(resourceID))
}

func (a *CMCCAICCV2Adapter) CreateGroup(ctx context.Context, request GroupRequest) (GroupResult, error) {
	var result cmccGroup
	err := a.request(ctx, http.MethodPost, "/asset-group/", map[string]any{
		"groupType": "AIGC", "groupName": request.Name, "description": request.Description,
	}, &result)
	return normalizeCMCCGroup(result), err
}

func (a *CMCCAICCV2Adapter) GetGroup(ctx context.Context, resourceID string) (GroupResult, error) {
	var result cmccGroup
	err := a.request(ctx, http.MethodGet, "/asset-group/"+url.PathEscape(resourceID), nil, &result)
	return normalizeCMCCGroup(result), err
}

func (a *CMCCAICCV2Adapter) DeleteGroup(ctx context.Context, resourceID string) error {
	return a.delete(ctx, "/asset-group/"+url.PathEscape(resourceID))
}

func (a *CMCCAICCV2Adapter) CreateVerificationSession(ctx context.Context, request VerificationRequest) (VerificationResult, error) {
	if strings.TrimSpace(request.RedirectURL) != "" {
		return VerificationResult{}, ErrAssetOperationUnsupported
	}
	var result struct {
		BytedToken string `json:"bytedToken"`
		H5Link     string `json:"h5Link"`
		ExpiresIn  int64  `json:"expiresIn"`
	}
	err := a.request(ctx, http.MethodPost, "/real-person-auth/sessions", nil, &result)
	expiresAt := int64(0)
	if result.ExpiresIn > 0 {
		expiresAt = a.now().UTC().Add(time.Duration(result.ExpiresIn) * time.Second).Unix()
	}
	return VerificationResult{
		SessionID: result.BytedToken, H5URL: result.H5Link, Status: "verifying", ExpiresAt: expiresAt,
	}, err
}

func (a *CMCCAICCV2Adapter) GetVerificationSession(ctx context.Context, sessionID string) (VerificationResult, error) {
	return a.GetVerificationResult(ctx, sessionID)
}

func (a *CMCCAICCV2Adapter) GetVerificationResult(ctx context.Context, sessionID string) (VerificationResult, error) {
	var groupID string
	err := a.request(ctx, http.MethodPost, "/real-person-auth/asset-group/by-byted-token", map[string]any{
		"bytedToken": sessionID,
	}, &groupID)
	status := "verifying"
	if strings.TrimSpace(groupID) != "" {
		status = "active"
	}
	return VerificationResult{GroupID: groupID, Status: status}, err
}

func (a *CMCCAICCV2Adapter) delete(ctx context.Context, path string) error {
	var deleted bool
	if err := a.request(ctx, http.MethodDelete, path, nil, &deleted); err != nil {
		return err
	}
	if !deleted {
		return &upstreamStringApplicationError{provider: "CMCC AICC", code: "delete_rejected", definitive: true}
	}
	return nil
}

func (a *CMCCAICCV2Adapter) request(ctx context.Context, method, path string, body any, result any) error {
	var payload []byte
	var err error
	if body != nil {
		payload, err = common.Marshal(body)
		if err != nil {
			return err
		}
	}
	endpoint, err := url.Parse(a.baseURL + path)
	if err != nil {
		return err
	}
	query, err := a.signedQuery(method, endpoint.EscapedPath(), a.now().UTC())
	if err != nil {
		return err
	}
	endpoint.RawQuery = query
	req, err := http.NewRequestWithContext(ctx, method, endpoint.String(), bytes.NewReader(payload))
	if err != nil {
		return err
	}
	req.Header.Set("Accept", "application/json")
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	resp, err := a.http.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	responseBody, err := io.ReadAll(io.LimitReader(resp.Body, cmccAICCMaxResponseBytes+1))
	if err != nil {
		return err
	}
	if len(responseBody) > cmccAICCMaxResponseBytes {
		return fmt.Errorf("CMCC AICC response exceeds size limit")
	}
	if resp.StatusCode < http.StatusOK || resp.StatusCode >= http.StatusMultipleChoices {
		return &upstreamHTTPError{StatusCode: resp.StatusCode}
	}
	var envelope cmccEnvelope
	if err := common.Unmarshal(responseBody, &envelope); err != nil {
		return err
	}
	if !strings.EqualFold(strings.TrimSpace(envelope.State), "OK") {
		code := strings.TrimSpace(envelope.ErrorCode)
		upperCode := strings.ToUpper(code)
		return &upstreamStringApplicationError{
			provider: "CMCC AICC", code: code, definitive: true,
			notFound: strings.Contains(upperCode, "NOT_FOUND") || strings.Contains(upperCode, "NOTFOUND"),
		}
	}
	if result == nil {
		return nil
	}
	if len(envelope.Body) == 0 || bytes.Equal(bytes.TrimSpace(envelope.Body), []byte("null")) {
		return &upstreamStringApplicationError{provider: "CMCC AICC", code: "missing_body"}
	}
	return common.Unmarshal(envelope.Body, result)
}

func (a *CMCCAICCV2Adapter) signedQuery(method, escapedPath string, now time.Time) (string, error) {
	nonce := make([]byte, 16)
	if _, err := a.readNonce(nonce); err != nil {
		return "", err
	}
	values := map[string]string{
		"AccessKey":        a.accessKey,
		"Timestamp":        now.Format("2006-01-02T15:04:05Z"),
		"SignatureMethod":  "HmacSHA256",
		"SignatureVersion": "V2.0",
		"SignatureNonce":   hex.EncodeToString(nonce),
	}
	canonical := cmccCanonicalQuery(values)
	queryHash := sha256.Sum256([]byte(canonical))
	stringToSign := strings.ToUpper(method) + "\n" + cmccPercentEncode(escapedPath) + "\n" + hex.EncodeToString(queryHash[:])
	mac := hmac.New(sha256.New, []byte("BC_SIGNATURE&"+a.secretKey))
	_, _ = mac.Write([]byte(stringToSign))
	values["Signature"] = hex.EncodeToString(mac.Sum(nil))
	return cmccCanonicalQuery(values), nil
}

func cmccCanonicalQuery(values map[string]string) string {
	keys := make([]string, 0, len(values))
	for key := range values {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	parts := make([]string, 0, len(keys))
	for _, key := range keys {
		parts = append(parts, cmccPercentEncode(key)+"="+cmccPercentEncode(values[key]))
	}
	return strings.Join(parts, "&")
}

func cmccPercentEncode(value string) string {
	const upperHex = "0123456789ABCDEF"
	var encoded strings.Builder
	for i := 0; i < len(value); i++ {
		character := value[i]
		if character >= 'a' && character <= 'z' || character >= 'A' && character <= 'Z' ||
			character >= '0' && character <= '9' || character == '-' || character == '_' || character == '.' || character == '~' {
			encoded.WriteByte(character)
			continue
		}
		encoded.WriteByte('%')
		encoded.WriteByte(upperHex[character>>4])
		encoded.WriteByte(upperHex[character&15])
	}
	return encoded.String()
}

func normalizeCMCCAsset(asset cmccAsset) AssetResult {
	result := AssetResult{
		ResourceID: asset.AssetID, BusinessID: asset.AssetID,
		Status: strings.ToLower(strings.TrimSpace(asset.Status)), ErrorMessage: asset.ErrorMessage,
	}
	if strings.EqualFold(asset.Status, "ACTIVE") {
		result.ReferenceType = "asset_uri_id"
		result.ReferenceValue = asset.AssetID
	}
	if strings.EqualFold(asset.Status, "FAILED") {
		result.ErrorCode = "provider_failed"
	}
	return result
}

func normalizeCMCCGroup(group cmccGroup) GroupResult {
	return GroupResult{ResourceID: group.GroupID, BusinessID: group.GroupID, Status: "active"}
}
