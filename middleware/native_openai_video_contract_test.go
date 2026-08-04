package middleware

import (
	"bytes"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/model"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestNativeOpenAIVideoContractAcceptsNativeModelsAndDefaultsSora(t *testing.T) {
	gin.SetMode(gin.TestMode)
	tests := []struct {
		name        string
		requestBody func(t *testing.T) (*bytes.Buffer, string)
	}{
		{
			name: "JSON native model",
			requestBody: func(t *testing.T) (*bytes.Buffer, string) {
				return bytes.NewBufferString(`{"model":"sora-2-pro","prompt":"test"}`), "application/json"
			},
		},
		{
			name: "multipart defaults to sora-2",
			requestBody: func(t *testing.T) (*bytes.Buffer, string) {
				var body bytes.Buffer
				writer := multipart.NewWriter(&body)
				require.NoError(t, writer.WriteField("prompt", "test"))
				require.NoError(t, writer.Close())
				return &body, writer.FormDataContentType()
			},
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			body, contentType := test.requestBody(t)
			recorder := httptest.NewRecorder()
			context, _ := gin.CreateTestContext(recorder)
			context.Request = httptest.NewRequest(http.MethodPost, "/v1/videos", body)
			context.Request.Header.Set("Content-Type", contentType)

			NativeOpenAIVideoContractConstraint()(context)

			assert.False(t, context.IsAborted())
			modelName := common.GetContextKeyString(context, constant.ContextKeyOriginalModel)
			assert.Contains(t, []string{"sora-2", "sora-2-pro"}, modelName)
		})
	}
}

func TestNativeOpenAIVideoContractRejectsRegisteredLinkSKU(t *testing.T) {
	gin.SetMode(gin.TestMode)
	for _, path := range []string{"/v1/videos", "/v1/video/generations"} {
		recorder := httptest.NewRecorder()
		context, _ := gin.CreateTestContext(recorder)
		context.Request = httptest.NewRequest(
			http.MethodPost,
			path,
			bytes.NewBufferString(`{"model":"`+model.VideoSKUSeedance20Oversea+`","prompt":"test"}`),
		)
		context.Request.Header.Set("Content-Type", "application/json")

		NativeOpenAIVideoContractConstraint()(context)

		assert.True(t, context.IsAborted())
		assert.Equal(t, http.StatusBadRequest, recorder.Code)
		assert.Contains(t, recorder.Body.String(), "link_sku_contract_mismatch")
	}
}
