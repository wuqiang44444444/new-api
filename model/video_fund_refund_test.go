package model

import (
	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"testing"
)

func TestVideoFundRefundSettledAndDebtCannotRecharge(t *testing.T) {
	for _, state := range []TaskBillingState{TaskBillingStateSettled, TaskBillingStateDebt, TaskBillingStateAwaitingUsage} {
		t.Run(string(state), func(t *testing.T) {
			truncateTables(t)
			user := createFundsTestUser(t, 1710, 75)
			token := createFundsTestToken(t, user.Id, "refund-task-"+string(state), 75)
			require.NoError(t, DB.Model(token).Update("used_quota", 25).Error)
			target := 60
			task := Task{TaskID: GenerateTaskID(), UserId: user.Id, AppID: 1, Platform: "video", ClientProtocol: TaskClientProtocolModelArkV3, Quota: 25, Status: TaskStatusSuccess, BillingState: state, PrivateData: TaskPrivateData{BillingSource: "wallet", TokenId: token.Id, AsyncBilling: &TaskAsyncBillingContext{State: state, TargetQuota: &target}}}
			require.NoError(t, DB.Create(&task).Error)
			stale := task
			preview, err := GetVideoFundLog("task", task.ID)
			require.NoError(t, err)
			require.NoError(t, RequestVideoRefund("task", task.ID, preview.Version, 1, "customer reported missing result"))
			applied, _, err := ApplyTaskBillingTarget(&stale, 60)
			require.NoError(t, err)
			assert.False(t, applied)
			require.NoError(t, CompleteVideoTaskRefund(task.ID))
			require.NoError(t, CompleteVideoTaskRefund(task.ID))
			require.NoError(t, stale.Update())
			applied, _, err = ApplyTaskBillingTarget(&stale, 60)
			require.NoError(t, err)
			assert.False(t, applied)
			require.NoError(t, DB.First(user, user.Id).Error)
			assert.Equal(t, 100, user.Quota)
			require.NoError(t, DB.First(token, token.Id).Error)
			assert.Equal(t, 100, token.RemainQuota)
			require.NoError(t, DB.First(&task, task.ID).Error)
			assert.Zero(t, task.Quota)
			assert.Equal(t, TaskStatus(TaskStatusSuccess), task.Status)
			assert.Equal(t, "refunded", task.VideoRefundState)
			if state == TaskBillingStateSettled {
				assert.Zero(t, task.VideoRefundWaivedQuota)
			} else {
				assert.Equal(t, 35, task.VideoRefundWaivedQuota)
			}
			var count int64
			require.NoError(t, DB.Model(&TaskBillingDelivery{}).Where("task_row_id = ? AND event = ?", task.ID, "customer_refund").Count(&count).Error)
			assert.Equal(t, int64(1), count)
		})
	}
}

func TestVideoFundLogScopesAndDeduplicatesTransfer(t *testing.T) {
	truncateTables(t)
	user := createFundsTestUser(t, 1711, 100)
	token := createFundsTestToken(t, user.Id, "fund-log", 100)
	a := createHeldTestAttempt(t, user.Id, token.Id, "projection", 25, "wallet")
	task := Task{TaskID: a.PublicTaskID, UserId: user.Id, ClientProtocol: TaskClientProtocolModelArkV3, Quota: 25, Platform: "video"}
	require.NoError(t, DB.Create(&task).Error)
	require.NoError(t, DB.Model(a).Update("billing_hold_state", TaskCreateAttemptBillingTransferred).Error)
	batch := Task{TaskID: "batch-test", Platform: constant.TaskPlatformAzureBatch, Action: "generate", Quota: 100}
	image := Task{TaskID: "image-test", ClientProtocol: TaskClientProtocolImageOpenAIV1, Action: "generate", Quota: 100}
	require.NoError(t, DB.Create(&batch).Error)
	require.NoError(t, DB.Create(&image).Error)
	items, total, err := ListVideoFundLogs(VideoFundFilters{Limit: 20})
	require.NoError(t, err)
	assert.Equal(t, int64(1), total)
	require.Len(t, items, 1)
	assert.Equal(t, "task", items[0].Kind)
}

