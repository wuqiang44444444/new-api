package controller

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/QuantumNous/new-api/clienterrlog"
	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/dto"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/plugins"
	"github.com/QuantumNous/new-api/relay"
	taskseedance "github.com/QuantumNous/new-api/relay/channel/task/seedance"
	relaycommon "github.com/QuantumNous/new-api/relay/common"
	"github.com/QuantumNous/new-api/service"
	"github.com/QuantumNous/new-api/setting/system_setting"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestTaskSubmissionKeepsProviderTaskAndCreationRequestIDsSeparate(t *testing.T) {
	for _, traceID := range []string{"provider-create-request", ""} {
		t.Run("trace="+traceID, func(t *testing.T) {
			events := []string{}
			db := setupTaskSubmissionDatabase(t, true, &events)
			oldLogConsumeEnabled := common.LogConsumeEnabled
			common.LogConsumeEnabled = false
			t.Cleanup(func() { common.LogConsumeEnabled = oldLogConsumeEnabled })
			c := taskSubmissionTestContext()
			c.Set(common.RequestIdKey, "platform-create-request")
			c.Set(common.UpstreamRequestIdKey, traceID)
			info := taskSubmissionRelayInfo(&taskSubmissionTestBilling{events: &events})
			outcome, taskErr := executeTaskSubmissionWith(c, info, func(*gin.Context, *relaycommon.RelayInfo) (*relay.TaskSubmitResult, *dto.TaskError) {
				return &relay.TaskSubmitResult{Platform: constant.TaskPlatform("document-parser"), UpstreamTaskID: "provider-task"}, nil
			})
			require.Nil(t, taskErr)
			require.NotNil(t, outcome)
			var saved model.Task
			require.NoError(t, db.First(&saved, outcome.Task.ID).Error)
			assert.Equal(t, "provider-task", saved.PrivateData.UpstreamTaskID)
			assert.NotEqual(t, saved.TaskID, saved.PrivateData.UpstreamTaskID)
			assert.Equal(t, traceID, saved.PrivateData.UpstreamRequestID)
			require.NotNil(t, saved.PrivateData.Execution)
			assert.Equal(t, "platform-create-request", saved.PrivateData.Execution.RequestID)
		})
	}
}

