package middleware

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/QuantumNous/new-api/dto"
	relaycommon "github.com/QuantumNous/new-api/relay/common"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestKlingRequestConvertStoresTypedContractWithoutMetadata(t *testing.T) {
	gin.SetMode(gin.TestMode)
	engine := gin.New()
	engine.POST("/kling/v1/videos/image2video", KlingRequestConvert(), func(c *gin.Context) {
		var internal relaycommon.TaskSubmitReq
		require.NoError(t, c.ShouldBindJSON(&internal))
		assert.Empty(t, internal.Metadata)
		assert.Equal(t, "kling-v2-master", internal.Model)
		contract, ok := relaycommon.GetVideoContractRequest(c)
		require.True(t, ok)
		assert.Equal(t, dto.VideoContractKlingV1, contract.ContractID)
		require.NotNil(t, contract.Kling)
		require.NotNil(t, contract.Kling.Image)
		require.NotNil(t, contract.Kling.ImageTail)
		assert.Equal(t, "https://example.com/first.png", *contract.Kling.Image)
		assert.Equal(t, "https://example.com/last.png", *contract.Kling.ImageTail)
		c.Status(http.StatusNoContent)
	})
	request := httptest.NewRequest(http.MethodPost, "/kling/v1/videos/image2video", strings.NewReader(
		`{"model_name":"kling-v2-master","prompt":"move","image":"https://example.com/first.png","image_tail":"https://example.com/last.png","duration":"5"}`,
	))
	request.Header.Set("Content-Type", "application/json")
	response := httptest.NewRecorder()

	engine.ServeHTTP(response, request)

	assert.Equal(t, http.StatusNoContent, response.Code)
}

func TestJimengRequestConvertStoresTypedContractWithoutMetadata(t *testing.T) {
	gin.SetMode(gin.TestMode)
	engine := gin.New()
	engine.POST("/jimeng/", JimengRequestConvert(), func(c *gin.Context) {
		var internal relaycommon.TaskSubmitReq
		require.NoError(t, c.ShouldBindJSON(&internal))
		assert.Empty(t, internal.Metadata)
		assert.Equal(t, "jimeng_v30", internal.Model)
		contract, ok := relaycommon.GetVideoContractRequest(c)
		require.True(t, ok)
		assert.Equal(t, dto.VideoContractJimeng, contract.ContractID)
		require.NotNil(t, contract.Jimeng)
		assert.Equal(t, []string{"https://example.com/input.png"}, contract.Jimeng.ImageURLs)
		c.Status(http.StatusNoContent)
	})
	request := httptest.NewRequest(
		http.MethodPost,
		"/jimeng/?Action=CVSync2AsyncSubmitTask&Version=2022-08-31",
		strings.NewReader(`{"req_key":"jimeng_v30","prompt":"move","image_urls":["https://example.com/input.png"],"frames":121}`),
	)
	request.Header.Set("Content-Type", "application/json")
	response := httptest.NewRecorder()

	engine.ServeHTTP(response, request)

	assert.Equal(t, http.StatusNoContent, response.Code)
}

func TestOfficialVideoConvertersRejectProviderEscapeHatches(t *testing.T) {
	tests := []struct {
		path       string
		middleware gin.HandlerFunc
		body       string
	}{
		{
			path: "/kling/v1/videos/text2video", middleware: KlingRequestConvert(),
			body: `{"model_name":"kling-v1","prompt":"move","provider_options":{"private":true}}`,
		},
		{
			path: "/jimeng/?Action=CVSync2AsyncSubmitTask&Version=2022-08-31", middleware: JimengRequestConvert(),
			body: `{"req_key":"jimeng_v30","prompt":"move","extra":{"private":true}}`,
		},
		{
			path: "/jimeng/?Action=CVSync2AsyncGetResult&Version=2022-08-31", middleware: JimengRequestConvert(),
			body: `{"task_id":"task-1","extra":{"private":true}}`,
		},
		{
			path: "/kling/v1/videos/image2video", middleware: KlingRequestConvert(),
			body: `{"model_name":"kling-v1","prompt":"move","dynamic_masks":[{"mask":"https://example.com/mask.png","provider_options":{"private":true}}]}`,
		},
	}
	for _, test := range tests {
		engine := gin.New()
		engine.POST(strings.Split(test.path, "?")[0], test.middleware, func(c *gin.Context) {
			t.Fatal("request should have been aborted")
		})
		request := httptest.NewRequest(http.MethodPost, test.path, strings.NewReader(test.body))
		request.Header.Set("Content-Type", "application/json")
		response := httptest.NewRecorder()

		engine.ServeHTTP(response, request)

		assert.Equal(t, http.StatusBadRequest, response.Code)
	}
}

func TestKlingTextToVideoRejectsImageTail(t *testing.T) {
	engine := gin.New()
	engine.POST("/kling/v1/videos/text2video", KlingRequestConvert(), func(c *gin.Context) {
		t.Fatal("request should have been aborted")
	})
	request := httptest.NewRequest(
		http.MethodPost,
		"/kling/v1/videos/text2video",
		strings.NewReader(`{"model_name":"kling-v1","prompt":"move","image_tail":"https://example.com/last.png"}`),
	)
	request.Header.Set("Content-Type", "application/json")
	response := httptest.NewRecorder()

	engine.ServeHTTP(response, request)

	assert.Equal(t, http.StatusBadRequest, response.Code)
}
