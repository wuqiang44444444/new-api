package controller

import (
	"net/http"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/dto"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/plugins"
	"github.com/QuantumNous/new-api/setting/config"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// Exercise the real plugin, HTTP polling and durable funding path. A query
// failure cannot refund; the later matching business failure refunds once.
func TestSynlinkFailedTaskRecoversFromReconciliationAndRefundsOnce(t *testing.T) {
	for _, tc := range []struct{ name, observation, code string }{
		{"string error with metadata", `{"task":{"id":"prov-task-1","status":"failed","error":"Synlink: The service encountered an unexpected internal error for customer-synlink. api_key=fixture-sensitive-value","outputs":[],"metadata":{"id":"internal-task","status":"failed","error":{"code":"InternalServiceError","message":"private internal diagnostic"},"model":"private-model"},"usage":{"total_tokens":87300}}}`, "upstream_task_failed"},
		{"object error without metadata or usage", `{"task":{"id":"prov-task-1","status":"failed","error":{"code":"InternalServiceError","message":"Synlink: The service encountered an unexpected internal error for customer-synlink. api_key=fixture-sensitive-value","details":"private internal diagnostic"},"outputs":[]}}`, "InternalServiceError"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			fx := newSeedanceFundsFixture(t)
			var channel model.Channel
			require.NoError(t, fx.db.First(&channel, fx.channelID).Error)
			mapping, err := common.Marshal(map[string]string{"customer-synlink": "doubao-seedance-2-0-260128"})
			require.NoError(t, err)
			channel.Models = "customer-synlink"
			channel.ModelMapping = common.GetPointer(string(mapping))
			channel.SetOtherSettings(dto.ChannelOtherSettings{VideoUpstreamProtocol: dto.VideoUpstreamProtocolSynlinkVideoV1, AssetUpstreamProtocol: dto.AssetUpstreamProtocolNone})
			require.NoError(t, fx.db.Save(&channel).Error)
			require.NoError(t, config.GlobalConfig.LoadFromDB(map[string]string{
				"billing_setting.billing_mode": `{"customer-synlink":"tiered_expr"}`,
				"billing_setting.billing_expr": `{"customer-synlink":"tier(\"fixed\", 1)"}`,
			}))
			fx.createBodyRes.Store(`{"task":{"id":"prov-task-1","status":"pending"}}`)
			fx.assertAttemptAtPost.Store(true)
			publicID := decodeSeedanceFundsCreateID(t, fx.submitCreateBody(`{"model":"customer-synlink","content":[{"type":"text","text":"A landscape"}],"duration":4,"resolution":"720p","ratio":"16:9"}`))
			task := fx.loadTask(publicID)
			require.NotNil(t, task.PrivateData.Execution)
			require.NotNil(t, task.PrivateData.Execution.TaskPlugin)
			assert.Equal(t, plugins.SeedanceVersion(), task.PrivateData.Execution.TaskPlugin.Version)
			fx.queryBodyRes.Store(`{"task":{"id":"prov-task-1","status":"processing"}}`)
			fx.pollOnce(publicID)
			task = fx.loadTask(publicID)
			require.Equal(t, model.TaskStatus(model.TaskStatusInProgress), task.Status)
			require.NotZero(t, task.StartTime)
			startedAt := task.StartTime
			assert.Equal(t, "50%", task.Progress)

			for _, observation := range []struct {
				status       int
				body, reason string
			}{
				{http.StatusNotFound, `{"error":{"code":"task_not_found","message":"private query detail"}}`, "Upstream task query returned HTTP 404; the result is unconfirmed"},
				{http.StatusNotFound, `<html>Proxy route missing</html>`, "Upstream task query returned HTTP 404; the result is unconfirmed"},
				{http.StatusForbidden, `{"error":"private credential detail"}`, "unverified"},
				{http.StatusOK, `{"error":"Query unavailable","task":{"id":"prov-task-1","status":"failed","error":"Generation failed"}}`, ""},
				{http.StatusOK, `{"task":{"id":"other-task","status":"failed","error":"Generation failed"}}`, ""},
				{http.StatusOK, `{"task":{"id":"prov-task-1","status":"failed","error":"Generation failed","outputs":["https://result.example/video"]}}`, ""},
			} {
				fx.queryStatus.Store(int32(observation.status))
				fx.queryBodyRes.Store(observation.body)
				fx.pollOnce(publicID)
				task = fx.loadTask(publicID)
				require.Equal(t, model.TaskStatusReconciliationRequired, task.Status)
				assert.Contains(t, task.FailReason, observation.reason)
				assert.NotContains(t, task.FailReason, "private query detail")
				assert.Equal(t, model.TaskBillingStatePending, task.BillingState)
				assert.Equal(t, seedanceFundsHold, task.Quota)
				assert.Equal(t, seedanceFundsInitialQuota-seedanceFundsHold, fx.userQuota())
				remain, used := fx.tokenQuota()
				assert.Equal(t, seedanceFundsInitialQuota-seedanceFundsHold, remain)
				assert.Equal(t, seedanceFundsHold, used)
				assert.Empty(t, fx.logsByType()[model.LogTypeRefund])
			}

			fx.queryStatus.Store(http.StatusOK)
			fx.queryBodyRes.Store(tc.observation)
			fx.pollOnce(publicID)
			task = fx.loadTask(publicID)
			require.Equal(t, model.TaskStatus(model.TaskStatusFailure), task.Status)
			assert.Equal(t, startedAt, task.StartTime)
			assert.NotZero(t, task.FinishTime)
			assert.Equal(t, "100%", task.Progress)
			assert.Contains(t, task.FailReason, "The service encountered an unexpected internal error")
			assert.NotContains(t, task.FailReason, "fixture-sensitive-value")
			assert.Contains(t, task.FailReason, "video service:")
			assert.Contains(t, task.FailReason, "customer-synlink")
			assert.NotContains(t, task.FailReason, "Synlink:")
			assert.Contains(t, string(task.Data), "customer-synlink")
			assert.NotContains(t, string(task.Data), "Synlink:")
			assert.NotContains(t, task.FailReason, "upstream_contract_violation")
			assert.NotContains(t, string(task.Data), "private internal diagnostic")
			assert.NotContains(t, string(task.Data), "fixture-sensitive-value")
			assert.Equal(t, tc.code, task.PublicVideoFailure().Code)
			assert.False(t, task.PrivateData.AsyncBilling.ActualUsageReported)
			assert.Empty(t, task.PrivateData.ResultURL)
			assert.Equal(t, model.TaskBillingStateSettled, task.BillingState)
			assert.Equal(t, model.TaskBillingStateSettled, task.PrivateData.AsyncBilling.State)
			require.NotNil(t, task.PrivateData.AsyncBilling.TargetQuota)
			assert.Zero(t, *task.PrivateData.AsyncBilling.TargetQuota)
			assert.Zero(t, task.Quota)
			assert.Equal(t, seedanceFundsInitialQuota, fx.userQuota())
			remain, used := fx.tokenQuota()
			assert.Equal(t, seedanceFundsInitialQuota, remain)
			assert.Zero(t, used)
			assert.Empty(t, model.GetAllUnFinishSyncTasks(10), "failed task must leave background polling")

			fx.pollOnce(publicID)
			assert.Equal(t, seedanceFundsInitialQuota, fx.userQuota())
			remain, used = fx.tokenQuota()
			assert.Equal(t, seedanceFundsInitialQuota, remain)
			assert.Zero(t, used)
			logs := fx.logsByType()
			require.Len(t, logs[model.LogTypeRefund], 1)
			assert.Equal(t, seedanceFundsHold, logs[model.LogTypeRefund][0].Quota)
			assert.NotContains(t, logs[model.LogTypeRefund][0].Other, "fixture-sensitive-value")
			assert.NotContains(t, logs[model.LogTypeRefund][0].Other, "Synlink:")
			assert.EqualValues(t, 1, fx.createCalls.Load(), "poll recovery must never resend creation")
		})
	}
}
