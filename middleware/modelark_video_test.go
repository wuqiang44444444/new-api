package middleware

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/dto"
	"github.com/QuantumNous/new-api/model"
	relaycommon "github.com/QuantumNous/new-api/relay/common"
	"github.com/QuantumNous/new-api/service"
	"github.com/gin-gonic/gin"
	"github.com/glebarez/sqlite"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

func TestModelArkVideoCreateConvertHashesAndRemovesEndUserSubject(t *testing.T) {
	gin.SetMode(gin.TestMode)
	engine := gin.New()
	engine.POST("/api/v3/contents/generations/tasks", func(c *gin.Context) {
		c.Set("token_id", 321)
		c.Next()
	}, ModelArkVideoCreateConvert(), func(c *gin.Context) {
		contract, ok := relaycommon.GetVideoContractRequest(c)
		require.True(t, ok)
		require.NotNil(t, contract.ModelArk)
		assert.Nil(t, contract.ModelArk.EndUserSubject)
		expected, err := service.EndUserSubjectHash(321, "customer-42")
		require.NoError(t, err)
		assert.Equal(t, expected, common.GetContextKeyString(c, constant.ContextKeyEndUserSubjectHash))
		c.Status(http.StatusNoContent)
	})
	request := httptest.NewRequest(http.MethodPost, "/api/v3/contents/generations/tasks", strings.NewReader(`{
		"model":"seedance-model",
		"end_user_subject":"customer-42",
		"content":[{"type":"text","text":"make a video"}]
	}`))
	request.Header.Set("Content-Type", "application/json")
	response := httptest.NewRecorder()

	engine.ServeHTTP(response, request)

	assert.Equal(t, http.StatusNoContent, response.Code)
}

func TestModelArkVideoCreateConvertRejectsSubjectWithClientSafetyIdentifier(t *testing.T) {
	engine := gin.New()
	engine.POST("/api/v3/contents/generations/tasks", func(c *gin.Context) {
		c.Set("token_id", 321)
		c.Next()
	}, ModelArkVideoCreateConvert(), func(c *gin.Context) {
		t.Fatal("request should have been aborted")
	})
	request := httptest.NewRequest(http.MethodPost, "/api/v3/contents/generations/tasks", strings.NewReader(`{
		"model":"seedance-model",
		"end_user_subject":"customer-42",
		"safety_identifier":"client-controlled",
		"content":[{"type":"text","text":"make a video"}]
	}`))
	request.Header.Set("Content-Type", "application/json")
	response := httptest.NewRecorder()

	engine.ServeHTTP(response, request)

	assert.Equal(t, http.StatusBadRequest, response.Code)
}

