package middleware

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/model"
	relaycommon "github.com/QuantumNous/new-api/relay/common"
	"github.com/gin-gonic/gin"
	"github.com/glebarez/sqlite"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

func TestModelArkVideoCreateConvertPreservesOfficialContentInMetadata(t *testing.T) {
	gin.SetMode(gin.TestMode)
	engine := gin.New()
	var converted relaycommon.TaskSubmitReq
	var convertedPath string
	engine.POST("/api/v3/contents/generations/tasks", ModelArkVideoCreateConvert(), func(c *gin.Context) {
		convertedPath = c.Request.URL.Path
		require.NoError(t, c.ShouldBindJSON(&converted))
		c.Status(http.StatusNoContent)
	})
	request := httptest.NewRequest(http.MethodPost, "/api/v3/contents/generations/tasks", strings.NewReader(`{
		"model":"seedance-model",
		"content":[
			{"type":"text","text":"make a video"},
			{"type":"image_url","role":"first_frame","image_url":{"url":"https://example.com/input.png"}}
		],
		"duration":8,
		"service_tier":"flex",
		"generate_audio":false
	}`))
	request.Header.Set("Content-Type", "application/json")
	recorder := httptest.NewRecorder()

	engine.ServeHTTP(recorder, request)

	assert.Equal(t, http.StatusNoContent, recorder.Code)
	assert.Equal(t, "/v1/video/generations", convertedPath)
	assert.Equal(t, "/api/v3/contents/generations/tasks", request.URL.Path)
	assert.Equal(t, "seedance-model", converted.Model)
	assert.Equal(t, "make a video", converted.Prompt)
	assert.Equal(t, float64(8), converted.Metadata["duration"])
	assert.Equal(t, "flex", converted.Metadata["service_tier"])
	assert.Equal(t, false, converted.Metadata["generate_audio"])
	content, ok := converted.Metadata["content"].([]any)
	require.True(t, ok)
	require.Len(t, content, 2)
}

func TestModelArkVideoCreateConvertAcceptsMediaOnlyContent(t *testing.T) {
	gin.SetMode(gin.TestMode)
	engine := gin.New()
	var converted relaycommon.TaskSubmitReq
	engine.POST("/api/v3/contents/generations/tasks", ModelArkVideoCreateConvert(), func(c *gin.Context) {
		require.NoError(t, c.ShouldBindJSON(&converted))
		assert.True(t, common.GetContextKeyBool(c, constant.ContextKeyTaskPromptValidated))
		assert.True(t, common.GetContextKeyBool(c, constant.ContextKeyTaskDurationValidated))
		c.Status(http.StatusNoContent)
	})
	request := httptest.NewRequest(http.MethodPost, "/api/v3/contents/generations/tasks", strings.NewReader(`{
		"model":"seedance-model",
		"content":[{"type":"video_url","role":"reference_video","video_url":{"url":"asset://asset-123"}}],
		"duration":-1,
		"resolution":"4k",
		"ratio":"adaptive"
	}`))
	request.Header.Set("Content-Type", "application/json")
	recorder := httptest.NewRecorder()

	engine.ServeHTTP(recorder, request)

	assert.Equal(t, http.StatusNoContent, recorder.Code)
	assert.Empty(t, converted.Prompt)
	assert.Equal(t, float64(-1), converted.Metadata["duration"])
}

func TestModelArkVideoCreateConvertRejectsInvalidKnownFields(t *testing.T) {
	tests := []struct {
		name string
		body string
	}{
		{name: "duration", body: `{"model":"m","content":[{"type":"text","text":"video"}],"duration":16}`},
		{name: "resolution", body: `{"model":"m","content":[{"type":"text","text":"video"}],"resolution":"2k"}`},
		{name: "role", body: `{"model":"m","content":[{"type":"image_url","role":"reference_video","image_url":{"url":"https://example.com/a.png"}}]}`},
		{name: "URL", body: `{"model":"m","content":[{"type":"video_url","role":"reference_video","video_url":{"url":"file:///tmp/a.mp4"}}]}`},
		{name: "audio only", body: `{"model":"m","content":[{"type":"audio_url","role":"reference_audio","audio_url":{"url":"https://example.com/a.mp3"}}]}`},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			engine := gin.New()
			engine.POST("/api/v3/contents/generations/tasks", ModelArkVideoCreateConvert(), func(c *gin.Context) {
				t.Fatal("request should have been aborted")
			})
			request := httptest.NewRequest(http.MethodPost, "/api/v3/contents/generations/tasks", strings.NewReader(test.body))
			request.Header.Set("Content-Type", "application/json")
			recorder := httptest.NewRecorder()

			engine.ServeHTTP(recorder, request)

			assert.Equal(t, http.StatusBadRequest, recorder.Code)
		})
	}
}

