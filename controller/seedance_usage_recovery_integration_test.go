package controller

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/dto"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/pkg/billingexpr"
	"github.com/QuantumNous/new-api/relay/channel/task/seedance"
	relaycommon "github.com/QuantumNous/new-api/relay/common"
	"github.com/QuantumNous/new-api/service"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestSeedanceHTTPObservationAndBackgroundSettlement(t *testing.T) {
	for _, protocol := range []dto.VideoUpstreamProtocol{
		dto.VideoUpstreamProtocolModelArkV3Volcengine, dto.VideoUpstreamProtocolModelArkV3BytePlus,
		dto.VideoUpstreamProtocolModelArkV3CMCC, dto.VideoUpstreamProtocolMoxingModelArkV1,
		dto.VideoUpstreamProtocolFunCloudSeedance, dto.VideoUpstreamProtocolFunCloudModelArkV3,
		dto.VideoUpstreamProtocolArkMediaV1, dto.VideoUpstreamProtocolTokenSaveMediaTaskV1,
		dto.VideoUpstreamProtocolMoxingMediaTaskV1, dto.VideoUpstreamProtocolFeicaiVideosV1,
	} {
		t.Run(string(protocol), func(t *testing.T) {
			events := []string{}
			db := setupTaskSubmissionDatabase(t, true, &events)
			sqlDB, err := db.DB()
			require.NoError(t, err)
			sqlDB.SetMaxOpenConns(1)
			require.NoError(t, db.AutoMigrate(&model.User{}, &model.Token{}, &model.Log{}, &model.Channel{}, &model.UserSubscription{}))
			oldLog, oldMemory, oldRedis, oldBatch, oldConsume := model.LOG_DB, common.MemoryCacheEnabled, common.RedisEnabled, common.BatchUpdateEnabled, common.LogConsumeEnabled
			common.SetDatabaseTypes(common.DatabaseTypeSQLite, common.DatabaseTypeSQLite)
			t.Setenv("LOG_SQL_DSN", "")
			require.NoError(t, model.InitLogDB())
			model.LOG_DB = db
			common.MemoryCacheEnabled, common.RedisEnabled, common.BatchUpdateEnabled, common.LogConsumeEnabled = false, false, false, false
			oldFactory := service.GetTaskAdaptorFunc
			service.GetTaskAdaptorFunc = func(constant.TaskPlatform) service.TaskPollingAdaptor { return &seedance.TaskAdaptor{} }
			service.InitHttpClient()
			t.Cleanup(func() {
				service.GetTaskAdaptorFunc = oldFactory
				model.LOG_DB = oldLog
				common.MemoryCacheEnabled, common.RedisEnabled, common.BatchUpdateEnabled, common.LogConsumeEnabled = oldMemory, oldRedis, oldBatch, oldConsume
			})
			var calls atomic.Int32
			seconds := protocol == dto.VideoUpstreamProtocolTokenSaveMediaTaskV1 || protocol == dto.VideoUpstreamProtocolMoxingMediaTaskV1 || protocol == dto.VideoUpstreamProtocolFeicaiVideosV1
			_, path := protocol.TransportPaths("seedance-2-fast")
			if path == "" {
				path = "/api/v3/contents/generations/tasks/{task_id}"
			}
			expectedPath := strings.ReplaceAll(path, "{task_id}", "provider-task")
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				n := calls.Add(1)
				assert.Equal(t, http.MethodGet, r.Method)
				assert.Equal(t, expectedPath, r.URL.Path)
				assert.Equal(t, "Bearer frozen-fixture-key", r.Header.Get("Authorization"))
				usage := ""
				if n > 1 {
					usage = `,"usage":{"completion_tokens":100,"total_tokens":120}`
				}
				var body string
				switch protocol.TransportProfile() {
				case dto.VideoUpstreamProfileThirdPartyFunCloudSeedance:
					tokens := ""
					if n > 1 {
						tokens = `,"completionTokens":100`
					}
					body = `{"code":0,"data":{"taskId":"provider-task","status":"success","result":["https://cdn.example.com/video.mp4"]` + tokens + `}}`
				case dto.VideoUpstreamProfileThirdPartyRelay, dto.VideoUpstreamProfileThirdPartyMoxingModelArk:
					body = `{"data":{"task_id":"provider-task","status":"succeeded","result":"https://cdn.example.com/video.mp4"` + usage + `}}`
				case dto.VideoUpstreamProfileThirdPartyFeicaiVideos:
					body = fmt.Sprintf(`{"id":"provider-task","status":"completed","video_url":"http://%s/video.mp4"}`, r.Host)
				case dto.VideoUpstreamProfileThirdPartyReverseProxy:
					body = `{"data":{"id":"provider-task","status":"succeeded","content":{"video_url":"https://cdn.example.com/video.mp4"}` + usage + `}}`
				default:
					body = `{"id":"provider-task","status":"succeeded","content":{"video_url":"https://cdn.example.com/video.mp4"}` + usage + `}`
				}
				w.Header().Set("Content-Type", "application/json")
				_, err := w.Write([]byte(body))
				assert.NoError(t, err)
			}))
			defer server.Close()
			require.NoError(t, db.Create(&model.User{Id: 8170, Username: "recovery-integration", Quota: 1000}).Error)
			expr := `tier("tokens",c*2)`
			expectedQuota := 100
			if seconds {
				expr = `tier("seconds",param("_task.duration_seconds")*100)`
				expectedQuota = 200
			}
			task := &model.Task{
				TaskID: "task-recovery", UserId: 8170, AppID: 8171, Quota: 700,
				Platform:       constant.TaskPlatform(strconv.Itoa(constant.ChannelTypeSeedanceLink)),
				ClientProtocol: model.TaskClientProtocolModelArkV3, Status: model.TaskStatusInProgress, BillingState: model.TaskBillingStatePending,
				Properties: model.Properties{OriginModelName: "customer-model", UpstreamModelName: "seedance-2-fast"},
				PrivateData: model.TaskPrivateData{
					VideoUpstreamProtocol: protocol, VideoUpstreamProfile: protocol.TransportProfile(),
					SouthboundAdapterVersion: relaycommon.CurrentVideoSouthboundAdapterVersion(constant.ChannelTypeSeedanceLink, protocol.TransportProfile()),
					Key:                      "frozen-fixture-key", VideoUpstreamQueryBaseURL: server.URL, VideoUpstreamQueryPathTemplate: path, UpstreamTaskID: "provider-task",
					AsyncBilling: &model.TaskAsyncBillingContext{State: model.TaskBillingStatePending, EstimatedTokens: 1000,
						TieredSnapshot: &billingexpr.BillingSnapshot{BillingMode: "tiered_expr", ModelName: "customer-model", ExprString: expr, ExprHash: billingexpr.ExprHashString(expr), GroupRatio: 1, QuotaPerUnit: 500000, EstimatedQuotaAfterGroup: 700},
						BillingProbe:   &billingexpr.RequestInput{Body: []byte(`{"_task":{"duration_seconds":4,"resolution":"720p","has_video_input":false}}`)},
					},
				},
			}
			require.NoError(t, db.Create(task).Error)
			require.NoError(t, service.RefreshVideoTask(context.Background(), task))
			var saved model.Task
			require.NoError(t, db.First(&saved, task.ID).Error)
			require.EqualValues(t, model.TaskStatusSuccess, saved.Status)
			if !seconds {
				require.Equal(t, model.TaskBillingStateAwaitingUsage, saved.BillingState)
				assert.Equal(t, 700, saved.Quota)
				// No client GET: real adapter HTTP GET is driven by the background worker.
				service.ReconcileTaskUsage(context.Background())
			}
			require.NoError(t, db.First(&saved, task.ID).Error)
			assert.Equal(t, model.TaskBillingStateSettled, saved.BillingState)
			assert.Equal(t, expectedQuota, saved.Quota)
			var user model.User
			require.NoError(t, db.First(&user, 8170).Error)
			assert.Equal(t, 1700-expectedQuota, user.Quota)
			before := calls.Load()
			service.ReconcileTaskUsage(context.Background())
			assert.Equal(t, before, calls.Load())
		})
	}
}
