package service

import (
	"bytes"
	"context"
	"crypto/x509"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"strings"
	"testing"

	"github.com/QuantumNous/new-api/clienterrlog"
	logtest "github.com/QuantumNous/new-api/clienterrlog/testutil"
	"github.com/QuantumNous/new-api/model"
	assetadapter "github.com/QuantumNous/new-api/relay/channel/task/seedance/assets"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// 素材合同错误 → 受控阶段/原因码的映射契约；5xx 错误必须返回空阶段。
func TestAssetClientErrorDiagnosticMapping(t *testing.T) {
	mapped := map[error][2]string{
		ErrInvalidAssetRequest: {"request_validation", "invalid_request"},
		ErrAssetURLRequired:    {"source_validation", "source_url_required"},
		&AssetURLTTLInsufficientError{RequiredMinTTLSeconds: 60}: {"source_validation", "url_ttl_insufficient"},
		ErrUnsafeAssetURL:                 {"source_validation", "unsafe_url"},
		ErrReservedAssetGroupName:         {"request_validation", "reserved_asset_group_name"},
		ErrAssetModelNotFound:             {"model_resolution", "model_not_found"},
		ErrAssetNotFound:                  {"upstream_operation", "resource_not_found"},
		ErrUnsupportedAssetType:           {"capability_check", "unsupported_asset_type"},
		ErrUnsupportedAssetOperation:      {"capability_check", "unsupported_asset_operation"},
		ErrAssetLibraryUnsupported:        {"capability_check", "unsupported_asset_operation"},
		ErrDefaultAssetGroupNotConfigured: {"group_resolution", "default_asset_group_not_configured"},
	}
	for err, expected := range mapped {
		stage, reason := AssetClientErrorDiagnostic(fmt.Errorf("wrapped: %w", err))
		assert.Equal(t, expected[0], stage, "%v", err)
		assert.Equal(t, expected[1], reason, "%v", err)
	}
	for _, err := range []error{ErrAssetUpstreamError, ErrAssetUpstreamUnavailable, ErrAssetLibraryUnavailable, errors.New("unknown")} {
		stage, _ := AssetClientErrorDiagnostic(err)
		assert.Empty(t, stage, "%v must not map to a 4xx stage", err)
	}
}

// 枚举类明细：合法值透传、非法值只记 invalid、空值省略。
func TestAssetKindMediaDetailLimitsInvalidInput(t *testing.T) {
	detail := assetKindMediaDetail("general", "image")
	assert.Equal(t, map[string]string{"asset_kind": "general", "media_type": "image"}, detail)

	detail = assetKindMediaDetail("javascript:alert(1)", "not-a-media-type")
	assert.Equal(t, map[string]string{"asset_kind": "invalid", "media_type": "invalid"}, detail)

	assert.Empty(t, assetKindMediaDetail("", ""))
}

// 下载失败分类：按错误类型判定，不依赖错误文本。
func TestClassifyAssetSourceFetchFailure(t *testing.T) {
	tests := []struct {
		name   string
		err    error
		reason string
	}{
		{"dns", &url.Error{Op: "Get", Err: &net.DNSError{Err: "no such host", Name: "src.example", IsNotFound: true}}, "dns_error"},
		{"dns wrapped in url.Error", &net.DNSError{Err: "i/o timeout", Name: "src.example", IsTimeout: true}, "dns_error"},
		{"redirect limit", &url.Error{Op: "Get", Err: errAssetSourceRedirectLimit}, "redirect_rejected"},
		{"redirect unsafe", &url.Error{Op: "Get", Err: errAssetSourceRedirectUnsafe}, "redirect_rejected"},
		{"unknown authority", &url.Error{Op: "Get", Err: x509.UnknownAuthorityError{Cert: &x509.Certificate{}}}, "tls_error"},
		{"timeout", &net.OpError{Op: "dial", Err: os.ErrDeadlineExceeded}, "timeout"},
		{"deadline", context.DeadlineExceeded, "timeout"},
		{"connection", &net.OpError{Op: "dial", Err: errors.New("connect: connection refused")}, "connection_error"},
		{"network", errors.New("some odd failure"), "network_error"},
	}
	for _, tc := range tests {
		assert.Equal(t, tc.reason, classifyAssetSourceFetchFailure(tc.err), tc.name)
	}
}

// 下载失败：保留公开 400 文案不变，同时记录 source_fetch 阶段与分类原因。
func TestOpenFunCloudAssetSourceAttachesFetchFailureReason(t *testing.T) {
	dnsErr := &net.DNSError{Err: "no such host", Name: "source.example", IsNotFound: true}
	client := &http.Client{Transport: funCloudSourceRoundTripper(func(*http.Request) (*http.Response, error) {
		return nil, &url.Error{Op: "Get", URL: "https://source.example/secret?sig=1", Err: dnsErr}
	})}

	var report clienterrlog.Report
	var hasReport bool
	gin.SetMode(gin.TestMode)
	engine := gin.New()
	engine.POST("/v1/assets", logtest.New(t).Middleware(), func(c *gin.Context) {
		clienterrlog.MarkAuthPassed(c, clienterrlog.AuthSourceAPIToken)
		_, err := openFunCloudAssetSourceWithClient(c.Request.Context(), "https://source.example/img", "image", client)
		require.ErrorIs(t, err, ErrInvalidAssetRequest)
		report, hasReport = clienterrlog.PeekReport(c.Request.Context())
		c.JSON(http.StatusBadRequest, gin.H{"error": gin.H{"message": "asset request is invalid", "code": "invalid_request"}})
	})

	response := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodPost, "/v1/assets", nil)
	engine.ServeHTTP(response, request)

	require.Equal(t, http.StatusBadRequest, response.Code)
	assert.NotContains(t, response.Body.String(), "secret")
	require.True(t, hasReport)
	assert.Equal(t, "source_fetch", report.Stage)
	assert.Equal(t, "dns_error", report.Reason)
}

