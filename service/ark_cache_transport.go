package service

import (
	"bytes"
	"context"
	"crypto/sha256"
	"errors"
	"fmt"
	"io"
	"net/http"
	"regexp"
	"strings"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/setting"
)

var arkCacheProviderID = regexp.MustCompile(`^resp_[a-zA-Z0-9_-]{1,180}$`)
var errArkCacheHTTPResponse = errors.New("Ark cache request returned an unsuccessful HTTP status")
var arkCacheClient = func() *http.Client {
	protocols := new(http.Protocols)
	protocols.SetHTTP1(true)
	return &http.Client{Timeout: 60 * time.Second,
		Transport:     WithAPIRequestEvidenceTransport(&http.Transport{Protocols: protocols, DisableKeepAlives: true}),
		CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse },
	}
}()

type ArkCacheReply struct {
	ID        string            `json:"id"`
	Object    string            `json:"object"`
	Status    string            `json:"status"`
	Model     string            `json:"model"`
	CreatedAt int64             `json:"created_at"`
	ExpireAt  int64             `json:"expire_at"`
	Deleted   bool              `json:"deleted"`
	Output    []ArkCacheOutput  `json:"output"`
	Usage     common.RawMessage `json:"usage"`
	Store     bool              `json:"store"`
	Caching   struct {
		Type string `json:"type"`
	} `json:"caching"`
	PreviousResponseID string `json:"previous_response_id"`
	ErrorCode          string `json:"-"`
}

type ArkCacheOutput struct {
	Type    string `json:"type"`
	ID      string `json:"id"`
	Status  string `json:"status"`
	Role    string `json:"role"`
	Content []struct {
		Type    string `json:"type"`
		Text    string `json:"text,omitempty"`
		Refusal string `json:"refusal,omitempty"`
	} `json:"content"`
}

func ArkCacheChannel(ctx context.Context, id int, credential string) (*model.Channel, string) {
	channel, err := model.FindArkCacheChannel(model.DB.WithContext(ctx), id)
	if err != nil || channel == nil {
		return nil, ""
	}
	key := strings.TrimSpace(channel.Key)
	if key == "" || (credential != "" && credential != ArkCacheDigest(key)) {
		return nil, ""
	}
	return channel, key
}

// Cleanup and receipt reads use the frozen endpoint and key fingerprint.
// Channel admission settings may change while older responses still exist.
func ArkCacheResourceCredential(row model.ArkPrefixCache) string {
	if row.MeterVersion == model.ArkSessionLocalMeterVersion && !setting.ArkCacheIsolatedLocalDatabase() {
		return ""
	}
	channel, err := model.GetChannelById(row.ChannelID, true)
	if err != nil || channel.Type != constant.ChannelTypeVolcEngine || channel.ChannelInfo.IsMultiKey || row.Credential == "" {
		return ""
	}
	key := strings.TrimSpace(channel.Key)
	if key == "" || ArkCacheDigest(key) != row.Credential {
		return ""
	}
	return key
}

func SendArkCacheRequest(ctx context.Context, key, method, id string, request any) (ArkCacheReply, int, error) {
	var response ArkCacheReply
	url := setting.ArkCacheRequestBaseURL() + "/api/v3/responses"
	var body io.Reader
	if request != nil {
		encoded, err := common.Marshal(request)
		if err != nil {
			return response, 0, err
		}
		body = bytes.NewReader(encoded)
	}
	if id != "" {
		if !arkCacheProviderID.MatchString(id) {
			return response, 0, model.ErrArkCacheConflict
		}
		url += "/" + id
	}
	req, err := http.NewRequestWithContext(ctx, method, url, body)
	if err != nil {
		return response, 0, err
	}
	req.GetBody = nil
	req.Header.Set("Authorization", "Bearer "+key)
	if request != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	upstream, err := arkCacheClient.Do(req)
	if err != nil {
		return response, 0, err
	}
	defer upstream.Body.Close()
	if upstream.StatusCode != http.StatusOK {
		if upstream.StatusCode >= 400 && upstream.StatusCode < 500 {
			var errorEnvelope struct {
				Error struct {
					Code string `json:"code"`
				} `json:"error"`
			}
			if common.DecodeJsonBody(io.LimitReader(upstream.Body, 4096), &errorEnvelope) == nil {
				response.ErrorCode = errorEnvelope.Error.Code
			}
		}
		return response, upstream.StatusCode, errArkCacheHTTPResponse
	}
	err = common.DecodeJsonBody(io.LimitReader(upstream.Body, 4<<20), &response)
	return response, upstream.StatusCode, err
}

func ValidArkCacheProviderID(id string) bool { return arkCacheProviderID.MatchString(id) }

func IsArkCacheMissing(reply ArkCacheReply, status int) bool {
	return status == http.StatusNotFound && reply.ErrorCode == "ResourceNotFound"
}

// ArkCacheCreateRejected recognizes provider admission errors that establish
// that this POST was rejected. Untyped or ambiguous failures stay unknown.
func ArkCacheCreateRejected(reply ArkCacheReply, status int) bool {
	code := reply.ErrorCode
	switch status {
	case http.StatusBadRequest:
		return code == "InvalidParameter" || code == "MissingParameter" || strings.HasPrefix(code, "InvalidParameter.")
	case http.StatusUnauthorized:
		return code == "AuthenticationError" || code == "InvalidAccountStatus"
	case http.StatusForbidden:
		return code == "AccessDenied" || strings.HasPrefix(code, "AccessDenied.") ||
			code == "OperationDenied.ServiceNotOpen" || code == "OperationDenied.ServiceOverdue" || code == "OperationDenied.PermissionDenied"
	case http.StatusNotFound:
		return IsArkCacheMissing(reply, status) || code == "InvalidEndpointOrModel.NotFound"
	case http.StatusTooManyRequests:
		return code == "RequestBurstTooFast" || code == "ServerOverloaded" || code == "AccountRateLimitExceeded" ||
			code == "ModelAccountRpmRateLimitExceeded" || code == "ModelAccountTpmRateLimitExceeded" ||
			strings.HasPrefix(code, "RateLimitExceeded.")
	default:
		return false
	}
}

func ArkCacheDigest(value string) string { return fmt.Sprintf("%x", sha256.Sum256([]byte(value))) }
