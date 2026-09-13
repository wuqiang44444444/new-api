package main

import (
	"fmt"
	"path/filepath"
	"testing"

	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/pkg/billingexpr"
	"github.com/glebarez/sqlite"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

func TestRepairMissingInitialLogIsIdempotentAndDoesNotChangeFunds(t *testing.T) {
	db, s := setupTaskLogRepair(t)
	var task model.Task
	require.NoError(t, db.First(&task, s.CreateTaskID).Error)
	preview, err := repair(db, s, false)
	require.NoError(t, err)
	assert.Equal(t, 1, preview.Creates)
	assert.EqualValues(t, -60, preview.NetBefore)
	assert.EqualValues(t, 40, preview.NetAfter)
	assert.Equal(t, "mismatch", preview.Before.Status)
	assert.EqualValues(t, 100, preview.Before.NetDifference)
	assert.Equal(t, "matched", preview.After.Status)
	assert.Zero(t, preview.After.NetDifference)
	var count int64
	require.NoError(t, db.Model(&model.Log{}).Count(&count).Error)
	assert.EqualValues(t, 1, count)
	applied, err := repair(db, s, true)
	require.NoError(t, err)
	assert.Equal(t, preview, applied)
	again, err := repair(db, s, true)
	require.NoError(t, err)
	assert.Zero(t, again.Creates)
	assert.Zero(t, again.MetadataUpdates)
	var user model.User
	var token model.Token
	var after model.Task
	require.NoError(t, db.First(&user, 9).Error)
	require.NoError(t, db.First(&token, 4).Error)
	require.NoError(t, db.First(&after, task.ID).Error)
	assert.Equal(t, 9999, user.Quota)
	assert.Equal(t, 40, user.UsedQuota)
	assert.Equal(t, 8888, token.RemainQuota)
	assert.Equal(t, 40, token.UsedQuota)
	assert.Equal(t, task.Quota, after.Quota)
	assert.Equal(t, task.PrivateData, after.PrivateData)
}

func TestRepairRejectsAnonymousCreatesRegardlessOfTimestamp(t *testing.T) {
	for _, createdAt := range []int64{900, 1110, 1210} {
		t.Run(fmt.Sprint(createdAt), func(t *testing.T) {
			db, s := setupTaskLogRepair(t)
			initial := model.Log{UserId: 9, TokenId: 4, ChannelId: 8, ModelName: "video", Type: model.LogTypeConsume, Quota: 100, CreatedAt: createdAt, Other: `{"is_task":true,"group_ratio":1}`}
			require.NoError(t, db.Create(&initial).Error)
			var before []model.Log
			require.NoError(t, db.Order("id").Find(&before).Error)
			_, err := repair(db, s, true)
			require.ErrorContains(t, err, "unlinked initial log")
			var after []model.Log
			require.NoError(t, db.Order("id").Find(&after).Error)
			assert.Equal(t, before, after)
		})
	}
}

func TestRepairDetectsMissingInitialWithoutBeingToldTaskID(t *testing.T) {
	db, s := setupTaskLogRepair(t)
	s.CreateTaskID = 0
	preview, err := repair(db, s, false)
	require.NoError(t, err)
	assert.Equal(t, "mismatch", preview.After.Status)
	assert.EqualValues(t, 100, preview.After.NetDifference)
	assert.Equal(t, 1, preview.After.MissingInitialLogs)
	var before []model.Log
	require.NoError(t, db.Order("id").Find(&before).Error)
	_, err = repair(db, s, true)
	require.ErrorContains(t, err, "integrity is mismatch")
	var after []model.Log
	require.NoError(t, db.Order("id").Find(&after).Error)
	assert.Equal(t, before, after)
}

func TestRepairDoesNotClaimCrossPeriodLogsAreMissing(t *testing.T) {
	db, s := setupTaskLogRepair(t)
	require.NoError(t, db.Create(&model.Log{UserId: 9, TokenId: 4, ChannelId: 8, ModelName: "video", Type: model.LogTypeConsume, Quota: 100, CreatedAt: 900, Other: `{"is_task":true,"task_id":"public-task","group_ratio":1}`}).Error)
	_, err := repair(db, s, false)
	require.ErrorContains(t, err, "outside the selected period")
	s.CreateTaskID = 0
	preview, err := repair(db, s, false)
	require.NoError(t, err)
	assert.Equal(t, "incomplete", preview.After.Status)
}

func TestRepairReportsPendingSettlementAsIncomplete(t *testing.T) {
	db, s := setupTaskLogRepair(t)
	var task model.Task
	require.NoError(t, db.First(&task, s.CreateTaskID).Error)
	task.PrivateData.AsyncBilling.State = model.TaskBillingStatePending
	require.NoError(t, db.Save(&task).Error)
	s.CreateTaskID = 0
	preview, err := repair(db, s, false)
	require.NoError(t, err)
	assert.Equal(t, "incomplete", preview.After.Status)
	_, err = repair(db, s, true)
	require.ErrorContains(t, err, "integrity is incomplete")
}

func TestRepairDistinguishesAggregateMatchFromTaskOwnership(t *testing.T) {
	db, s := setupTaskLogRepair(t)
	require.NoError(t, db.Create(&model.Log{UserId: 9, TokenId: 4, ChannelId: 8, ModelName: "video", Type: model.LogTypeConsume, Quota: 100, CreatedAt: 1100, Other: `{"is_task":true,"group_ratio":1}`}).Error)
	s.CreateTaskID = 0
	preview, err := repair(db, s, false)
	require.NoError(t, err)
	assert.Equal(t, "aggregate_matched", preview.After.Status)
	assert.Equal(t, 1, preview.After.UnlinkedInitialLogs)
	assert.Zero(t, preview.After.NetDifference)
}

func TestRepairDetectsOpposingTaskErrorsEvenWhenTotalMatches(t *testing.T) {
	db, s := setupTaskLogRepair(t)
	require.NoError(t, db.Model(&model.Log{}).Where("type = ?", model.LogTypeRefund).Update("quota", 50).Error)
	var second model.Task
	require.NoError(t, db.First(&second, s.CreateTaskID).Error)
	second.ID, second.TaskID = 0, "second-task"
	require.NoError(t, db.Create(&second).Error)
	require.NoError(t, db.Create(&[]model.Log{
		{UserId: 9, TokenId: 4, ChannelId: 8, ModelName: "video", Type: model.LogTypeConsume, Quota: 100, CreatedAt: 1100, Other: `{"is_task":true,"task_id":"public-task","group_ratio":1}`},
		{UserId: 9, TokenId: 4, ChannelId: 8, ModelName: "video", Type: model.LogTypeConsume, Quota: 100, CreatedAt: 1100, Other: `{"is_task":true,"task_id":"second-task","group_ratio":1}`},
		{UserId: 9, TokenId: 4, ChannelId: 8, ModelName: "video", Type: model.LogTypeRefund, Quota: 70, CreatedAt: 1110, Other: `{"task_id":"second-task","group_ratio":1,"actual_quota":40,"pre_consumed_quota":100}`},
	}).Error)
	s.CreateTaskID = 0
	preview, err := repair(db, s, false)
	require.NoError(t, err)
	assert.Zero(t, preview.After.NetDifference)
	assert.Equal(t, "mismatch", preview.After.Status)
	assert.Equal(t, 2, preview.After.UnreconciledTasks)
}

func TestRepairRejectsUntransferredHoldAndRollsBackMetadata(t *testing.T) {
	db, err := gorm.Open(sqlite.Open(filepath.Join(t.TempDir(), "reject.db")), &gorm.Config{})
	require.NoError(t, err)
	require.NoError(t, db.AutoMigrate(&model.Task{}, &model.TaskCreateAttempt{}, &model.Log{}))
	quota := 40
	task := model.Task{TaskID: "public-task", UserId: 9, Quota: 40, SubmitTime: 1100, Properties: model.Properties{OriginModelName: "video"}, PrivateData: model.TaskPrivateData{TokenId: 4, AsyncBilling: &model.TaskAsyncBillingContext{State: model.TaskBillingStateSettled, TargetQuota: &quota, TieredSnapshot: &billingexpr.BillingSnapshot{ExprString: `tier("base", c)`, GroupRatio: 1}}}}
	require.NoError(t, db.Create(&task).Error)
	log := model.Log{UserId: 9, TokenId: 4, ModelName: "video", CreatedAt: 1110, Type: model.LogTypeRefund, Quota: 60, Other: `{"task_id":"public-task","group_ratio":1,"actual_quota":40}`}
	require.NoError(t, db.Create(&log).Error)
	_, err = repair(db, scope{UserID: 9, TokenID: 4, Model: "video", Start: 1000, End: 1200, CreateTaskID: task.ID}, true)
	require.ErrorContains(t, err, "creation attempt")
	var after model.Log
	require.NoError(t, db.First(&after, log.Id).Error)
	assert.Equal(t, log.Other, after.Other)
}

func setupTaskLogRepair(t *testing.T) (*gorm.DB, scope) {
	t.Helper()
	db, err := gorm.Open(sqlite.Open(filepath.Join(t.TempDir(), "repair.db")), &gorm.Config{})
	require.NoError(t, err)
	require.NoError(t, db.AutoMigrate(&model.Task{}, &model.TaskCreateAttempt{}, &model.User{}, &model.Token{}, &model.Log{}))
	require.NoError(t, db.Create(&model.User{Id: 9, Username: "customer", Quota: 9999, UsedQuota: 40}).Error)
	require.NoError(t, db.Create(&model.Token{Id: 4, UserId: 9, Name: "key", RemainQuota: 8888, UsedQuota: 40}).Error)
	quota := 40
	task := model.Task{TaskID: "public-task", UserId: 9, AppID: 1, ChannelId: 8, Quota: 40, SubmitTime: 1100, Status: model.TaskStatusSuccess, Properties: model.Properties{OriginModelName: "video"}, PrivateData: model.TaskPrivateData{TokenId: 4, AsyncBilling: &model.TaskAsyncBillingContext{State: model.TaskBillingStateSettled, TargetQuota: &quota, ActualTokens: 80, BillingProbe: &billingexpr.RequestInput{}, TieredSnapshot: &billingexpr.BillingSnapshot{ExprString: `tier("base", c)`, GroupRatio: 1, QuotaPerUnit: 500000}}}}
	require.NoError(t, db.Create(&task).Error)
	attempt := model.TaskCreateAttempt{AttemptID: "attempt", PublicTaskID: task.TaskID, UserID: 9, TokenID: 4, AppID: 1, ChannelID: 8, PublicModel: "video", HeldQuota: 100, Status: model.TaskCreateAttemptComplete, BillingHoldState: model.TaskCreateAttemptBillingTransferred}
	require.NoError(t, db.Create(&attempt).Error)
	require.NoError(t, db.Create(&model.Log{UserId: 9, TokenId: 4, ModelName: "video", ChannelId: 8, CreatedAt: 1110, Type: model.LogTypeRefund, Quota: 60, Other: `{"task_id":"public-task","model_price":0,"group_ratio":1,"pre_consumed_quota":100,"actual_quota":40}`}).Error)
	s := scope{UserID: 9, TokenID: 4, Model: "video", Start: 1000, End: 1200, CreateTaskID: task.ID}
	return db, s
}

func TestRepairNewUsageExpressionUsesActualFrozenInput(t *testing.T) {
	db, s := setupTaskLogRepair(t)
	var task model.Task
	require.NoError(t, db.First(&task, s.CreateTaskID).Error)
	async := task.PrivateData.AsyncBilling
	async.TieredSnapshot.ExprString = `tier("base", u("tokens") / 1000000)`
	async.TieredSnapshot.ExprHash = billingexpr.ExprHashString(async.TieredSnapshot.ExprString)
	async.TieredSnapshot.TaskUsageBilling = true
	async.TieredSnapshot.UsageUnits = map[string]string{"tokens": "token"}
	async.TieredSnapshot.UsageFacts = map[string]any{"tokens": float64(999999)}
	async.ActualUsageReported = true
	task.PrivateData.VideoUpstreamProtocol = "modelark_v3_volcengine"
	require.NoError(t, db.Save(&task).Error)
	first, err := repair(db, s, true)
	require.NoError(t, err)
	assert.Equal(t, 1, first.Creates)
	again, err := repair(db, s, true)
	require.NoError(t, err)
	assert.Zero(t, again.Creates)
	assert.Zero(t, again.MetadataUpdates)
	var after model.Task
	require.NoError(t, db.First(&after, task.ID).Error)
	assert.Equal(t, task.PrivateData, after.PrivateData)
	assert.Equal(t, 40, after.Quota)
}

func TestRepairMissingZeroDeltaCompletionRequiresExplicitTask(t *testing.T) {
	db, s := setupTaskLogRepair(t)
	var task model.Task
	require.NoError(t, db.First(&task, s.CreateTaskID).Error)
	task.FinishTime = 1110
	require.NoError(t, db.Save(&task).Error)
	require.NoError(t, db.Where("1 = 1").Delete(&model.Log{}).Error)
	require.NoError(t, db.Create(&model.Log{UserId: 9, TokenId: 4, ChannelId: 8, ModelName: "video", Type: model.LogTypeConsume, Quota: 40, CreatedAt: 1100, Other: `{"task_id":"public-task","task_billing_event":"create","group_ratio":1}`}).Error)
	s.CreateTaskID = 0
	preview, err := repair(db, s, false)
	require.NoError(t, err)
	assert.Zero(t, preview.After.NetDifference)
	assert.Equal(t, "mismatch", preview.After.Status)
	assert.Equal(t, 1, preview.After.MissingFinalLogs)
	s.FinalTaskID = task.ID
	fixed, err := repair(db, s, true)
	require.NoError(t, err)
	assert.Equal(t, 1, fixed.FinalLogs)
	assert.Equal(t, "matched", fixed.After.Status)
	again, err := repair(db, s, true)
	require.NoError(t, err)
	assert.Zero(t, again.FinalLogs)
	var logs []model.Log
	require.NoError(t, db.Order("id").Find(&logs).Error)
	require.Len(t, logs, 2)
	assert.Zero(t, logs[1].Quota)
	assert.Equal(t, 80, logs[1].CompletionTokens)
	var user model.User
	require.NoError(t, db.First(&user, 9).Error)
	assert.Equal(t, 9999, user.Quota)
	assert.Equal(t, 40, user.UsedQuota)
}
