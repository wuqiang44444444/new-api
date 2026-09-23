package controller

import (
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/QuantumNous/new-api/clienterrlog"
	logtest "github.com/QuantumNous/new-api/clienterrlog/testutil"
	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/relaykit/dto"
	"github.com/QuantumNous/new-api/service"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestAssetUpstreamDiagnosticPersistsThroughHandler(t *testing.T) {
	for _, scenario := range []struct {
		name, body, reason string
		upstreamStatus     int
	}{
		{"http_rejection", `{"error":"private-provider-body"}`, "upstream_http_error", 503},
		{"invalid_json", `private-invalid-json`, "upstream_invalid_response", 200},
	} {
		t.Run(scenario.name, func(t *testing.T) {
			previousDB, previousLogDB := model.DB, model.LOG_DB
			previousMain, previousLog, previousRedis := common.MainDatabaseType(), common.LogDatabaseType(), common.RedisEnabled
			t.Cleanup(func() {
				model.DB, model.LOG_DB = previousDB, previousLogDB
				common.SetDatabaseTypes(previousMain, previousLog)
				common.RedisEnabled = previousRedis
			})
			db := setupModelListControllerTestDB(t)
			require.NoError(t, db.AutoMigrate(&model.ErrorEvent{}))
			seedPublishedSeedanceControllerArtifact(t)
			upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.Header().Set("Content-Type", "application/json")
				w.WriteHeader(scenario.upstreamStatus)
				_, _ = w.Write([]byte(scenario.body))
			}))
			defer upstream.Close()
			service.InitHttpClient()
			channel := model.Channel{Type: constant.ChannelTypeSeedanceLink, Status: common.ChannelStatusEnabled, Models: "customer-model", Group: "default", Key: "fixture", BaseURL: common.GetPointer(upstream.URL)}
			channel.SetOtherSettings(dto.ChannelOtherSettings{VideoUpstreamProtocol: dto.VideoUpstreamProtocolMoxingModelArkV1, AssetUpstreamProtocol: dto.AssetUpstreamProtocolMoxingVolcAssetsV1})
			require.NoError(t, db.Create(&channel).Error)
			sink := logtest.New(t)
			engine := gin.New()
			engine.Use(sink.Middleware(), func(c *gin.Context) {
				c.Set(common.RequestIdKey, "asset-"+scenario.name)
				c.Set("route_tag", "asset")
				c.Set("id", 7)
				common.SetContextKey(c, constant.ContextKeyUsingGroup, "default")
				clienterrlog.MarkAuthPassed(c, clienterrlog.AuthSourceAPIToken)
			})
			engine.GET("/v1/assets/:asset_id", GetAsset)
			response := httptest.NewRecorder()
			engine.ServeHTTP(response, httptest.NewRequest(http.MethodGet, "/v1/assets/opaque?model=customer-model", nil))
			require.Equal(t, http.StatusBadGateway, response.Code, response.Body.String())
			assert.Contains(t, response.Body.String(), `"code":"asset_upstream_error"`)
			assert.NotContains(t, response.Body.String(), "private")
			assert.NotContains(t, response.Body.String(), upstream.URL)
			require.Eventually(t, func() bool { return sink.Health().Persisted == 1 }, time.Second, time.Millisecond)
			assert.EqualValues(t, 1, sink.Health().Accepted)
			rows, total, err := model.GetErrorEvents(model.ErrorEventFilter{RequestId: "asset-" + scenario.name}, 0, 10)
			require.NoError(t, err)
			require.EqualValues(t, 1, total)
			require.Len(t, rows, 1)
			assert.Equal(t, scenario.reason, rows[0].Reason)
			assert.Equal(t, "upstream_operation", rows[0].Stage)
			assert.Equal(t, "customer-model", rows[0].ModelName)
			assert.NotContains(t, rows[0].Detail, "private")
			var detail map[string]string
			require.NoError(t, common.UnmarshalJsonStr(rows[0].Detail, &detail))
			assert.Equal(t, "get", detail["operation"])
			if scenario.name == "http_rejection" {
				assert.Equal(t, "503", detail["upstream_status"])
			}
		})
	}
}