func TestVideoDeadlineMigrationNullBatchExclusionAndRetryFairness(t *testing.T) {
	truncateTables(t)
	oldMaster := common.IsMasterNode
	common.IsMasterNode = true
	t.Cleanup(func() { common.IsMasterNode = oldMaster })
	user := createFundsTestUser(t, 1712, 100)
	token := createFundsTestToken(t, user.Id, "fund-scan", 100)
	a := createHeldTestAttempt(t, user.Id, token.Id, "null", 10, "wallet")
	require.NoError(t, DB.Model(a).Updates(map[string]any{"created_at": common.GetTimestamp() - 90000, "funds_deadline_at": nil}).Error)
	batch := TaskCreateAttempt{AttemptID: "batch-fund", PublicTaskID: "batch-fund", ClientProtocol: "azurebatch", BillingHoldState: TaskCreateAttemptBillingHeld, Status: TaskCreateAttemptUnknown, FundsDeadlineAt: 1, HeldQuota: 20, UserID: user.Id, BillingSource: "wallet"}
	require.NoError(t, DB.Create(&batch).Error)
	require.NoError(t, InitTaskCreateAttemptFundsDeadline())
	rows, scanErr := GetTaskCreateAttemptFundDebts(common.GetTimestamp(), 100)
	require.NoError(t, scanErr)
	require.Len(t, rows, 1)
	assert.Equal(t, a.ID, rows[0].ID)
	require.NoError(t, MarkTaskCreateAttemptRefundRetry(a.ID, common.GetTimestamp(), "test"))
	rows, scanErr = GetTaskCreateAttemptFundDebts(common.GetTimestamp(), 100)
	require.NoError(t, scanErr)
	assert.Empty(t, rows)
	_, err := ReleaseTaskCreateAttemptHoldWarranty(batch.ID, TaskCreateAttemptReleaseWarrantyDeadline, 0)
	require.Error(t, err)
}

func TestVideoRefundRejectsStalePreviewAndSurvivesFailedFunds(t *testing.T) {
	truncateTables(t)
	user := createFundsTestUser(t, 1713, 100)
	token := createFundsTestToken(t, user.Id, "fund-stale", 100)
	a := createHeldTestAttempt(t, user.Id, token.Id, "stale", 25, "wallet")
	preview, err := GetVideoFundLog("attempt", a.ID)
	require.NoError(t, err)
	require.NoError(t, DB.Model(a).Update("status", TaskCreateAttemptUnknown).Error)
	require.ErrorIs(t, RequestVideoRefund("attempt", a.ID, preview.Version, 1, "test"), ErrVideoRefundConflict)
	preview, err = GetVideoFundLog("attempt", a.ID)
	require.NoError(t, err)
	require.NoError(t, RequestVideoRefund("attempt", a.ID, preview.Version, 1, "test"))
	// A missing wallet prevents completion but never loses the accepted instruction.
	require.NoError(t, DB.Delete(user).Error)
	_, err = ReleaseTaskCreateAttemptHoldWarranty(a.ID, TaskCreateAttemptReleaseWarrantyManual, 0)
	require.Error(t, err)
	require.NoError(t, DB.First(a, a.ID).Error)
	assert.Equal(t, "pending", a.VideoRefundState)
	assert.Equal(t, TaskCreateAttemptBillingHeld, a.BillingHoldState)
}

func TestVideoRefundDoesNotConvertUnsupportedFundingToWallet(t *testing.T) {
	truncateTables(t)
	user := createFundsTestUser(t, 1714, 100)
	a := TaskCreateAttempt{AttemptID: "unsupported", PublicTaskID: "unsupported", ClientProtocol: TaskClientProtocolModelArkV3, UserID: user.Id, BillingSource: "subscription", HeldQuota: 25, Status: TaskCreateAttemptUnknown, BillingHoldState: TaskCreateAttemptBillingHeld, FundsDeadlineAt: 1}
	require.NoError(t, DB.Create(&a).Error)
	_, err := ReleaseTaskCreateAttemptHoldWarranty(a.ID, TaskCreateAttemptReleaseWarrantyDeadline, 0)
	require.Error(t, err)
	require.NoError(t, DB.First(user, user.Id).Error)
	assert.Equal(t, 100, user.Quota)
}

func TestVideoOrdinaryTransferAfterDeadlineRefundsInstead(t *testing.T) {
	truncateTables(t)
	user := createFundsTestUser(t, 1715, 100)
	token := createFundsTestToken(t, user.Id, "fund-transfer-deadline", 100)
	a := createHeldTestAttempt(t, user.Id, token.Id, "late-transfer", 25, "wallet")
	require.NoError(t, DB.Model(a).Update("status", TaskCreateAttemptUpstreamSucceeded).Error)
	expireTestAttemptDeadline(t, a.ID)
	task := Task{TaskID: a.PublicTaskID, UserId: user.Id, Quota: 25, ClientProtocol: TaskClientProtocolModelArkV3}
	require.ErrorIs(t, InsertTaskWithCreateAttempt(&task, 0, a.ID), ErrTaskCreateAttemptMovedToWarrantyRefund)
	require.NoError(t, DB.First(user, user.Id).Error)
	assert.Equal(t, 100, user.Quota)
	var count int64
	require.NoError(t, DB.Model(&Task{}).Where("task_id = ?", task.TaskID).Count(&count).Error)
	assert.Zero(t, count)
}

