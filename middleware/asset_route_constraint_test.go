package middleware

import (
	"bytes"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"net/url"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/setting/asset_setting"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func setAssetLibraryEnabledForTest(t *testing.T, enabled bool) {
	t.Helper()
	settings := asset_setting.GetBusinessSetting()
	original := settings.Enabled
	settings.Enabled = enabled
	t.Cleanup(func() { settings.Enabled = original })
}

func TestCollectPlatformAssetReferencesUsesCanonicalTaskFields(t *testing.T) {
	body := map[string]any{
		"image":  "asset://ast_0123456789abcdefghijklmnopqrstuv",
		"images": []any{"https://example.com/a.png", "asset://ast_ABCDEFGHIJKLMNOPQRSTUVWXYZ012345"},
		"metadata": map[string]any{"content": []any{
			map[string]any{"image_url": map[string]any{"url": "asset://ast_0123456789abcdefghijklmnopqrstuv"}},
			map[string]any{"video_url": map[string]any{"url": "asset://ast_abcdefghijklmnopqrstuvwxyzABCDEF"}},
		}},
	}
	references, err := collectPlatformAssetReferences(body)
	require.NoError(t, err)
	assert.Equal(t, map[string]struct{}{
		"ast_0123456789abcdefghijklmnopqrstuv": {},
		"ast_ABCDEFGHIJKLMNOPQRSTUVWXYZ012345": {},
		"ast_abcdefghijklmnopqrstuvwxyzABCDEF": {},
	}, references)
}

func TestAssetRouteConstraintRejectsReferencesWhenLibraryDisabled(t *testing.T) {
	setAssetLibraryEnabledForTest(t, false)
	gin.SetMode(gin.TestMode)
	recorder := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(recorder)
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/video/generations", bytes.NewBufferString(`{"image":"asset://ast_0123456789abcdefghijklmnopqrstuv"}`))
	c.Request.Header.Set("Content-Type", "application/json")

	AssetRouteConstraint()(c)

	assert.Equal(t, http.StatusServiceUnavailable, recorder.Code)
	assert.Contains(t, recorder.Body.String(), "asset_library_disabled")
}

func TestAssetRouteConstraintUsesOfficialVideoErrorEnvelopes(t *testing.T) {
	setAssetLibraryEnabledForTest(t, false)
	tests := []struct {
		name     string
		protocol string
		wantCode float64
	}{
		{name: "Kling", protocol: model.TaskClientProtocolKlingV1, wantCode: 5001},
		{name: "Jimeng", protocol: model.TaskClientProtocolJimeng, wantCode: 50500},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			recorder := httptest.NewRecorder()
			c, _ := gin.CreateTestContext(recorder)
			c.Request = httptest.NewRequest(http.MethodPost, "/v1/video/generations", bytes.NewBufferString(
				`{"image":"asset://ast_0123456789abcdefghijklmnopqrstuv"}`,
			))
			c.Request.Header.Set("Content-Type", "application/json")
			common.SetContextKey(c, constant.ContextKeyTaskClientProtocol, test.protocol)

			AssetRouteConstraint()(c)

			assert.Equal(t, http.StatusServiceUnavailable, recorder.Code)
			var body map[string]any
			require.NoError(t, common.Unmarshal(recorder.Body.Bytes(), &body))
			assert.Equal(t, test.wantCode, body["code"])
			assert.NotContains(t, body, "error")
			assert.Contains(t, body, "data")
			if test.protocol == model.TaskClientProtocolJimeng {
				assert.Equal(t, test.wantCode, body["status"])
			}
		})
	}
}

func TestCollectPlatformAssetReferencesRejectsNonPlatformReferences(t *testing.T) {
	for _, reference := range []string{"asset://asset-upstream-id", "ASSET://ast_0123456789abcdefghijklmnopqrstuv"} {
		_, err := collectPlatformAssetReferences(map[string]any{"image": reference})
		require.Error(t, err)
		assert.Contains(t, err.Error(), "invalid platform asset reference")
	}
}

func TestCollectPlatformAssetReferencesRejectsMalformedMetadata(t *testing.T) {
	_, err := collectPlatformAssetReferences(map[string]any{"metadata": `{"content":`})
	require.ErrorContains(t, err, "invalid metadata")
}

