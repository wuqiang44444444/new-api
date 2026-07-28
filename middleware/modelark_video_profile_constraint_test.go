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
		Key: "relay", OtherSettings: `{"video_upstream_profile":"third_party_relay"}`,
	}).Error)

	engine := gin.New()
	engine.POST("/", func(c *gin.Context) {
		relaycommon.SetVideoContractRequest(c, dto.VideoContractRequest{
			ContractID: dto.VideoContractModelArkV3,
			ModelArk: &dto.ModelArkVideoCreateRequest{
				Model: "seedance-model",
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
	}, ModelArkVideoChannelConstraint(), func(c *gin.Context) {
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
