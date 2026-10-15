package controller

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/relay/channel"
	relaycommon "github.com/QuantumNous/new-api/relay/common"
	"github.com/QuantumNous/new-api/service"
	"github.com/QuantumNous/new-api/setting/system_setting"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestEvidenceCleanupDoesNotInterruptBusinessOrChangeAccounting(t *testing.T) {
	previousConfig := system_setting.GetTaskRequestEvidenceConfig()
	t.Cleanup(func() {
		system_setting.SetTaskRequestEvidenceConfig(previousConfig)
		_ = service.InitTaskRequestEvidenceStore(previousConfig)
	})
	for _, tc := range []struct {
		name      string
		scheduled bool
		accepted  bool
		afterSend bool
	}{
		{name: "manual accepted before send", accepted: true},
		{name: "manual completed before send"},
		{name: "automatic expiry before send", scheduled: true},
		{name: "manual completed during response", afterSend: true},
		{name: "automatic expiry during response", scheduled: true, afterSend: true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			_, dir := setupEvidenceCleanupEnv(t)
			require.NoError(t, model.DB.AutoMigrate(&model.User{}, &model.Token{}, &model.Channel{}, &model.Log{}, &model.TaskCreateAttempt{}))
			now := common.GetTimestamp()
			user := &model.User{Id: 7, Username: "cleanup-fixture", Quota: 1000, UsedQuota: 300, RequestCount: 4}
			token := &model.Token{Id: 3, UserId: 7, Key: "local-fixture-key", RemainQuota: 800, UsedQuota: 300}
			accountChannel := &model.Channel{Id: 5, UsedQuota: 300}
			businessTask := &model.Task{TaskID: "business-existing", UserId: 7, AppID: 3, CreatedAt: now - 8*86400, Status: model.TaskStatusInProgress, Quota: 100, ClientProtocol: "openai_video", PrivateData: model.TaskPrivateData{AsyncBilling: &model.TaskAsyncBillingContext{State: model.TaskBillingStatePending}}}
			attempt := &model.TaskCreateAttempt{AttemptID: "cleanup-inflight", UserID: 7, AppID: 3, Status: model.TaskCreateAttemptSending, BillingHoldState: model.TaskCreateAttemptBillingHeld, HeldQuota: 100}
			logs := []*model.Log{{UserId: 7, Type: model.LogTypeConsume, Quota: 60, PromptTokens: 10, CompletionTokens: 20, CreatedAt: now - 8*86400}, {UserId: 7, Type: model.LogTypeRefund, Quota: 10, CreatedAt: now - 8*86400}}
			for _, value := range []any{user, token, accountChannel, businessTask, attempt, &logs} {
				require.NoError(t, model.DB.Create(value).Error)
			}
			// Freeze every persisted business field, including billing snapshots.
			before := make(map[string][]map[string]any)
			for _, table := range []string{"users", "tokens", "channels", "tasks", "task_create_attempts", "logs"} {
				var rows []map[string]any
				require.NoError(t, model.DB.Table(table).Order("id").Find(&rows).Error)
				before[table] = rows
			}
			prepared := make(chan int64, 1)
			arrived := make(chan struct{})
			release := make(chan struct{})
			var exchanges atomic.Int32
			const reply = `{"task_id":"local-provider-fixture","status":"queued"}`
			upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				exchanges.Add(1)
				body, err := io.ReadAll(r.Body)
				require.NoError(t, err)
				assert.JSONEq(t, `{"prompt":"media"}`, string(body))
				if tc.afterSend {
					close(arrived)
					<-release
				}
				w.Header().Set("Content-Type", "application/json")
				_, _ = io.WriteString(w, reply)
			}))
			defer upstream.Close()
			defer func() {
				select {
				case <-release:
				default:
					close(release)
				}
			}()
			engine := gin.New()
			engine.Use(service.APIRequestEvidence())
			engine.POST("/api/v3/contents/generations/tasks", func(c *gin.Context) {
				c.Set("id", 7)
				c.Set("app_id", 3)
				c.Set("token_id", 3)
				common.SetContextKey(c, constant.ContextKeyTaskCreateAttemptID, int(attempt.ID))
				require.NoError(t, service.BeginTaskRequestEvidence(c, model.TaskRequestEvidenceKindVideoTask))
				indexes, count, err := model.QueryTaskRequestEvidence(model.TaskRequestEvidenceQueryParams{})
				require.NoError(t, err)
				require.EqualValues(t, 1, count)
				prepared <- indexes[0].Id
				if !tc.afterSend {
					<-release
				}
				request, err := http.NewRequest(http.MethodPost, upstream.URL, strings.NewReader(`{"prompt":"media"}`))
				require.NoError(t, err)
				request.Header.Set("Content-Type", "application/json")
				info := &relaycommon.RelayInfo{ChannelMeta: &relaycommon.ChannelMeta{ChannelType: constant.ChannelTypeOpenAI}}
				// Use the real send boundary, including capture and durable permit.
				response, err := channel.DoRequest(c, request, info)
				if err != nil {
					c.JSON(503, gin.H{"error": "send rejected"})
					return
				}
				service.AttachTaskRequestEvidenceUpstreamResponse(c, response)
				body, err := io.ReadAll(response.Body)
				require.NoError(t, err)
				require.NoError(t, response.Body.Close())
				assert.False(t, info.SkipRequestRefund)
				c.Data(response.StatusCode, response.Header.Get("Content-Type"), body)
			})
			client := httptest.NewRecorder()
			done := make(chan struct{})
			go func() {
				defer close(done)
				request := httptest.NewRequest(http.MethodPost, "/api/v3/contents/generations/tasks", strings.NewReader(`{"prompt":"media"}`))
				request.Header.Set("Content-Type", "application/json")
				engine.ServeHTTP(client, request)
			}()
			id := <-prepared
			if tc.afterSend {
				<-arrived
			}
			age := int64(1)
			if tc.scheduled {
				age = 8 * 86400
			}
			require.NoError(t, model.DB.Model(&model.TaskRequestEvidence{}).Where("id = ?", id).Updates(map[string]any{"created_at": now - age, "retain_until": 0}).Error)
			var cleanup *model.SystemTask
			var runner string
			if tc.scheduled {
				cleanup, runner = createCleanupTask(t)
			} else {
				var created bool
				var err error
				cleanup, created, err = service.StartEvidenceCleanupTask(common.GetTimestamp(), 7)
				require.NoError(t, err)
				require.True(t, created)
				runner = "business-cleanup"
				if !tc.accepted {
					_, claimed, err := model.ClaimSystemTask(cleanup.ID, cleanup.Type, runner, now+3600)
					require.NoError(t, err)
					require.True(t, claimed)
				}
			}
			if !tc.accepted {
				evidenceRetentionHandler{}.Run(context.Background(), cleanup, runner)
			}
			close(release)
			<-done
			assert.Equal(t, http.StatusOK, client.Code, client.Body.String())
			assert.JSONEq(t, reply, client.Body.String())
			assert.EqualValues(t, 1, exchanges.Load())
			if tc.accepted {
				_, claimed, err := model.ClaimSystemTask(cleanup.ID, cleanup.Type, runner, now+3600)
				require.NoError(t, err)
				require.True(t, claimed)
				evidenceRetentionHandler{}.Run(context.Background(), cleanup, runner)
			}
			finished, err := model.GetSystemTaskByTaskID(cleanup.TaskID)
			require.NoError(t, err)
			assert.Equal(t, model.SystemTaskStatusSucceeded, finished.Status)
			entries, err := os.ReadDir(dir)
			require.NoError(t, err)
			assert.Empty(t, entries)
			var events int64
			require.NoError(t, model.DB.Model(&model.TaskRequestEvidenceEvent{}).Count(&events).Error)
			assert.Zero(t, events)
			for table, expected := range before {
				var rows []map[string]any
				require.NoError(t, model.DB.Table(table).Order("id").Find(&rows).Error)
				assert.Equal(t, expected, rows, table+" must survive evidence cleanup unchanged")
			}
		})
	}
}