func TestAssetRouteConstraintChecksMetadataReferencesAcrossContentTypes(t *testing.T) {
	setAssetLibraryEnabledForTest(t, false)
	gin.SetMode(gin.TestMode)
	validMetadata := `{"content":[{"type":"image_url","image_url":{"url":"asset://ast_0123456789abcdefghijklmnopqrstuv"}}]}`
	invalidMetadata := `{"content":[{"type":"image_url","image_url":{"url":"asset://asset-upstream-id"}}]}`

	for _, test := range []struct {
		name        string
		contentType string
		body        func(string) *bytes.Buffer
	}{
		{
			name:        "json",
			contentType: "application/json",
			body: func(metadata string) *bytes.Buffer {
				return bytes.NewBufferString(`{"metadata":` + metadata + `}`)
			},
		},
		{
			name:        "urlencoded",
			contentType: "application/x-www-form-urlencoded",
			body: func(metadata string) *bytes.Buffer {
				return bytes.NewBufferString(url.Values{"metadata": {metadata}}.Encode())
			},
		},
	} {
		t.Run(test.name, func(t *testing.T) {
			for _, metadata := range []struct {
				name       string
				value      string
				wantStatus int
				wantCode   string
			}{
				{name: "platform reference", value: validMetadata, wantStatus: http.StatusServiceUnavailable, wantCode: "asset_library_disabled"},
				{name: "raw upstream reference", value: invalidMetadata, wantStatus: http.StatusBadRequest, wantCode: "invalid_asset_reference"},
			} {
				t.Run(metadata.name, func(t *testing.T) {
					recorder := httptest.NewRecorder()
					c, _ := gin.CreateTestContext(recorder)
					c.Request = httptest.NewRequest(http.MethodPost, "/v1/video/generations", test.body(metadata.value))
					c.Request.Header.Set("Content-Type", test.contentType)

					AssetRouteConstraint()(c)

					assert.Equal(t, metadata.wantStatus, recorder.Code)
					assert.Contains(t, recorder.Body.String(), metadata.wantCode)
				})
			}
		})
	}
}

func TestAssetRouteConstraintChecksMultipartMetadataReferences(t *testing.T) {
	setAssetLibraryEnabledForTest(t, false)
	gin.SetMode(gin.TestMode)

	for _, test := range []struct {
		name       string
		metadata   string
		wantStatus int
		wantCode   string
	}{
		{
			name:       "platform reference",
			metadata:   `{"content":[{"type":"video_url","video_url":{"url":"asset://ast_0123456789abcdefghijklmnopqrstuv"}}]}`,
			wantStatus: http.StatusServiceUnavailable,
			wantCode:   "asset_library_disabled",
		},
		{
			name:       "raw upstream reference",
			metadata:   `{"content":[{"type":"audio_url","audio_url":{"url":"asset://asset-upstream-id"}}]}`,
			wantStatus: http.StatusBadRequest,
			wantCode:   "invalid_asset_reference",
		},
	} {
		t.Run(test.name, func(t *testing.T) {
			var body bytes.Buffer
			writer := multipart.NewWriter(&body)
			require.NoError(t, writer.WriteField("metadata", test.metadata))
			require.NoError(t, writer.Close())

			recorder := httptest.NewRecorder()
			c, _ := gin.CreateTestContext(recorder)
			c.Request = httptest.NewRequest(http.MethodPost, "/v1/video/generations", &body)
			c.Request.Header.Set("Content-Type", writer.FormDataContentType())

			AssetRouteConstraint()(c)

			assert.Equal(t, test.wantStatus, recorder.Code)
			assert.Contains(t, recorder.Body.String(), test.wantCode)
		})
	}
}

func TestAssetRouteConstraintChecksEveryMultipartImage(t *testing.T) {
	setAssetLibraryEnabledForTest(t, false)
	gin.SetMode(gin.TestMode)

	for _, test := range []struct {
		name       string
		second     string
		wantStatus int
		wantCode   string
	}{
		{name: "platform reference", second: "asset://ast_0123456789abcdefghijklmnopqrstuv", wantStatus: http.StatusServiceUnavailable, wantCode: "asset_library_disabled"},
		{name: "raw upstream reference", second: "asset://asset-upstream-id", wantStatus: http.StatusBadRequest, wantCode: "invalid_asset_reference"},
	} {
		t.Run(test.name, func(t *testing.T) {
			var body bytes.Buffer
			writer := multipart.NewWriter(&body)
			require.NoError(t, writer.WriteField("images", "https://example.com/reference.png"))
			require.NoError(t, writer.WriteField("images", test.second))
			require.NoError(t, writer.Close())

			recorder := httptest.NewRecorder()
			c, _ := gin.CreateTestContext(recorder)
			c.Request = httptest.NewRequest(http.MethodPost, "/v1/video/generations", &body)
			c.Request.Header.Set("Content-Type", writer.FormDataContentType())

			AssetRouteConstraint()(c)

			assert.Equal(t, test.wantStatus, recorder.Code)
			assert.Contains(t, recorder.Body.String(), test.wantCode)
		})
	}
}