// 源站非 2xx 与 MIME 不匹配：记录状态/归一化 MIME，不记录 URL。
func TestOpenFunCloudAssetSourceAttachesHttpAndMimeReasons(t *testing.T) {
	run := func(contentType string, status int, expectStage, expectReason, expectDetail string) clienterrlog.Report {
		client := &http.Client{Transport: funCloudSourceRoundTripper(func(*http.Request) (*http.Response, error) {
			return &http.Response{
				StatusCode: status,
				Header:     http.Header{"Content-Type": []string{contentType}},
				Body:       io.NopCloser(strings.NewReader("data")),
			}, nil
		})}
		var report clienterrlog.Report
		var hasReport bool
		gin.SetMode(gin.TestMode)
		engine := gin.New()
		engine.POST("/v1/assets", logtest.New(t).Middleware(), func(c *gin.Context) {
			clienterrlog.MarkAuthPassed(c, clienterrlog.AuthSourceAPIToken)
			_, _ = openFunCloudAssetSourceWithClient(c.Request.Context(), "https://source.example/img", "image", client)
			report, hasReport = clienterrlog.PeekReport(c.Request.Context())
			c.Status(http.StatusBadRequest)
		})
		response := httptest.NewRecorder()
		engine.ServeHTTP(response, httptest.NewRequest(http.MethodPost, "/v1/assets", nil))
		require.Equal(t, http.StatusBadRequest, response.Code)
		require.True(t, hasReport)
		assert.Equal(t, expectStage, report.Stage)
		assert.Equal(t, expectReason, report.Reason)
		if expectDetail != "" {
			assert.Equal(t, expectDetail, report.Detail["source_content_type"])
		}
		return report
	}

	httpReport := run("application/json", http.StatusBadGateway, "source_fetch", "source_http_error", "")
	assert.Equal(t, "502", httpReport.Detail["source_status"])

	run("text/plain", http.StatusOK, "content_validation", "content_type_mismatch", "text/plain")
}

