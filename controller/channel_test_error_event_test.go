package controller

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strconv"
	"testing"
	"time"

	"github.com/QuantumNous/new-api/clienterrlog"
	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/setting/ratio_setting"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

// Follow the real test controller, adapter, event queue, persistence and query
// paths. Only the upstream and database are fixtures; no real channel is called.
func TestChannelTestFailurePersistsHTTPDiagnostics(t *testing.T) {
	for _, mode := range []string{"auto", "manual", "config_failure"} {
		t.Run(mode, func(t *testing.T) {
			previousDB, previousLogDB := model.DB, model.LOG_DB
			previousRedis, previousMemory := common.RedisEnabled, common.MemoryCacheEnabled
			previousDisable, previousEnable := common.AutomaticDisableChannelEnabled, common.AutomaticEnableChannelEnabled
			previousMainType, previousLogType := common.MainDatabaseType(), common.LogDatabaseType()
			t.Cleanup(func() {
				model.DB, model.LOG_DB = previousDB, previousLogDB
				common.RedisEnabled, common.MemoryCacheEnabled = previousRedis, previousMemory
				common.AutomaticDisableChannelEnabled, common.AutomaticEnableChannelEnabled = previousDisable, previousEnable
				common.SetDatabaseTypes(previousMainType, previousLogType)
			})
			db := setupModelListControllerTestDB(t)
			previousGroupRatios := ratio_setting.GroupRatio2JSONString()
			require.NoError(t, ratio_setting.UpdateGroupRatioByJSONString(`{"default":1}`))
			t.Cleanup(func() { require.NoError(t, ratio_setting.UpdateGroupRatioByJSONString(previousGroupRatios)) })
			previousRatios, err := common.Marshal(ratio_setting.GetModelRatioCopy())
			require.NoError(t, err)
			require.NoError(t, ratio_setting.UpdateModelRatioByJSONString(`{"gpt-4o":1}`))
			t.Cleanup(func() { require.NoError(t, ratio_setting.UpdateModelRatioByJSONString(string(previousRatios))) })
			common.MemoryCacheEnabled = false
			common.AutomaticDisableChannelEnabled, common.AutomaticEnableChannelEnabled = false, false
			require.NoError(t, model.MigrateErrorEvents())
			user := &model.User{Username: "test-operator", Group: "default", Status: common.UserStatusEnabled, Quota: 1000000}
			require.NoError(t, db.Create(user).Error)
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				assert.Equal(t, "POST", r.Method)
				assert.Equal(t, "/v1/chat/completions", r.URL.Path)
				var request map[string]any
				if assert.NoError(t, common.DecodeJson(r.Body, &request)) {
					assert.Equal(t, "gpt-4o", request["model"])
				}
				w.Header().Set("Content-Type", "application/json")
				w.Header().Set(common.RequestIdKey, "fixture-upstream-request")
				w.WriteHeader(http.StatusBadRequest)
				_, _ = w.Write([]byte(`{"error":{"code":"invalid_parameter","message":"invalid test parameter"},"api_key":"fixture-response-secret"}`))
			}))
			t.Cleanup(server.Close)
			channel := &model.Channel{Type: constant.ChannelTypeOpenAI, Name: "diagnostic fixture", Key: "fixture-channel-secret", Models: "gpt-4o", Group: "default", Status: common.ChannelStatusEnabled, BaseURL: common.GetPointer(server.URL)}
			if mode == "config_failure" {
				channel.ModelMapping = common.GetPointer("{")
			}
			require.NoError(t, db.Create(channel).Error)
			persisted := make(chan int, 1)
			require.NoError(t, db.Callback().Create().After("gorm:commit_or_rollback_transaction").Register("test:error_event_committed", func(tx *gorm.DB) {
				if row, ok := tx.Statement.Dest.(*model.ErrorEvent); ok && tx.Error == nil {
					persisted <- row.Id
				}
			}))
			if mode != "manual" {
				summary := testChannelForHealthCheck(context.Background(), channel, user.Id, false, 0, true)
				assert.Equal(t, 1, summary.Failed)
				assert.Zero(t, summary.Succeeded)
			} else {
				w := httptest.NewRecorder()
				c, _ := gin.CreateTestContext(w)
				c.Request = httptest.NewRequest(http.MethodGet, "/api/channel/test/"+strconv.Itoa(channel.Id), nil)
				c.Params = gin.Params{{Key: "id", Value: strconv.Itoa(channel.Id)}}
				c.Set("id", user.Id)
				c.Set("username", user.Username)
				TestChannel(c)
				assert.Equal(t, 200, w.Code)
				var response struct {
					Success bool `json:"success"`
				}
				require.NoError(t, common.Unmarshal(w.Body.Bytes(), &response))
				assert.False(t, response.Success)
			}
			select {
			case <-persisted:
			case <-time.After(5 * time.Second):
				t.Fatal("channel test failure was not persisted")
			}
			rows, total, err := model.GetErrorEvents(model.ErrorEventFilter{ChannelId: channel.Id}, 0, 10)
			require.NoError(t, err)
			require.EqualValues(t, 1, total)
			require.Len(t, rows, 1)
			row := rows[0]
			// Query through the real HTTP handler, not a mocked API response.
			for _, status := range []int{200, 400, 0} {
				w := httptest.NewRecorder()
				c, _ := gin.CreateTestContext(w)
				c.Request = httptest.NewRequest(http.MethodGet, "/api/error_log/?status="+strconv.Itoa(status), nil)
				GetErrorLogs(c)
				require.Equal(t, http.StatusOK, w.Code)
				var response struct {
					Success bool `json:"success"`
					Data    struct {
						Total int                `json:"total"`
						Items []model.ErrorEvent `json:"items"`
					} `json:"data"`
				}
				require.NoError(t, common.Unmarshal(w.Body.Bytes(), &response))
				require.True(t, response.Success)
				if (mode == "config_failure" && status == 0) || (mode != "config_failure" && status == 400) {
					assert.Equal(t, 1, response.Data.Total)
					require.Len(t, response.Data.Items, 1)
					assert.Equal(t, row.RequestId, response.Data.Items[0].RequestId)
				} else {
					assert.Zero(t, response.Data.Total)
					assert.Empty(t, response.Data.Items)
				}
			}
			assert.Equal(t, "POST", row.Method)
			assert.Equal(t, "/v1/chat/completions", row.Route)
			assert.Equal(t, "openai", row.Protocol)
			if mode != "manual" {
				assert.Zero(t, row.Status)
				assert.Zero(t, row.UserId)
				assert.Empty(t, row.Username)
			} else {
				assert.Equal(t, 200, row.Status)
				assert.Equal(t, user.Id, row.UserId)
				assert.Equal(t, user.Username, row.Username)
			}
			var detail map[string]string
			require.NoError(t, common.UnmarshalJsonStr(row.Detail, &detail))
			if mode == "manual" {
				assert.Equal(t, "manual", detail["test_mode"])
			} else {
				assert.Equal(t, "auto", detail["test_mode"])
			}
			var exchange clienterrlog.HTTPExchange
			require.NoError(t, common.UnmarshalJsonStr(detail[clienterrlog.HTTPExchangeDetailKey], &exchange))
			assert.Equal(t, "captured", exchange.Request.State)
			assert.Contains(t, exchange.Request.Body, `"hi"`)
			if mode == "config_failure" {
				assert.Equal(t, "test_config_error", row.Reason)
				assert.Empty(t, row.UpstreamRequestId)
				assert.NotContains(t, detail, "upstream_status")
				assert.Equal(t, "not_recorded", exchange.UpstreamRequest.State)
				assert.Equal(t, "not_recorded", exchange.UpstreamResponse.State)
				return
			}
			assert.Equal(t, "test_upstream_rejected", row.Reason)
			assert.Equal(t, "fixture-upstream-request", row.UpstreamRequestId)
			assert.Equal(t, "400", detail["upstream_status"])
			assert.Equal(t, "captured", exchange.UpstreamRequest.State)
			assert.Contains(t, exchange.UpstreamRequest.Body, `"gpt-4o"`)
			assert.Equal(t, "captured", exchange.UpstreamResponse.State)
			assert.Equal(t, 400, exchange.UpstreamResponse.Status)
			assert.Contains(t, exchange.UpstreamResponse.Body, "invalid test parameter")
			assert.Equal(t, "not_recorded", exchange.Response.State)
			assert.NotContains(t, row.Detail, "fixture-channel-secret")
			assert.NotContains(t, row.Detail, "fixture-response-secret")
		})
	}
}