func TestModelArkVideoCreateConvertUsesTypedContractWithoutMetadata(t *testing.T) {
	gin.SetMode(gin.TestMode)
	engine := gin.New()
	var converted relaycommon.TaskSubmitReq
	var convertedPath string
	engine.POST("/api/v3/contents/generations/tasks", ModelArkVideoCreateConvert(), func(c *gin.Context) {
		convertedPath = c.Request.URL.Path
		require.NoError(t, c.ShouldBindJSON(&converted))
		contract, ok := relaycommon.GetVideoContractRequest(c)
		require.True(t, ok)
		require.NotNil(t, contract.ModelArk)
		assert.Equal(t, dto.VideoContractModelArkV3, contract.ContractID)
		assert.Equal(t, 8, *contract.ModelArk.Duration)
		assert.Equal(t, "flex", *contract.ModelArk.ServiceTier)
		assert.Equal(t, false, *contract.ModelArk.GenerateAudio)
		assert.Len(t, contract.ModelArk.Content, 2)
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
	assert.Empty(t, converted.Metadata)
}

func TestModelArkVideoCreateConvertAcceptsMediaOnlyContent(t *testing.T) {
	gin.SetMode(gin.TestMode)
	engine := gin.New()
	var converted relaycommon.TaskSubmitReq
	engine.POST("/api/v3/contents/generations/tasks", ModelArkVideoCreateConvert(), func(c *gin.Context) {
		require.NoError(t, c.ShouldBindJSON(&converted))
		contract, ok := relaycommon.GetVideoContractRequest(c)
		require.True(t, ok)
		require.NotNil(t, contract.ModelArk)
		assert.Equal(t, -1, *contract.ModelArk.Duration)
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
	assert.Equal(t, -1, converted.Duration)
	assert.Empty(t, converted.Metadata)
}

func TestModelArkVideoPublishedContractRejectsInvalidKnownFields(t *testing.T) {
	tests := []struct {
		name string
		body string
	}{
		{name: "duration", body: `{"model":"seedance-byteplus","content":[{"type":"text","text":"video"}],"duration":16}`},
		{name: "resolution", body: `{"model":"seedance-byteplus","content":[{"type":"text","text":"video"}],"resolution":"2k"}`},
		{name: "role", body: `{"model":"seedance-byteplus","content":[{"type":"image_url","role":"reference_video","image_url":{"url":"https://example.com/a.png"}}]}`},
		{name: "URL", body: `{"model":"seedance-byteplus","content":[{"type":"video_url","role":"reference_video","video_url":{"url":"file:///tmp/a.mp4"}}]}`},
		{name: "execution expiry", body: `{"model":"seedance-byteplus","content":[{"type":"text","text":"video"}],"execution_expires_after":3599}`},
		{name: "seed", body: `{"model":"seedance-byteplus","content":[{"type":"text","text":"video"}],"seed":-2}`},
		{name: "frames not published", body: `{"model":"seedance-byteplus","content":[{"type":"text","text":"video"}],"frames":121}`},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			engine := gin.New()
			engine.POST("/api/v3/contents/generations/tasks", ModelArkVideoCreateConvert(), ResolveVideoSKUCapability(), func(c *gin.Context) {
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

func TestModelArkVideoPublishedContractRejectsWebhookParameter(t *testing.T) {
	gin.SetMode(gin.TestMode)
	engine := gin.New()
	engine.POST("/api/v3/contents/generations/tasks", ModelArkVideoCreateConvert(), ResolveVideoSKUCapability(), func(c *gin.Context) {
		t.Fatal("request should have been aborted")
	})
	request := httptest.NewRequest(http.MethodPost, "/api/v3/contents/generations/tasks", strings.NewReader(
		`{"model":"seedance-byteplus","content":[{"type":"text","text":"video"}],"callback_url":"https://example.com/hook"}`,
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
	engine.GET("/", func(c *gin.Context) {
		relaycommon.SetVideoContractRequest(c, dto.VideoContractRequest{
			ContractID: dto.VideoContractModelArkV3,
			ModelArk: &dto.ModelArkVideoCreateRequest{
				Model:   "seedance-model",
				Content: []dto.ModelArkVideoContent{{Type: "text", Text: common.GetPointer("video")}},
			},
		})
		c.Next()
	}, ModelArkVideoChannelConstraint(), func(c *gin.Context) {
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

func TestModelArkVideoChannelConstraintFiltersNonEquivalentProfile(t *testing.T) {
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
		{Name: "official", Type: constant.ChannelTypeDoubaoVideo, Status: common.ChannelStatusEnabled, Key: "official", Models: model.VideoSKUSeedanceBytePlus, OtherSettings: `{"video_upstream_profile":"official","asset_upstream_profile":"official_action_assets","link_implementation":{"id":"byteplus.seedance-ark","version":"v1"}}`},
		{Name: "relay", Type: constant.ChannelTypeDoubaoVideo, Status: common.ChannelStatusEnabled, Key: "relay", Models: model.VideoSKUDoubaoSeedance20260128, OtherSettings: `{"video_upstream_profile":"third_party_relay","video_upstream_create_path":"/v1/media/generations","video_upstream_query_path_template":"/v1/media/tasks/{task_id}","asset_upstream_profile":"relay_assets","link_implementation":{"id":"tokensave.seedance-media-task","version":"v1"}}`},
	}
	require.NoError(t, db.Create(&channels).Error)

	var allowed map[int]struct{}
	engine := gin.New()
	engine.GET("/", func(c *gin.Context) {
		relaycommon.SetVideoContractRequest(c, dto.VideoContractRequest{
			ContractID: dto.VideoContractModelArkV3,
			ModelArk: &dto.ModelArkVideoCreateRequest{
				Model: model.VideoSKUSeedanceBytePlus,
				Content: []dto.ModelArkVideoContent{{
					Type: "video_url", Role: common.GetPointer("reference_video"),
					VideoURL: &dto.VideoMediaURL{URL: "https://example.com/reference.mp4"},
				}},
			},
		})
		capability, ok := model.ResolveVideoSKUCapability(model.VideoSKUSeedanceBytePlus)
		require.True(t, ok)
		common.SetContextKey(c, constant.ContextKeyResolvedVideoSKUCapability, capability)
		c.Next()
	}, ModelArkVideoChannelConstraint(), func(c *gin.Context) {
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
	assert.NotContains(t, allowed, channels[1].Id)
}

func TestModelArkVideoCreateConvertRejectsUnknownTopLevelField(t *testing.T) {
	engine := gin.New()
	engine.POST("/api/v3/contents/generations/tasks", ModelArkVideoCreateConvert(), func(c *gin.Context) {
		t.Fatal("request should have been aborted")
	})
	request := httptest.NewRequest(http.MethodPost, "/api/v3/contents/generations/tasks", strings.NewReader(
		`{"model":"seedance-model","content":[{"type":"text","text":"video"}],"provider_options":{"mode":"private"}}`,
	))
	request.Header.Set("Content-Type", "application/json")
	response := httptest.NewRecorder()

	engine.ServeHTTP(response, request)

	assert.Equal(t, http.StatusBadRequest, response.Code)
}

func TestModelArkVideoCreateConvertRejectsUnknownNestedField(t *testing.T) {
	gin.SetMode(gin.TestMode)
	engine := gin.New()
	engine.POST("/api/v3/contents/generations/tasks", ModelArkVideoCreateConvert(), func(c *gin.Context) {
		t.Fatal("request should have been aborted")
	})
	request := httptest.NewRequest(http.MethodPost, "/api/v3/contents/generations/tasks", strings.NewReader(
		`{"model":"seedance-model","content":[{"type":"image_url","role":"first_frame","image_url":{"url":"https://example.com/a.png","provider_id":"private"}}]}`,
	))
	request.Header.Set("Content-Type", "application/json")
	recorder := httptest.NewRecorder()

	engine.ServeHTTP(recorder, request)

	assert.Equal(t, http.StatusBadRequest, recorder.Code)
}