func TestModelArkVideoCreateConvertRejectsWebhookParameter(t *testing.T) {
	gin.SetMode(gin.TestMode)
	engine := gin.New()
	engine.POST("/api/v3/contents/generations/tasks", ModelArkVideoCreateConvert(), func(c *gin.Context) {
		t.Fatal("request should have been aborted")
	})
	request := httptest.NewRequest(http.MethodPost, "/api/v3/contents/generations/tasks", strings.NewReader(
		`{"model":"seedance-model","content":[{"type":"text","text":"video"}],"callback_url":"https://example.com/hook"}`,
	))
	request.Header.Set("Content-Type", "application/json")
	recorder := httptest.NewRecorder()

	engine.ServeHTTP(recorder, request)

	assert.Equal(t, http.StatusBadRequest, recorder.Code)
	var body map[string]any
	require.NoError(t, common.Unmarshal(recorder.Body.Bytes(), &body))
	errorBody, ok := body["error"].(map[string]any)
	require.True(t, ok)
	assert.Equal(t, "unsupported_parameter", errorBody["code"])
}

func TestModelArkVideoChannelConstraintAllowsAdaptedProtocols(t *testing.T) {
	previousDB := model.DB
	previousType := common.MainDatabaseType()
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	require.NoError(t, err)
	require.NoError(t, db.AutoMigrate(&model.Channel{}))
	model.DB = db
	common.SetMainDatabaseType(common.DatabaseTypeSQLite)
	t.Cleanup(func() {
		model.DB = previousDB
		common.SetMainDatabaseType(previousType)
	})

	channels := []model.Channel{
		{Name: "legacy", Type: constant.ChannelTypeDoubaoVideo, Status: common.ChannelStatusEnabled, Key: "legacy", OtherSettings: `{}`},
		{Name: "official", Type: constant.ChannelTypeDoubaoVideo, Status: common.ChannelStatusEnabled, Key: "official", OtherSettings: `{"video_upstream_profile":"official"}`},
		{Name: "reverse-proxy", Type: constant.ChannelTypeDoubaoVideo, Status: common.ChannelStatusEnabled, Key: "reverse-proxy", OtherSettings: `{"video_upstream_profile":"third_party_reverse_proxy"}`},
		{Name: "Moxing-relay", Type: constant.ChannelTypeDoubaoVideo, Status: common.ChannelStatusEnabled, Key: "relay", OtherSettings: `{"video_upstream_profile":"third_party_relay"}`},
		{Name: "unknown", Type: constant.ChannelTypeDoubaoVideo, Status: common.ChannelStatusEnabled, Key: "unknown", OtherSettings: `{"video_upstream_profile":"unknown"}`},
		{Name: "disabled-relay", Type: constant.ChannelTypeDoubaoVideo, Status: common.ChannelStatusManuallyDisabled, Key: "disabled", OtherSettings: `{"video_upstream_profile":"third_party_relay"}`},
		{Name: "other-type", Type: constant.ChannelTypeOpenAI, Status: common.ChannelStatusEnabled, Key: "other", OtherSettings: `{}`},
	}
	require.NoError(t, db.Create(&channels).Error)

	var allowed map[int]struct{}
	engine := gin.New()
	engine.GET("/", ModelArkVideoChannelConstraint(), func(c *gin.Context) {
		value, ok := common.GetContextKey(c, constant.ContextKeyAssetAllowedChannelIDs)
		require.True(t, ok)
		allowed, ok = value.(map[int]struct{})
		require.True(t, ok)
		c.Status(http.StatusNoContent)
	})
	response := httptest.NewRecorder()

	engine.ServeHTTP(response, httptest.NewRequest(http.MethodGet, "/", nil))

	assert.Equal(t, http.StatusNoContent, response.Code)
	assert.Contains(t, allowed, channels[0].Id)
	assert.Contains(t, allowed, channels[1].Id)
	assert.Contains(t, allowed, channels[2].Id)
	assert.Contains(t, allowed, channels[3].Id)
	assert.NotContains(t, allowed, channels[4].Id)
	assert.NotContains(t, allowed, channels[5].Id)
	assert.NotContains(t, allowed, channels[6].Id)
}
