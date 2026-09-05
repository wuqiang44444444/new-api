// Package assets implements Seedance asset-library protocols.
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

var ErrAssetOperationUnsupported = errors.New("asset upstream operation is not supported by the verified protocol")

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
	Name       string
	Status     string
	RequestID  string
}

type AssetRequest struct {
	GroupResourceID string
	URL             string
	Name            string
	MediaType       string
	Source          io.Reader
	SourceType      string
	SourceMaxBytes  int64
	SourceFilename  string
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
	SessionID string
	GroupID   string
	H5URL     string
	Status    string
	ExpiresAt int64
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
}

type GroupSearchAdapter interface {
	ListGroups(ctx context.Context, req GroupListRequest) ([]GroupResult, int, error)
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

type upstreamStringApplicationError struct {
	provider   string
	code       string
	definitive bool
	notFound   bool
}

func (e *upstreamStringApplicationError) Error() string {
	if e.code == "" {
		return fmt.Sprintf("%s asset upstream rejected request", e.provider)
	}
	return fmt.Sprintf("%s asset upstream rejected request with code %s", e.provider, e.code)
}

func (e *upstreamApplicationError) Error() string {
	return fmt.Sprintf("%s asset upstream rejected request with code %d", e.provider, e.code)
}

func IsDefinitiveUpstreamRejection(err error) bool {
	var statusErr *upstreamHTTPError
	if errors.As(err, &statusErr) {
		if statusErr.StatusCode == http.StatusRequestTimeout || statusErr.StatusCode == http.StatusTooEarly || statusErr.StatusCode == http.StatusTooManyRequests {
			return false
		}
		return statusErr.StatusCode >= http.StatusBadRequest && statusErr.StatusCode < http.StatusInternalServerError
	}
	var applicationErr *upstreamApplicationError
	if errors.As(err, &applicationErr) {
		if applicationErr.code == http.StatusRequestTimeout || applicationErr.code == http.StatusTooEarly || applicationErr.code == http.StatusTooManyRequests {
			return false
		}
		return applicationErr.definitive || applicationErr.code >= 400 && applicationErr.code < 500 || applicationErr.code >= 40000 && applicationErr.code < 50000
	}
	var stringErr *upstreamStringApplicationError
	if !errors.As(err, &stringErr) {
		return false
	}
	return stringErr.definitive
}

func (e *upstreamHTTPError) Error() string {
	if e.ProviderCode != "" {
		return fmt.Sprintf("asset upstream returned HTTP %d (%s)", e.StatusCode, e.ProviderCode)
	}
	return fmt.Sprintf("asset upstream returned HTTP %d", e.StatusCode)
}

func upstreamNotFound(err error) bool {
	var statusErr *upstreamHTTPError
	if errors.As(err, &statusErr) {
		return statusErr.StatusCode == http.StatusNotFound
	}
	var stringErr *upstreamStringApplicationError
	return errors.As(err, &stringErr) && stringErr.notFound
}

func IsUpstreamNotFound(err error) bool {
	return upstreamNotFound(err)
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
		return classifyTransportError(AssetStageWaitResponse, err)
	}
	defer resp.Body.Close()
	if resp.StatusCode < http.StatusOK || resp.StatusCode >= http.StatusMultipleChoices {
		_, _ = io.Copy(io.Discard, io.LimitReader(resp.Body, 4096))
		return &upstreamHTTPError{StatusCode: resp.StatusCode}
	}
	if result == nil {
		return nil
	}
	if err := common.DecodeJson(resp.Body, result); err != nil {
		return invalidUpstreamResponse(err)
	}
	return nil
}

func normalizedMediaType(value string) string {
	value = strings.ToLower(strings.TrimSpace(value))
	if value == "" {
		return value
	}
	return strings.ToUpper(value[:1]) + value[1:]
}
