package controller

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/setting/config"
	"github.com/QuantumNous/new-api/setting/system_setting"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestMiniMaxHostedAssetGroupAndUnsupportedOperations(t *testing.T) {
	fx := newMiniMaxFundsFixture(t)
	require.NoError(t, config.GlobalConfig.LoadFromDB(map[string]string{"asset_setting.enabled": "true"}))
	catalog, err := model.GetConfiguredMiniMaxPublicModels()
	require.NoError(t, err)
	require.Len(t, catalog, 1)
	require.NotNil(t, catalog[0].API.Assets)
	assert.Equal(t, "platform_hosted", catalog[0].API.Assets.ManagementMode)
	require.NoError(t, fx.db.AutoMigrate(&model.FunCloudHostedAsset{}, &model.FunCloudHostedAssetGroup{}))
	cases := []struct {
		name, method, path, body string
		handler                  gin.HandlerFunc
		status                   int
		code                     string
	}{
		{"get asset", "GET", "/v1/assets/example?model=customer-video", "", GetAsset, http.StatusNotFound, "asset_not_found"},
		{"update asset", "PATCH", "/v1/assets/example", `{"model":"customer-video","name":"example"}`, UpdateAsset, http.StatusUnprocessableEntity, "unsupported_asset_operation"},
		{"delete asset", "DELETE", "/v1/assets/example?model=customer-video", "", DeleteAsset, http.StatusNotFound, "asset_not_found"},
		{"create group", "POST", "/v1/asset-groups", `{"model":"customer-video","name":"example","group_kind":"general"}`, CreateAssetGroup, http.StatusCreated, "asset_group"},
		{"get group", "GET", "/v1/asset-groups/example?model=customer-video", "", GetAssetGroup, http.StatusNotFound, "asset_not_found"},
	}
	for _, tc := range cases {
		for _, group := range []string{"default", "not-authorized"} {
			t.Run(tc.name+"/"+group, func(t *testing.T) {
				w := httptest.NewRecorder()
				c, _ := gin.CreateTestContext(w)
				c.Request = httptest.NewRequest(tc.method, tc.path, strings.NewReader(tc.body))
				c.Request.Header.Set("Content-Type", "application/json")
				c.Params = gin.Params{{Key: "asset_id", Value: "example"}, {Key: "group_id", Value: "example"}}
				c.Set("id", fx.userID)
				common.SetContextKey(c, constant.ContextKeyUsingGroup, group)
				tc.handler(c)
				if group == "default" {
					assert.Equal(t, tc.status, w.Code, w.Body.String())
					assert.Contains(t, w.Body.String(), tc.code)
				} else {
					assert.Equal(t, http.StatusNotFound, w.Code, w.Body.String())
					assert.Contains(t, w.Body.String(), "model_not_found")
				}
				assert.Zero(t, fx.createCalls.Load())
				assert.Zero(t, fx.queryCalls.Load())
			})
		}
	}
	assert.Equal(t, seedanceFundsInitialQuota, fx.userQuota())
}

func TestStandardVideoUsesHostedAssetFromAnotherModel(t *testing.T) {
	for _, tc := range []struct {
		name    string
		fixture func(*testing.T) *seedanceFundsFixture
		options string
	}{
		{"MiniMax nested media", func(t *testing.T) *seedanceFundsFixture { return newMiniMaxFundsFixture(t) }, `"duration":6,"resolution":"768p","ratio":"16:9"`},
		{"Seedance flattened media", newSeedanceFundsFixture, `"duration":4,"resolution":"720p","ratio":"21:9"`},
	} {
		t.Run(tc.name, func(t *testing.T) {
			fx := tc.fixture(t)
			require.NoError(t, fx.db.AutoMigrate(&model.FunCloudHostedAsset{}))
			store := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				assert.Equal(t, http.MethodHead, r.Method)
				assert.Equal(t, "/assets/fixture.jpg", r.URL.Path)
				w.Header().Set("Content-Length", "12")
			}))
			t.Cleanup(store.Close)
			storage := system_setting.ObjectStorageConfig{Backend: "s3", Endpoint: store.URL, Bucket: "assets", AccountName: "fixture", Region: "us-east-1", Credential: "fixture-secret", Revision: "shared-hosted"}
			data, err := common.Marshal(storage)
			require.NoError(t, err)
			model.NotifyObjectStorageSettingUpdate(string(data))
			t.Cleanup(func() { model.NotifyObjectStorageSettingUpdate("") })
			require.NoError(t, fx.db.Create(&model.FunCloudHostedAsset{
				ID: "fhas_cross_model", UserID: fx.userID, Model: "different-video-model", Name: "reference",
				StorageLocation: model.FunCloudHostedStorageLocation{Backend: "s3", Endpoint: store.URL, Bucket: "assets"},
				ObjectKey:       "fixture.jpg", MediaType: "image", MimeType: "image/jpeg", SizeBytes: 12, Status: model.FunCloudHostedAssetStatusReady,
			}).Error)
			response := fx.submitCreateBody(`{"model":"customer-video",` + tc.options + `,"content":[{"type":"text","text":"A landscape"},{"type":"image_url","role":"reference_image","image_url":{"url":"asset://fhas_cross_model"}}]}`)
			require.Contains(t, response, `"id":`)
			assert.Equal(t, int32(1), fx.createCalls.Load())
			upstreamBody := string(fx.createRequestBody())
			assert.Contains(t, upstreamBody, "/assets/fixture.jpg?")
			assert.Contains(t, upstreamBody, "X-Amz-Signature")
			assert.NotContains(t, upstreamBody, "asset://")
			assert.NotContains(t, upstreamBody, "hosted-media.invalid")
			var task model.Task
			require.NoError(t, fx.db.First(&task).Error)
			require.Len(t, task.PrivateData.HostedMedia, 1)
			assert.Equal(t, "fhas_cross_model", task.PrivateData.HostedMedia[0].AssetID)
			snapshot, err := common.Marshal(task.PrivateData)
			require.NoError(t, err)
			assert.NotContains(t, string(snapshot), "X-Amz-Signature")
		})
	}
}
