package assets

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/dto"
)

var ErrGroupDeletionUnsupported = errors.New("asset upstream group deletion is not supported by the verified protocol")

type HTTPDoer interface {
	Do(req *http.Request) (*http.Response, error)
}

type GroupRequest struct {
	Name        string
	Description string
	GroupType   string
}

type GroupResult struct {
	ResourceID string
	BusinessID string
	Status     string
	RequestID  string
}

type AssetRequest struct {
	GroupResourceID string
	URL             string
	Name            string
	MediaType       string
}

type AssetResult struct {
	ResourceID     string
	BusinessID     string
	ReferenceType  string
	ReferenceValue string
	Status         string
	ErrorCode      string
	ErrorMessage   string
	RequestID      string
}

type VerificationRequest struct {
	RedirectURL string
	ProjectName string
}

type VerificationResult struct {
	SessionID             string
	Handle                string
	EncryptedHandle       string
	EncryptedH5URL        string
	VerificationTokenHash string
	GroupID               string
	H5URL                 string
	Status                string
	ExpiresAt             int64
}

type AssetListRequest struct {
	GroupIDs  []string
	GroupType string
	Statuses  []string
	Name      string
	Page      int
	PageSize  int
}

type GroupListRequest struct {
	GroupIDs  []string
	GroupType string
	Name      string
	Page      int
	PageSize  int
}

type ReconciliationAdapter interface {
	ListAssets(ctx context.Context, req AssetListRequest) ([]AssetResult, int, error)
	ListGroups(ctx context.Context, req GroupListRequest) ([]GroupResult, int, error)
}

type ConnectivityAdapter interface {
	CheckConnectivity(ctx context.Context) error
}

type Adapter interface {
	Profile() dto.AssetUpstreamProfile
	Supports(kind, mediaType string) bool
	CreateAsset(ctx context.Context, req AssetRequest) (AssetResult, error)
	GetAsset(ctx context.Context, resourceID string) (AssetResult, error)
	UpdateAsset(ctx context.Context, resourceID, name string) (AssetResult, error)
	DeleteAsset(ctx context.Context, resourceID string) error
}

type GroupAdapter interface {
	CreateGroup(ctx context.Context, req GroupRequest) (GroupResult, error)
	GetGroup(ctx context.Context, resourceID string) (GroupResult, error)
	UpdateGroup(ctx context.Context, resourceID string, req GroupRequest) (GroupResult, error)
	DeleteGroup(ctx context.Context, resourceID string) error
}

type VerificationAdapter interface {
	Adapter
	GroupAdapter
	CreateVerificationSession(ctx context.Context, req VerificationRequest) (VerificationResult, error)
	GetVerificationSession(ctx context.Context, sessionID string) (VerificationResult, error)
	GetVerificationResult(ctx context.Context, sessionID string) (VerificationResult, error)
}

type client struct {
	baseURL string
	apiKey  string
	http    HTTPDoer
}

type upstreamHTTPError struct {
	StatusCode   int
	ProviderCode string
}

type upstreamApplicationError struct {
	provider   string
	code       int
	definitive bool
}

func (e *upstreamApplicationError) Error() string {
	return fmt.Sprintf("%s asset upstream rejected request with code %d", e.provider, e.code)
}

func IsDefinitiveUpstreamRejection(err error) bool {
	if statusErr, ok := err.(*upstreamHTTPError); ok {
		if statusErr.StatusCode == http.StatusRequestTimeout || statusErr.StatusCode == http.StatusTooEarly || statusErr.StatusCode == http.StatusTooManyRequests {
			return false
		}
		return statusErr.StatusCode >= http.StatusBadRequest && statusErr.StatusCode < http.StatusInternalServerError
	}
	applicationErr, ok := err.(*upstreamApplicationError)
	if !ok {
		return false
	}
	if applicationErr.code == http.StatusRequestTimeout || applicationErr.code == http.StatusTooEarly || applicationErr.code == http.StatusTooManyRequests {
		return false
	}
	return applicationErr.definitive || applicationErr.code >= 400 && applicationErr.code < 500 || applicationErr.code >= 40000 && applicationErr.code < 50000
}

func (e *upstreamHTTPError) Error() string {
	if e.ProviderCode != "" {
		return fmt.Sprintf("asset upstream returned HTTP %d (%s)", e.StatusCode, e.ProviderCode)
	}
	return fmt.Sprintf("asset upstream returned HTTP %d", e.StatusCode)
}

func upstreamNotFound(err error) bool {
	statusErr, ok := err.(*upstreamHTTPError)
	return ok && statusErr.StatusCode == http.StatusNotFound
}

func newClient(baseURL, apiKey string, httpClient HTTPDoer) client {
	if httpClient == nil {
		httpClient = http.DefaultClient
	}
	return client{baseURL: strings.TrimRight(baseURL, "/"), apiKey: apiKey, http: httpClient}
}

func (c client) request(ctx context.Context, method, path string, body any, result any) error {
	var reader io.Reader
	if body != nil {
		data, err := common.Marshal(body)
		if err != nil {
			return err
		}
		reader = bytes.NewReader(data)
	}
	req, err := http.NewRequestWithContext(ctx, method, c.baseURL+path, reader)
	if err != nil {
		return err
	}
	req.Header.Set("Authorization", "Bearer "+c.apiKey)
	req.Header.Set("Accept", "application/json")
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	resp, err := c.http.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode < http.StatusOK || resp.StatusCode >= http.StatusMultipleChoices {
		_, _ = io.Copy(io.Discard, io.LimitReader(resp.Body, 4096))
		return &upstreamHTTPError{StatusCode: resp.StatusCode}
	}
	if result == nil {
		return nil
	}
	return common.DecodeJson(resp.Body, result)
}

func normalizedMediaType(value string) string {
	value = strings.ToLower(strings.TrimSpace(value))
	if value == "" {
		return value
	}
	return strings.ToUpper(value[:1]) + value[1:]
}
