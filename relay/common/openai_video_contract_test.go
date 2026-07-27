package common

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestOpenAIVideoCreateContractDefaultsAndPreservesReferenceObject(t *testing.T) {
	gin.SetMode(gin.TestMode)
	request := httptest.NewRequest(http.MethodPost, "/v1/videos", strings.NewReader(
		`{"prompt":"animate this","input_reference":{"image_url":"asset://ast_12345678901234567890123456789012"}}`,
	))
	request.Header.Set("Content-Type", "application/json")
	context, _ := gin.CreateTestContext(httptest.NewRecorder())
	context.Request = request
	info := &RelayInfo{TaskRelayInfo: &TaskRelayInfo{}}

	require.Nil(t, ValidateMultipartDirect(context, info))
	stored, err := GetTaskRequest(context)
	require.NoError(t, err)
	assert.Equal(t, "sora-2", stored.Model)
	assert.Equal(t, "4", stored.Seconds)
	assert.Equal(t, "720x1280", stored.Size)
	assert.True(t, stored.InputReferenceObject)
	assert.Equal(t, "asset://ast_12345678901234567890123456789012", stored.InputReferenceImageURL)
}

func TestOpenAIVideoCreateContractRejectsLegacyJSONReferenceAndInvalidShape(t *testing.T) {
	gin.SetMode(gin.TestMode)
	tests := []struct {
		name string
		body string
		code string
	}{
		{
			name: "legacy string",
			body: `{"model":"sora-2","prompt":"animate","input_reference":"https://example.com/image.png"}`,
			code: "invalid_input_reference",
		},
		{
			name: "both object fields",
			body: `{"model":"sora-2","prompt":"animate","input_reference":{"file_id":"file-1","image_url":"https://example.com/image.png"}}`,
			code: "invalid_json",
		},
		{
			name: "unsupported seconds",
			body: `{"model":"sora-2","prompt":"animate","seconds":"6"}`,
			code: "invalid_seconds",
		},
		{
			name: "unsupported size",
			body: `{"model":"sora-2","prompt":"animate","size":"1920x1080"}`,
			code: "invalid_size",
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			request := httptest.NewRequest(http.MethodPost, "/v1/videos", strings.NewReader(test.body))
			request.Header.Set("Content-Type", "application/json")
			context, _ := gin.CreateTestContext(httptest.NewRecorder())
			context.Request = request
			taskErr := ValidateMultipartDirect(context, &RelayInfo{TaskRelayInfo: &TaskRelayInfo{}})
			require.NotNil(t, taskErr)
			assert.Equal(t, test.code, taskErr.Code)
			assert.Equal(t, http.StatusBadRequest, taskErr.StatusCode)
		})
	}
}
