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
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestMiniMaxAssetOperationsUnsupportedWithoutRevealingHiddenModels(t *testing.T) {
	fx := newMiniMaxFundsFixture(t)
	require.NoError(t, config.GlobalConfig.LoadFromDB(map[string]string{"asset_setting.enabled": "true"}))
	catalog, err := model.GetConfiguredMiniMaxPublicModels()
	require.NoError(t, err)
	require.Len(t, catalog, 1)
	assert.Nil(t, catalog[0].API.Assets)
	cases := []struct {
		name, method, path, body string
		handler                  gin.HandlerFunc
	}{
		{"create asset", "POST", "/v1/assets", `{"model":"customer-video","name":"example","asset_kind":"general","media_type":"image","source":{"type":"url","url":"https://1.1.1.1/example.jpg"}}`, CreateAsset},
		{"get asset", "GET", "/v1/assets/example?model=customer-video", "", GetAsset},
		{"update asset", "PATCH", "/v1/assets/example", `{"model":"customer-video","name":"example"}`, UpdateAsset},
		{"delete asset", "DELETE", "/v1/assets/example?model=customer-video", "", DeleteAsset},
		{"create group", "POST", "/v1/asset-groups", `{"model":"customer-video","name":"example","group_kind":"general"}`, CreateAssetGroup},
		{"get group", "GET", "/v1/asset-groups/example?model=customer-video", "", GetAssetGroup},
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
					assert.Equal(t, http.StatusUnprocessableEntity, w.Code, w.Body.String())
					assert.Contains(t, w.Body.String(), "unsupported_asset_operation")
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