func TestNativeVideoRefundWaitsForCreationFundingAndStopsLaterAdjustments(t *testing.T) {
	truncateTables(t)
	user := createFundsTestUser(t, 1716, 75)
	task := Task{TaskID: GenerateTaskID(), Platform: "video", Action: constant.TaskActionTextToVideo, UserId: user.Id, Quota: 25, Status: TaskStatusInProgress, PrivateData: TaskPrivateData{BillingSource: "wallet"}}
	require.NoError(t, DB.Create(&task).Error)
	entry, err := GetVideoFundLog("task", task.ID)
	require.NoError(t, err)
	assert.False(t, entry.CanRefund)
	require.ErrorIs(t, RequestVideoRefund("task", task.ID, entry.Version, 1, "review"), ErrTaskCreateAttemptFundBlocked)
	require.NoError(t, MarkVideoTaskFundingReady(task.ID))
	entry, err = GetVideoFundLog("task", task.ID)
	require.NoError(t, err)
	assert.True(t, entry.CanRefund)
	require.NoError(t, RequestVideoRefund("task", task.ID, entry.Version, 1, "review"))
	require.NoError(t, CompleteVideoTaskRefund(task.ID))
	applied, _, err := ApplyTaskBillingTarget(&task, 50)
	require.NoError(t, err)
	assert.False(t, applied)
	require.NoError(t, DB.First(user, user.Id).Error)
	assert.Equal(t, 100, user.Quota)
}

func TestVideoFundSummaryUsesAllRowsAndRefundEventDates(t *testing.T) {
	truncateTables(t)
	user := createFundsTestUser(t, 1751, 100)
	token := createFundsTestToken(t, user.Id, "fund-summary", 100)
	a := createHeldTestAttempt(t, user.Id, token.Id, "summary-held", 25, "wallet")
	now := common.GetTimestamp()
	require.NoError(t, DB.Model(a).Updates(map[string]any{"funds_deadline_at": now - 1, "status": TaskCreateAttemptUnknown, "app_id": 11}).Error)
	task := Task{TaskID: "summary-refunded", UserId: user.Id, AppID: 12, Platform: "video", ClientProtocol: TaskClientProtocolModelArkV3, Status: TaskStatusFailure, Quota: 0, CreatedAt: now - 10000}
	require.NoError(t, DB.Create(&task).Error)
	require.NoError(t, DB.Create(&TaskBillingDelivery{TaskRowID: task.ID, Event: "refund", BeforeQuota: 15, AfterQuota: 0, CreatedAt: now}).Error)
	rows, total, err := ListVideoFundLogs(VideoFundFilters{UserID: user.Id, Limit: 1})
	require.NoError(t, err)
	require.Len(t, rows, 1)
	assert.Equal(t, int64(2), total)
	summary, err := SummarizeVideoFunds(VideoFundFilters{UserID: user.Id, Limit: 1, RefundFrom: now - 10})
	require.NoError(t, err)
	assert.Equal(t, int64(1), summary.HeldCount)
	assert.Equal(t, int64(25), summary.HeldQuota)
	assert.Equal(t, int64(1), summary.OverdueCount)
	assert.Equal(t, int64(15), summary.ReturnedQuota)
	refunded, total, err := ListVideoFundLogs(VideoFundFilters{UserID: user.Id, State: "refunded", Limit: 20})
	require.NoError(t, err)
	require.Len(t, refunded, 1)
	assert.Equal(t, int64(1), total)
	assert.Equal(t, "refunded", refunded[0].FundState)
	summary, err = SummarizeVideoFunds(VideoFundFilters{UserID: user.Id, RefundFrom: now + 1})
	require.NoError(t, err)
	assert.Zero(t, summary.ReturnedQuota)
	progress, total, err := ListVideoFundProgress(user.Id, 11, "", 0, 20)
	require.NoError(t, err)
	require.Len(t, progress, 1)
	assert.Equal(t, int64(1), total)
	assert.Equal(t, a.PublicTaskID, progress[0].TaskID)
	payload, err := common.Marshal(progress)
	require.NoError(t, err)
	assert.Contains(t, string(payload), a.PublicTaskID)
	assert.NotContains(t, string(payload), "channel_id")
	assert.NotContains(t, string(payload), "version")
	assert.NotContains(t, string(payload), "note")
	progress, total, err = ListVideoFundProgress(user.Id+1, 11, "", 0, 20)
	require.NoError(t, err)
	assert.Empty(t, progress)
	assert.Zero(t, total)
}

func projectionFundState(t *testing.T, kind string, id int64) string {
	t.Helper()
	sql, args := videoFundProjection()
	var row struct {
		FundState string
	}
	require.NoError(t, DB.Table("(?) AS v", DB.Raw(sql, args...)).Where("kind = ? AND id = ?", kind, id).Scan(&row).Error)
	return row.FundState
}

