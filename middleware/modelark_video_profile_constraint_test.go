package middleware

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/dto"
	"github.com/QuantumNous/new-api/model"
	relaycommon "github.com/QuantumNous/new-api/relay/common"
	"github.com/gin-gonic/gin"
	"github.com/glebarez/sqlite"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

func TestModelArkVideoChannelConstraintRejectsRelayEndFrameReferenceCombination(t *testing.T) {
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
	require.NoError(t, db.Create(&model.Channel{
		Name: "relay", Type: constant.ChannelTypeDoubaoVideo, Status: common.ChannelStatusEnabled,
		Key: "relay", Models: model.VideoSKUSeedance20Oversea,
		OtherSettings: `{"video_upstream_profile":"third_party_relay","video_upstream_create_path":"/v1/media/generations","video_upstream_query_path_template":"/v1/media/tasks/{task_id}","asset_upstream_profile":"relay_assets","link_implementation":{"id":"moxing.seedance-media-task","version":"v1"}}`,
	}).Error)

	engine := gin.New()
	engine.POST("/", func(c *gin.Context) {
		relaycommon.SetVideoContractRequest(c, dto.VideoContractRequest{
			ContractID: dto.VideoContractModelArkV3,
			ModelArk: &dto.ModelArkVideoCreateRequest{
				Model: model.VideoSKUSeedance20Oversea,
				Content: []dto.ModelArkVideoContent{
					{
						Type: "image_url", Role: common.GetPointer("first_frame"),
						ImageURL: &dto.VideoMediaURL{URL: "https://example.com/first.png"},
					},
					{
						Type: "image_url", Role: common.GetPointer("last_frame"),
						ImageURL: &dto.VideoMediaURL{URL: "https://example.com/last.png"},
					},
					{
						Type: "image_url", Role: common.GetPointer("reference_image"),
						ImageURL: &dto.VideoMediaURL{URL: "https://example.com/reference.png"},
					},
				},
			},
		})
		c.Next()
	}, ResolveVideoSKUCapability(), ModelArkVideoChannelConstraint(), func(c *gin.Context) {
		t.Fatal("incompatible request must stop before task submission and billing")
	})
	response := httptest.NewRecorder()

	engine.ServeHTTP(response, httptest.NewRequest(http.MethodPost, "/", nil))

	assert.Equal(t, http.StatusBadRequest, response.Code)
	var body map[string]any
	require.NoError(t, common.Unmarshal(response.Body.Bytes(), &body))
	errorBody, ok := body["error"].(map[string]any)
	require.True(t, ok)
	assert.Equal(t, "unsupported_parameter", errorBody["code"])
}

func TestModelArkVideoChannelConstraintAllowsDoubaoSeedanceRelay(t *testing.T) {
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
		{
			Name: "relay", Type: constant.ChannelTypeDoubaoVideo, Status: common.ChannelStatusEnabled,
			Key: "relay", Models: model.VideoSKUDoubaoSeedance20260128,
			OtherSettings: `{"video_upstream_profile":"third_party_relay","video_upstream_create_path":"/v1/media/generations","video_upstream_query_path_template":"/v1/media/tasks/{task_id}","asset_upstream_profile":"relay_assets","link_implementation":{"id":"tokensave.seedance-media-task","version":"v1"}}`,
		},
		{
			Name: "reverse", Type: constant.ChannelTypeDoubaoVideo, Status: common.ChannelStatusEnabled,
			Key: "reverse", OtherSettings: `{"video_upstream_profile":"third_party_reverse_proxy"}`,
		},
	}
	require.NoError(t, db.Create(&channels).Error)

	var allowed map[int]struct{}
	engine := gin.New()
	engine.POST("/", func(c *gin.Context) {
		relaycommon.SetVideoContractRequest(c, dto.VideoContractRequest{
			ContractID: dto.VideoContractModelArkV3,
			ModelArk: &dto.ModelArkVideoCreateRequest{
				Model:      model.VideoSKUDoubaoSeedance20260128,
				Content:    []dto.ModelArkVideoContent{{Type: "text", Text: common.GetPointer("snow mountain")}},
				Duration:   common.GetPointer(5),
				Resolution: common.GetPointer("720p"),
				Ratio:      common.GetPointer("16:9"),
			},
		})
		c.Next()
	}, ResolveVideoSKUCapability(), ModelArkVideoChannelConstraint(), func(c *gin.Context) {
		value, ok := common.GetContextKey(c, constant.ContextKeyAssetAllowedChannelIDs)
		require.True(t, ok)
		allowed, ok = value.(map[int]struct{})
		require.True(t, ok)
		c.Status(http.StatusNoContent)
	})
	response := httptest.NewRecorder()

	engine.ServeHTTP(response, httptest.NewRequest(http.MethodPost, "/", nil))

	assert.Equal(t, http.StatusNoContent, response.Code)
	assert.Contains(t, allowed, channels[0].Id)
	assert.NotContains(t, allowed, channels[1].Id)
}

func TestModelArkVideoChannelConstraintAllowsMoxingArkManagedAssetProfile(t *testing.T) {
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

	channel := model.Channel{
		Name: "moxing-ark", Type: constant.ChannelTypeDoubaoVideo, Status: common.ChannelStatusEnabled,
		Models: model.VideoSKUSeedance20Oversea, Key: "moxing-key",
		OtherSettings: `{"video_upstream_profile":"third_party_reverse_proxy","asset_upstream_profile":"ark_assets","link_implementation":{"id":"moxing.seedance-ark-assets","version":"v1"}}`,
	}
	require.NoError(t, db.Create(&channel).Error)

	var allowed map[int]struct{}
	engine := gin.New()
	engine.POST("/", func(c *gin.Context) {
		relaycommon.SetVideoContractRequest(c, dto.VideoContractRequest{
			ContractID: dto.VideoContractModelArkV3,
			ModelArk: &dto.ModelArkVideoCreateRequest{
				Model: model.VideoSKUSeedance20Oversea,
				Content: []dto.ModelArkVideoContent{
					{Type: "text", Text: common.GetPointer("portrait speaks")},
					{Type: "image_url", Role: common.GetPointer("reference_image"), ImageURL: &dto.VideoMediaURL{URL: "asset://ast_12345678901234567890123456789012"}},
				},
			},
		})
		common.SetContextKey(c, constant.ContextKeyAssetAllowedChannelIDs, map[int]struct{}{channel.Id: {}})
		c.Next()
	}, ResolveVideoSKUCapability(), ModelArkVideoChannelConstraint(), func(c *gin.Context) {
		value, ok := common.GetContextKey(c, constant.ContextKeyAssetAllowedChannelIDs)
		require.True(t, ok)
		allowed, ok = value.(map[int]struct{})
		require.True(t, ok)
		c.Status(http.StatusNoContent)
	})
	response := httptest.NewRecorder()
	engine.ServeHTTP(response, httptest.NewRequest(http.MethodPost, "/", nil))

	assert.Equal(t, http.StatusNoContent, response.Code)
	assert.Contains(t, allowed, channel.Id)
}