// 托管图片：无法解码与超大源分别记录 invalid_image / source_too_large，公共错误不变。
func TestFunCloudHostedCreateAssetAttachesContentReasons(t *testing.T) {
	withHostedAssetDB(t)
	store := newFakeHostedImageStore()
	gin.SetMode(gin.TestMode)

	// expectInvalid：无法解码命中 ErrInvalidAssetRequest（公开 400）；超大源按既有
	// 合同走 ErrAssetUpstreamError（公开 502），两者都保留 content_validation 原因。
	run := func(source []byte, maxBytes int64, expectReason string, expectInvalid bool) clienterrlog.Report {
		var report clienterrlog.Report
		var gotErr error
		engine := gin.New()
		engine.POST("/v1/assets", logtest.New(t).Middleware(), func(c *gin.Context) {
			clienterrlog.MarkAuthPassed(c, clienterrlog.AuthSourceAPIToken)
			ctx := withHostedImageSession(c.Request.Context(), store)
			request := assetRequestForBytes("bad-image", source, maxBytes)
			_, gotErr = newFunCloudHostedMaterialAdapter(3, "customer-model").CreateAsset(ctx, request)
			var ok bool
			report, ok = clienterrlog.PeekReport(c.Request.Context())
			require.True(t, ok)
			c.Status(http.StatusBadRequest)
		})
		response := httptest.NewRecorder()
		engine.ServeHTTP(response, httptest.NewRequest(http.MethodPost, "/v1/assets", nil))
		require.Equal(t, http.StatusBadRequest, response.Code)
		require.Error(t, gotErr)
		if expectInvalid {
			require.ErrorIs(t, gotErr, ErrInvalidAssetRequest)
			assert.Contains(t, gotErr.Error(), "invalid hosted image content")
		} else {
			require.ErrorIs(t, gotErr, errHostedSourceTooLarge)
		}
		assert.Equal(t, "content_validation", report.Stage)
		assert.Equal(t, expectReason, report.Reason)
		return report
	}

	garbage := run([]byte("this is not an image"), funCloudMaterialMaxBytes, "invalid_image", true)
	assert.Empty(t, garbage.Detail)

	oversized := run([]byte(strings.Repeat("x", 64)), 16, "source_too_large", false)
	assert.Empty(t, oversized.Detail)
}

func assetRequestForBytes(name string, data []byte, maxBytes int64) assetadapter.AssetRequest {
	return assetadapter.AssetRequest{
		URL:            "https://source.example/" + name,
		Name:           name,
		MediaType:      "image",
		Source:         bytes.NewReader(data),
		SourceType:     "image/png",
		SourceMaxBytes: maxBytes,
	}
}

func TestAssetBusinessFailuresRetainResolvedContext(t *testing.T) {
	db := withAssetGroupPolicyDB(t)
	require.NoError(t, db.AutoMigrate(&model.ChannelAssetScopeIdentity{}))
	channel := createMoxingAssetPolicyChannel(t, db, "https://provider.invalid")
	providerCalls := 0
	withAssetPolicyHTTPClient(t, func(*http.Request) (*http.Response, error) {
		providerCalls++
		return &http.Response{StatusCode: 404, Header: http.Header{"Content-Type": []string{"application/json"}}, Body: io.NopCloser(strings.NewReader(`{"error":{"message":"private upstream response"}}`))}, nil
	})
	for _, tc := range []struct {
		name, reason string
		expected     error
		run          func(context.Context) error
		resolved     bool
	}{
		{"model missing", "model_not_found", ErrAssetModelNotFound, func(ctx context.Context) error {
			_, err := GetRemoteAsset(ctx, "default", 7, "missing-model", "opaque-secret")
			return err
		}, false},
		{"default group missing", "default_asset_group_not_configured", ErrDefaultAssetGroupNotConfigured, func(ctx context.Context) error {
			_, err := CreateRemoteAsset(ctx, "default", 7, generalAssetPolicyRequest(""))
			return err
		}, true},
		{"upstream not found", "resource_not_found", ErrAssetNotFound, func(ctx context.Context) error {
			_, err := GetRemoteAsset(ctx, "default", 7, "customer-model", "opaque-secret")
			return err
		}, true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			capture := logtest.New(t)
			engine := gin.New()
			engine.Use(capture.Middleware())
			engine.GET("/v1/assets/:id", func(c *gin.Context) {
				c.Set("id", 7)
				clienterrlog.MarkAuthPassed(c, clienterrlog.AuthSourceAPIToken)
				err := tc.run(c.Request.Context())
				require.ErrorIs(t, err, tc.expected)
				AttachAssetClientError(c.Request.Context(), err, clienterrlog.Report{})
				status := 404
				if errors.Is(err, ErrDefaultAssetGroupNotConfigured) {
					status = 409
				}
				c.Status(status)
			})
			r := httptest.NewRecorder()
			engine.ServeHTTP(r, httptest.NewRequest("GET", "/v1/assets/opaque-secret", nil))
			log := capture.String()
			assert.Contains(t, log, "reason="+tc.reason)
			if tc.resolved {
				assert.Contains(t, log, "model=customer-model")
				assert.Contains(t, log, fmt.Sprintf("channel_id=%d", channel.Id))
				assert.Contains(t, log, "protocol=moxing_volc_assets_v1")
			} else {
				assert.Contains(t, log, "model=missing-model")
				assert.NotContains(t, log, "channel_id=")
			}
			assert.NotContains(t, log, "opaque-secret")
			assert.NotContains(t, log, "private upstream")
			assert.NotContains(t, log, "provider.invalid")
		})
	}
	assert.Equal(t, 1, providerCalls)
}