func TestViduFailureCommitsPublicErrorEventAndRefundOnce(t *testing.T) {
	for _, tc := range []struct{ name, code, wantCode, wantMessage, createRequestID string }{
		{"audit", "AuditSubmitIllegal", "AuditSubmitIllegal", "输入内容未通过安全审核 (input content failed the upstream safety review)", "provider-create-request"},
		{"audit-without-trace", "AuditSubmitIllegal", "AuditSubmitIllegal", "输入内容未通过安全审核 (input content failed the upstream safety review)", ""},
		{"credential", "access_token:fixture-secret", "generation_failed", "视频生成失败，上游未提供详细原因 (video generation failed; the upstream did not provide a detailed reason)", ""},
	} {
		t.Run(tc.name, func(t *testing.T) {
			task := setupGenericTaskTest(t)
			db := model.DB
			sqlDB, err := db.DB()
			require.NoError(t, err)
			sqlDB.SetMaxOpenConns(1)
			require.NoError(t, db.AutoMigrate(&model.TaskPlugin{}, &model.Token{}, &model.ErrorEvent{}, &model.Log{}, &model.TaskBillingDelivery{}, &model.QuotaData{}, &model.TaskRequestEvidence{}, &model.TaskRequestEvidenceEvent{}, &model.TaskRequestEvidenceAccessLog{}))
			oldLogDB, oldFactory := model.LOG_DB, service.GetTaskAdaptorFunc
			oldConfig := system_setting.GetTaskRequestEvidenceConfig()
			model.LOG_DB = db
			service.GetTaskAdaptorFunc = func(constant.TaskPlatform) service.TaskPollingAdaptor { return &taskseedance.TaskAdaptor{} }
			config := system_setting.TaskRequestEvidenceConfig{Enabled: true, StorageDir: t.TempDir(), EncryptionKeyHex: strings.Repeat("01", 32), MaxBodyBytes: 4096, MaxResponseBytes: 4096, WriteTimeoutSeconds: 5}
			system_setting.SetTaskRequestEvidenceConfig(config)
			require.NoError(t, service.InitTaskRequestEvidenceStore(config))
			t.Cleanup(func() {
				model.LOG_DB, service.GetTaskAdaptorFunc = oldLogDB, oldFactory
				system_setting.SetTaskRequestEvidenceConfig(oldConfig)
				_ = service.InitTaskRequestEvidenceStore(oldConfig)
			})
			allowPrivateTaskMediaTest(t)
			plugin := model.TaskPlugin{Key: "seedance-link", Version: plugins.SeedanceVersion(), Source: plugins.SeedanceSource(), SourceHash: fmt.Sprintf("%x", common.Sha256Raw([]byte(plugins.SeedanceSource()))), Enabled: true, Active: true}
			require.NoError(t, db.Create(&plugin).Error)
			require.NoError(t, taskseedance.SyncExtensionSnapshot(context.Background(), []model.TaskPlugin{plugin}))
			t.Cleanup(func() { require.NoError(t, taskseedance.SyncExtensionSnapshot(context.Background(), nil)) })
			body, err := common.Marshal(map[string]any{"id": "upstream-fixture", "status": "failed", "error": map[string]string{"code": tc.code}})
			require.NoError(t, err)
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if r.Method == http.MethodPost {
					assert.Equal(t, "/ent/api/v3/contents/generations/tasks", r.URL.Path)
					w.Header().Set("Content-Type", "application/json")
					if tc.createRequestID != "" {
						w.Header().Set("X-Request-Id", tc.createRequestID)
					}
					_, _ = io.WriteString(w, `{"id":"upstream-fixture"}`)
					return
				}
				assert.Equal(t, http.MethodGet, r.Method)
				assert.Equal(t, "/ent/api/v3/contents/generations/tasks/upstream-fixture", r.URL.Path)
				w.Header().Set("Content-Type", "application/json")
				_, _ = w.Write(body)
			}))
			defer server.Close()
			// Exercise the real creation transport before terminal polling. The
			// fixture below represents an already-held task; it does not re-test pricing.
			c, _ := gin.CreateTestContext(httptest.NewRecorder())
			c.Request = httptest.NewRequest(http.MethodPost, "/api/v3/contents/generations/tasks", strings.NewReader(`{"model":"customer-video","content":[{"type":"text","text":"fixture"}]}`))
			c.Request.Header.Set("Content-Type", "application/json")
			c.Set("id", task.UserId)
			c.Set(common.RequestIdKey, "create-fixture")
			info := &relaycommon.RelayInfo{UserId: task.UserId, OriginModelName: "customer-video", ChannelMeta: &relaycommon.ChannelMeta{ChannelType: constant.ChannelTypeSeedanceLink, ChannelId: task.ChannelId, ChannelBaseUrl: server.URL + "/ent", ApiKey: "fixture-key", UpstreamModelName: "viduq3-drama-std", ChannelOtherSettings: dto.ChannelOtherSettings{VideoUpstreamProtocol: dto.VideoUpstreamProtocolViduModelArkV3}}}
			adaptor := &taskseedance.TaskAdaptor{}
			adaptor.Init(info)
			require.NoError(t, taskseedance.PinSeedanceExtensionForChannel(c, dto.VideoUpstreamProtocolViduModelArkV3))
			var contract dto.ModelArkVideoCreateRequest
			require.NoError(t, common.Unmarshal([]byte(`{"model":"customer-video","content":[{"type":"text","text":"fixture"}]}`), &contract))
			relaycommon.SetVideoContractRequest(c, dto.VideoContractRequest{ContractID: dto.VideoContractModelArkV3, ModelArk: &contract})
			require.NoError(t, service.BeginTaskRequestEvidence(c, model.TaskRequestEvidenceKindVideoTask))
			requestBody, err := adaptor.BuildRequestBody(c, info)
			require.NoError(t, err)
			service.InitHttpClient()
			resp, err := adaptor.DoRequest(c, info, requestBody)
			require.NoError(t, err)
			created, createErr := adaptor.ParseResponse(c, resp, info)
			require.Nil(t, createErr)
			require.NotNil(t, created)
			assert.Equal(t, tc.createRequestID, c.GetString(common.UpstreamRequestIdKey))
			require.NoError(t, db.Model(&model.User{}).Where("id = ?", task.UserId).Update("quota", 9300).Error)
			token := model.Token{UserId: task.UserId, Key: "vidu-failure-fixture", RemainQuota: 9300, UsedQuota: 700, Status: common.TokenStatusEnabled}
			require.NoError(t, db.Create(&token).Error)
			task.Platform = constant.TaskPlatform(fmt.Sprint(constant.ChannelTypeSeedanceLink))
			task.ClientProtocol, task.Status, task.Quota = model.TaskClientProtocolModelArkV3, model.TaskStatusInProgress, 700
			task.Properties = model.Properties{OriginModelName: "customer-video", UpstreamModelName: "viduq3-drama-std"}
			task.BillingState = model.TaskBillingStatePending
			task.PrivateData = model.TaskPrivateData{
				Key: "fixture-key", UpstreamTaskID: created.UpstreamTaskID, UpstreamRequestID: c.GetString(common.UpstreamRequestIdKey), BillingSource: "wallet", TokenId: token.Id,
				VideoUpstreamProtocol:          dto.VideoUpstreamProtocolViduModelArkV3,
				VideoUpstreamProfile:           dto.VideoUpstreamProfileThirdPartyViduModelArkV3,
				VideoUpstreamQueryPathTemplate: "/ent/api/v3/contents/generations/tasks/{task_id}",
				VideoUpstreamQueryBaseURL:      server.URL, SouthboundAdapterVersion: relaycommon.CurrentVideoSouthboundAdapterVersion(constant.ChannelTypeSeedanceLink, dto.VideoUpstreamProfileThirdPartyViduModelArkV3),
				Execution:      &model.TaskExecutionSnapshot{RequestID: "create-fixture", TaskPlugin: &model.TaskPluginSnapshot{Key: "seedance-link", Version: plugins.SeedanceVersion(), APIVersion: 3}},
				AsyncBilling:   &model.TaskAsyncBillingContext{State: model.TaskBillingStatePending},
				BillingContext: &model.TaskBillingContext{OriginModelName: "customer-video", GroupRatio: 1},
			}
			require.NoError(t, db.Save(task).Error)
			service.AttachTaskRequestEvidenceTask(c, task)
			c.JSON(http.StatusOK, gin.H{"id": task.TaskID, "status": "queued"})
			service.FinishTaskRequestEvidenceClientDelivery(c)
			var stale model.Task
			require.NoError(t, db.First(&stale, task.ID).Error)
			accepted := clienterrlog.CurrentHealth().Accepted
			require.NoError(t, service.RefreshVideoTask(context.Background(), task))
			var saved model.Task
			require.NoError(t, db.First(&saved, task.ID).Error)
			require.EqualValues(t, model.TaskStatusFailure, saved.Status)
			assert.Equal(t, tc.wantMessage, saved.FailReason)
			assert.Equal(t, tc.wantCode, saved.ToModelArkVideoTask().Error.Code)
			assert.Equal(t, tc.wantMessage, saved.ToOpenAIVideo().Error.Message)
			assert.NotContains(t, string(saved.Data), "fixture-secret")
			assert.Zero(t, saved.Quota)
			assert.Equal(t, model.TaskBillingStateSettled, saved.BillingState)
			var events []model.ErrorEvent
			require.Eventually(t, func() bool {
				return db.Where("task_id = ?", task.TaskID).Find(&events).Error == nil && len(events) == 1
			}, 3*time.Second, 10*time.Millisecond)
			assert.Equal(t, tc.wantCode, events[0].PublicCode)
			assert.Equal(t, "task_failed", events[0].Reason)
			assert.Zero(t, events[0].Status)
			assert.Equal(t, "create-fixture", events[0].RequestId)
			var detail map[string]string
			require.NoError(t, common.Unmarshal([]byte(events[0].Detail), &detail))
			assert.Equal(t, tc.wantMessage, detail["fail_reason"])
			assert.Equal(t, tc.createRequestID, detail["create_upstream_request_id"])
			assert.Equal(t, tc.createRequestID, saved.PrivateData.UpstreamRequestID)
			assert.Equal(t, "upstream-fixture", saved.PrivateData.UpstreamTaskID)

			// A stale observer loses CAS; a fresh terminal read skips polling. Neither
			// may emit a second event or refund the wallet/token again.
			require.NoError(t, service.RefreshVideoTask(context.Background(), &stale))
			require.NoError(t, service.RefreshVideoTask(context.Background(), &saved))
			assert.Equal(t, accepted+1, clienterrlog.CurrentHealth().Accepted)
			var user model.User
			require.NoError(t, db.First(&user, task.UserId).Error)
			require.NoError(t, db.First(&token, token.Id).Error)
			assert.Equal(t, 10000, user.Quota)
			assert.Equal(t, 10000, token.RemainQuota)
			assert.Zero(t, token.UsedQuota)
			var deliveries []model.TaskBillingDelivery
			require.NoError(t, db.Where("task_row_id = ? AND event = ?", task.ID, "refund").Find(&deliveries).Error)
			require.Len(t, deliveries, 1)
			assert.Equal(t, 700, deliveries[0].BeforeQuota)
			assert.Zero(t, deliveries[0].AfterQuota)
			// Creation and failure share the same evidence session and immutable IDs.
			indexes, total, queryErr := model.QueryTaskRequestEvidence(model.TaskRequestEvidenceQueryParams{TaskID: task.TaskID, Num: 20})
			require.NoError(t, queryErr)
			require.EqualValues(t, 1, total)
			require.Len(t, indexes, 1)
			assert.Equal(t, "create-fixture", indexes[0].RequestID)
			assert.Equal(t, tc.createRequestID, indexes[0].UpstreamRequestID)
			evidenceEvents, err := model.ListTaskRequestEvidenceEvents(indexes[0].Id)
			require.NoError(t, err)
			require.GreaterOrEqual(t, len(evidenceEvents), 5)
			for i, stage := range []string{model.TaskRequestEvidenceStageNorthReceive, model.TaskRequestEvidenceStageSouthboundSend, model.TaskRequestEvidenceStageUpstreamResponse, model.TaskRequestEvidenceStageClientDelivery, model.TaskRequestEvidenceStagePolling} {
				assert.Equal(t, stage, evidenceEvents[i].Stage)
				assert.True(t, evidenceEvents[i].Complete)
				if i >= 2 {
					assert.Equal(t, http.StatusOK, evidenceEvents[i].StatusCode)
				}
				payload, err := service.GetTaskRequestEvidenceStore().Get(evidenceEvents[i].ObjectKey)
				require.NoError(t, err)
				assert.NotEmpty(t, payload)
			}
		})
	}
}