// The SQL projection and the Go derivation must agree for every reachable
// state combination; this locks the documented equivalence against drift.
func TestVideoFundLogProjectionMatchesEntryDerivation(t *testing.T) {
	truncateTables(t)
	user := createFundsTestUser(t, 1720, 100)
	token := createFundsTestToken(t, user.Id, "projection-drift", 100)

	held := createHeldTestAttempt(t, user.Id, token.Id, "drift-held", 10, "wallet")
	require.NoError(t, MarkTaskCreateAttemptUnknown(held.ID, "req"))
	released := createHeldTestAttempt(t, user.Id, token.Id, "drift-released", 10, "wallet")
	_, err := ReleaseTaskCreateAttemptHold(released.ID, TaskCreateAttemptRejected)
	require.NoError(t, err)
	pendingAttempt := createHeldTestAttempt(t, user.Id, token.Id, "drift-pending", 10, "wallet")
	require.NoError(t, MarkTaskCreateAttemptUnknown(pendingAttempt.ID, "req"))
	require.NoError(t, DB.First(pendingAttempt, pendingAttempt.ID).Error)
	require.NoError(t, RequestVideoRefund("attempt", pendingAttempt.ID, videoAttemptRefundVersion(pendingAttempt), 9, "drift pending"))

	newTask := func(suffix string, quota int, status TaskStatus, billingState TaskBillingState) *Task {
		t.Helper()
		task := Task{TaskID: GenerateTaskID(), UserId: user.Id, Platform: "video", ClientProtocol: TaskClientProtocolModelArkV3, Quota: quota, Status: status, BillingState: billingState}
		require.NoError(t, DB.Create(&task).Error)
		return &task
	}
	successCharged := newTask("drift-charged", 20, TaskStatusSuccess, "")
	failureHeld := newTask("drift-failure", 20, TaskStatusFailure, "")
	refundedTask := newTask("drift-refunded", 0, TaskStatusFailure, "")
	require.NoError(t, DB.Create(&TaskBillingDelivery{TaskRowID: refundedTask.ID, Event: "video_task_refund", BeforeQuota: 20, AfterQuota: 0, CreatedAt: common.GetTimestamp()}).Error)
	closed := newTask("drift-closed", 0, TaskStatusFailure, "")
	settled := newTask("drift-settled", 20, TaskStatusSuccess, TaskBillingStateSettled)
	pendingTask := newTask("drift-pending-task", 20, TaskStatusSuccess, "")
	require.NoError(t, DB.First(pendingTask, pendingTask.ID).Error)
	require.NoError(t, RequestVideoRefund("task", pendingTask.ID, videoTaskRefundVersion(pendingTask), 9, "drift pending task"))

	cases := []struct {
		kind string
		id   int64
	}{
		{"attempt", held.ID}, {"attempt", released.ID}, {"attempt", pendingAttempt.ID},
		{"task", successCharged.ID}, {"task", failureHeld.ID}, {"task", refundedTask.ID},
		{"task", closed.ID}, {"task", settled.ID}, {"task", pendingTask.ID},
	}
	for _, tc := range cases {
		entry, err := GetVideoFundLog(tc.kind, tc.id)
		require.NoError(t, err)
		assert.Equal(t, projectionFundState(t, tc.kind, tc.id), entry.FundState,
			"kind=%s id=%d", tc.kind, tc.id)
	}
}

func TestVideoAttemptManualRefundCarriesIntentOperator(t *testing.T) {
	truncateTables(t)
	user := createFundsTestUser(t, 1721, 50)
	token := createFundsTestToken(t, user.Id, "drift-operator", 50)
	attempt := createHeldTestAttempt(t, user.Id, token.Id, "operator", 25, "wallet")
	require.NoError(t, MarkTaskCreateAttemptUnknown(attempt.ID, "req"))
	require.NoError(t, DB.First(attempt, attempt.ID).Error)

	require.NoError(t, RequestVideoRefund("attempt", attempt.ID, videoAttemptRefundVersion(attempt), 77, "manual warranty refund"))
	result, err := ReleaseTaskCreateAttemptHoldWarranty(attempt.ID, TaskCreateAttemptReleaseWarrantyManual, 0)
	require.NoError(t, err)
	assert.Equal(t, 25, result.ReleasedQuota)

	require.NoError(t, DB.First(attempt, attempt.ID).Error)
	assert.Equal(t, 77, attempt.RefundOperatorID)
	assert.Equal(t, 77, attempt.ManualRecoveryBy)
	require.NoError(t, DB.First(user, user.Id).Error)
	assert.Equal(t, 50, user.Quota)
}
