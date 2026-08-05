package middleware

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/model"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestPublishedVideoContractsResolveOneVersionedSKUCapability(t *testing.T) {
	gin.SetMode(gin.TestMode)
	tests := []struct {
		name     string
		path     string
		protocol string
		body     string
		convert  gin.HandlerFunc
		model    string
	}{
		{
			name: "ModelArk",
			path: "/api/v3/contents/generations/tasks", protocol: model.TaskClientProtocolModelArkV3,
			body:    `{"model":"seedance-2.0-standard","content":[{"type":"text","text":"move"}]}`,
			convert: ModelArkVideoCreateConvert(), model: model.VideoSKUSeedance20Standard,
		},
		{
			name: "Kling",
			path: "/kling/v1/videos/text2video", protocol: model.TaskClientProtocolKlingV1,
			body:    `{"model_name":"kling-v1","prompt":"move","duration":"5"}`,
			convert: KlingRequestConvert(), model: model.VideoSKUKlingV1,
		},
		{
			name: "Jimeng",
			path: "/jimeng/?Action=CVSync2AsyncSubmitTask&Version=2022-08-31", protocol: model.TaskClientProtocolJimeng,
			body:    `{"req_key":"jimeng_vgfm_t2v_l20","prompt":"move"}`,
			convert: JimengRequestConvert(), model: model.VideoSKUJimengVGFMT2VL20,
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			engine := gin.New()
			engine.POST(strings.Split(test.path, "?")[0], func(c *gin.Context) {
				common.SetContextKey(c, constant.ContextKeyTaskClientProtocol, test.protocol)
				c.Next()
			}, test.convert, ResolveVideoSKUCapability(), func(c *gin.Context) {
				capability, ok := resolvedVideoSKUCapability(c)
				require.True(t, ok)
				assert.Equal(t, test.model, capability.PublicModel)
				assert.NotEmpty(t, capability.Version)
				assert.Len(t, capability.ContentHash, 64)
				c.Status(http.StatusNoContent)
			})
			request := httptest.NewRequest(http.MethodPost, test.path, strings.NewReader(test.body))
			request.Header.Set("Content-Type", "application/json")
			response := httptest.NewRecorder()

			engine.ServeHTTP(response, request)

			assert.Equal(t, http.StatusNoContent, response.Code)
		})
	}
}

func TestUnregisteredVideoSKUFailsClosedBeforeDistribution(t *testing.T) {
	gin.SetMode(gin.TestMode)
	engine := gin.New()
	engine.POST("/kling/v1/videos/text2video", func(c *gin.Context) {
		common.SetContextKey(c, constant.ContextKeyTaskClientProtocol, model.TaskClientProtocolKlingV1)
		c.Next()
	}, KlingRequestConvert(), ResolveVideoSKUCapability(), func(c *gin.Context) {
		t.Fatal("unregistered video SKU must not reach distribution")
	})
	request := httptest.NewRequest(
		http.MethodPost,
		"/kling/v1/videos/text2video",
		strings.NewReader(`{"model_name":"unregistered-kling","prompt":"move"}`),
	)
	request.Header.Set("Content-Type", "application/json")
	response := httptest.NewRecorder()

	engine.ServeHTTP(response, request)

	assert.Equal(t, http.StatusBadRequest, response.Code)
}

func TestPublishedCustomerVideoModelValidatesAgainstFrozenLinkSKU(t *testing.T) {
	gin.SetMode(gin.TestMode)
	engine := gin.New()
	engine.POST("/api/v3/contents/generations/tasks",
		ModelArkVideoCreateConvert(),
		func(c *gin.Context) {
			publication := model.LinkModelPublication{
				ContractNamespace:  model.LinkContractNamespaceDefault,
				RouteFamily:        model.LinkRouteFamilyModelArkVideo,
				CustomerModel:      "customer-seedance",
				LinkSKU:            model.VideoSKUSeedance20Standard,
				PublicationVersion: 3,
			}
			common.SetContextKey(c, constant.ContextKeyLinkRouteFamily, string(publication.RouteFamily))
			common.SetContextKey(c, constant.ContextKeyLinkModelPublication, publication)
			c.Next()
		},
		ResolveVideoSKUCapability(),
		func(c *gin.Context) {
			capability, ok := resolvedVideoSKUCapability(c)
			require.True(t, ok)
			assert.Equal(t, model.VideoSKUSeedance20Standard, capability.PublicModel)
			c.Status(http.StatusNoContent)
		},
	)
	request := httptest.NewRequest(http.MethodPost, "/api/v3/contents/generations/tasks", strings.NewReader(`{"model":"customer-seedance","content":[{"type":"text","text":"move"}]}`))
	request.Header.Set("Content-Type", "application/json")
	response := httptest.NewRecorder()

	engine.ServeHTTP(response, request)

	assert.Equal(t, http.StatusNoContent, response.Code)
}

func TestVideoSKUCreateGatesDoNotInterceptJimengReadAction(t *testing.T) {
	gin.SetMode(gin.TestMode)
	engine := gin.New()
	engine.POST(
		"/jimeng/",
		JimengRequestConvert(),
		ResolveVideoSKUCapability(),
		VideoSKUChannelConstraint(),
		func(c *gin.Context) {
			assert.Equal(t, http.MethodGet, c.Request.Method)
			assert.Equal(t, "task-1", c.GetString("task_id"))
			c.Status(http.StatusNoContent)
		},
	)
	response := httptest.NewRecorder()
	request := httptest.NewRequest(
		http.MethodPost,
		"/jimeng/?Action=CVSync2AsyncGetResult&Version=2022-08-31",
		strings.NewReader(`{"task_id":"task-1"}`),
	)
	request.Header.Set("Content-Type", "application/json")

	engine.ServeHTTP(response, request)

	assert.Equal(t, http.StatusNoContent, response.Code)
}
