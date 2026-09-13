package main

import (
	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/pkg/billingexpr"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"testing"
)

func TestRepairLinksInitialLogsOnlyByFrozenProviderRequestIdentity(t *testing.T) {
	for _, ambiguous := range []bool{false, true} {
		t.Run(map[bool]string{false: "exact identity", true: "duplicate identity"}[ambiguous], func(t *testing.T) {
			db, s := setupTaskLogRepair(t)
			var task model.Task
			require.NoError(t, db.First(&task, s.CreateTaskID).Error)
			task.PrivateData.UpstreamRequestID = "provider-request"
			require.NoError(t, db.Save(&task).Error)
			require.NoError(t, db.Model(&model.TaskCreateAttempt{}).Where("public_task_id = ?", task.TaskID).Update("upstream_request_id", "provider-request").Error)
			initial := model.Log{UserId: 9, TokenId: 4, ChannelId: 8, ModelName: "video", Type: model.LogTypeConsume, CreatedAt: 1100, Quota: 100, UpstreamRequestId: "provider-request", Other: `{"is_task":true,"group_ratio":1}`}
			require.NoError(t, db.Create(&initial).Error)
			if ambiguous {
				duplicate := task
				duplicate.ID = 0
				duplicate.TaskID = "second-task"
				require.NoError(t, db.Create(&duplicate).Error)
			}
			s.LinkInitialLogs = true
			preview, err := repair(db, s, false)
			if ambiguous {
				require.ErrorContains(t, err, "ambiguous")
				return
			}
			require.NoError(t, err)
			assert.Equal(t, 1, preview.LinkedInitialLogs)
			assert.Zero(t, preview.Creates)
			assert.Equal(t, "matched", preview.After.Status)
			applied, err := repair(db, s, true)
			require.NoError(t, err)
			assert.Equal(t, preview, applied)
			var after model.Log
			require.NoError(t, db.First(&after, initial.Id).Error)
			var other map[string]any
			require.NoError(t, common.UnmarshalJsonStr(after.Other, &other))
			assert.Equal(t, task.TaskID, other["task_id"])
			assert.Equal(t, initial.Quota, after.Quota)
			again, err := repair(db, s, true)
			require.NoError(t, err)
			assert.Zero(t, again.LinkedInitialLogs)
			assert.Zero(t, again.Creates)
		})
	}
}

func TestRepairMissingInitialSupportsFrozenParameterExpressionWithoutInventingTokens(t *testing.T) {
	db, s := setupTaskLogRepair(t)
	var task model.Task
	require.NoError(t, db.First(&task, s.CreateTaskID).Error)
	task.PrivateData.AsyncBilling.TieredSnapshot.ExprString = `tier("base", param("_task.duration_seconds") * 80)`
	task.PrivateData.AsyncBilling.TieredSnapshot.ExprHash = billingexpr.ExprHashString(task.PrivateData.AsyncBilling.TieredSnapshot.ExprString)
	task.PrivateData.AsyncBilling.BillingProbe.Body = []byte(`{"_task":{"duration_seconds":1}}`)
	task.PrivateData.AsyncBilling.ActualTokens = 0
	require.NoError(t, db.Save(&task).Error)
	result, err := repair(db, s, true)
	require.NoError(t, err)
	assert.Equal(t, "matched", result.After.Status)
	var initial model.Log
	require.NoError(t, db.Where("type = ?", model.LogTypeConsume).First(&initial).Error)
	var other map[string]any
	require.NoError(t, common.UnmarshalJsonStr(initial.Other, &other))
	snapshot := other["admin_info"].(map[string]any)["statement_snapshot"].(map[string]any)
	assert.Equal(t, "unknown", snapshot["billing_mode"])
	assert.Zero(t, initial.CompletionTokens)
}

func TestRepairInitialIdentityConflictsRollBackAllLogChanges(t *testing.T) {
	for _, conflict := range []string{"request identity", "hold amount", "malformed task identity", "durable delivery"} {
		t.Run(conflict, func(t *testing.T) {
			db, s := setupTaskLogRepair(t)
			var task model.Task
			require.NoError(t, db.First(&task, s.CreateTaskID).Error)
			task.PrivateData.UpstreamRequestID = "provider-request"
			require.NoError(t, db.Save(&task).Error)
			require.NoError(t, db.Model(&model.TaskCreateAttempt{}).Where("public_task_id = ?", task.TaskID).Update("upstream_request_id", "provider-request").Error)
			initial := model.Log{UserId: 9, TokenId: 4, ChannelId: 8, ModelName: "video", Type: model.LogTypeConsume, CreatedAt: 1100, Quota: 100, UpstreamRequestId: "provider-request", Other: `{"is_task":true,"group_ratio":1}`}
			switch conflict {
			case "request identity":
				require.NoError(t, db.Model(&model.TaskCreateAttempt{}).Where("public_task_id = ?", task.TaskID).Update("upstream_request_id", "different-request").Error)
			case "hold amount":
				initial.Quota = 90
			case "malformed task identity":
				initial.Other = `{"is_task":true,"task_id":{},"group_ratio":1}`
			case "durable delivery":
				require.NoError(t, db.AutoMigrate(&model.TaskBillingDelivery{}))
				require.NoError(t, db.Create(&model.TaskBillingDelivery{TaskRowID: task.ID}).Error)
			}
			require.NoError(t, db.Create(&initial).Error)
			var before []model.Log
			require.NoError(t, db.Order("id").Find(&before).Error)
			s.LinkInitialLogs = true
			_, err := repair(db, s, true)
			require.Error(t, err)
			var after []model.Log
			require.NoError(t, db.Order("id").Find(&after).Error)
			assert.Equal(t, before, after)
		})
	}
}

func TestRepairPreservesFailureDiagnosticWithoutDuplicatingSettledUsage(t *testing.T) {
	db, s := setupTaskLogRepair(t)
	diagnostic := model.Log{UserId: 9, TokenId: 4, ChannelId: 8, ModelName: "video", Type: model.LogTypeConsume, CreatedAt: 1105, Other: `{"task_id":"public-task","group_ratio":1,"admin_info":{"task_billing_state":"failed","task_billing_error":"fixture failure"}}`}
	require.NoError(t, db.Create(&diagnostic).Error)
	result, err := repair(db, s, true)
	require.NoError(t, err)
	assert.Equal(t, "matched", result.After.Status)
	var after model.Log
	require.NoError(t, db.First(&after, diagnostic.Id).Error)
	assert.Equal(t, diagnostic, after)
	var logs []model.Log
	require.NoError(t, db.Find(&logs).Error)
	tokens := 0
	for _, log := range logs {
		tokens += log.CompletionTokens
	}
	assert.Equal(t, 80, tokens)
}
