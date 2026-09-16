package service

import (
	"errors"
	"io"
	"net/http"
	"strings"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/setting/model_setting"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

func TestImageViolationFeeFrozenPolicyAndZeroRatio(t *testing.T) {
	settings := model_setting.GetGrokSettings()
	previous := *settings
	t.Cleanup(func() { *settings = previous })
	settings.ViolationDeductionEnabled, settings.ViolationDeductionAmount = true, 0.05
	for _, ratio := range []float64{0, 0.5, 1} {
		task := newWorkerImageTask(1, 100)
		task.PrivateData.BillingContext = &model.TaskBillingContext{GroupRatio: ratio}
		task.PrivateData.ImageTask.ViolationMarker = true
		freezeImageTaskViolationFeePolicy(task)
		want := calcViolationFeeQuota(0.05, ratio)
		settings.ViolationDeductionEnabled, settings.ViolationDeductionAmount = false, 999
		quota, _, err := frozenImageTaskViolationFee(task)
		require.NoError(t, err)
		assert.Equal(t, want, quota)
		settings.ViolationDeductionEnabled, settings.ViolationDeductionAmount = true, 0.05
	}
}

func TestImageViolationFeeRollbackAndReconcile(t *testing.T) {
	truncate(t)
	seedImageTaskUser(t, 9916, 1000)
	seedToken(t, 9917, 9916, "violation-fee-token", 1000)
	seedChannel(t, 42)
	task := newWorkerImageTask(9916, 100)
	task.PrivateData.TokenId = 9917
	task.PrivateData.AsyncBilling = &model.TaskAsyncBillingContext{State: model.TaskBillingStatePending}
	task.PrivateData.BillingContext = &model.TaskBillingContext{GroupRatio: 1}
	task.PrivateData.ImageTask.ViolationFeePolicy = &model.TaskImageViolationFeePolicy{Enabled: true, BaseAmount: 0.0004}
	require.NoError(t, model.InsertImageTask(model.ImageTaskInsertParams{Task: task,
		GlobalScope: model.ImageTaskAdmissionScopeGlobal(), AppScope: model.ImageTaskAdmissionScopeApp(task.UserId, task.AppID)}))
	won, err := model.FinishImageTaskFailureWithEvidence(task, model.TaskStatusFailure, "provider_rejected", model.TaskImageFailureEvidence{UpstreamStatus: 400, ViolationMarker: true})
	require.NoError(t, err)
	require.True(t, won)
	const callback = "image-violation-settlement-fault"
	require.NoError(t, model.DB.Callback().Update().Before("gorm:update").Register(callback, func(tx *gorm.DB) {
		if tx.Statement.Table == "tasks" {
			tx.AddError(errors.New("injected task write failure"))
		}
	}))
	t.Cleanup(func() { model.DB.Callback().Update().Remove(callback) })
	settleImageTaskBilling(t.Context(), task)
	assert.Equal(t, 900, getUserQuota(t, task.UserId))
	assert.Equal(t, 900, getTokenRemainQuota(t, 9917))
	assert.Equal(t, model.TaskBillingStatePending, reloadTask(t, task.ID).PrivateData.AsyncBilling.State)
	require.NoError(t, model.DB.Callback().Update().Remove(callback))
	settings := model_setting.GetGrokSettings()
	previous := *settings
	t.Cleanup(func() { *settings = previous })
	settings.ViolationDeductionEnabled, settings.ViolationDeductionAmount = false, 999
	ReconcileTaskBilling(t.Context(), 100)
	ReconcileTaskBilling(t.Context(), 100)
	persisted := reloadTask(t, task.ID)
	assert.Equal(t, 200, persisted.Quota)
	assert.Equal(t, 800, getUserQuota(t, task.UserId))
	assert.Equal(t, 800, getTokenRemainQuota(t, 9917))
	assert.Equal(t, model.TaskBillingStateSettled, persisted.PrivateData.AsyncBilling.State)
	log, err := BuildTaskBillingDeliveryLog(persisted, model.TaskBillingDelivery{Event: "complete", AfterQuota: 200})
	require.NoError(t, err)
	var other map[string]any
	require.NoError(t, common.UnmarshalJsonStr(log.Other, &other))
	assert.Equal(t, true, other["violation_fee"])
	assert.Equal(t, float64(200), other["fee_quota"])
}

func TestImageViolationFeeMatchesNativeErrorBoundary(t *testing.T) {
	for _, body := range []string{
		`{"error":{"message":"Content\u0020violates usage guidelines"}}`,
		`{"error":{"message":"invalid quality"},"note":"Content violates usage guidelines"}`,
		`{"error":{"code":"violation_fee.grok.csam","message":"rejected"}}`,
	} {
		resp := &http.Response{StatusCode: 400, Body: io.NopCloser(strings.NewReader(body))}
		nativeError := NormalizeViolationFeeError(RelayErrorHandler(t.Context(), resp, false))
		assert.Equal(t, shouldChargeViolationFee(nativeError), ImageTaskViolationFeeApplies([]byte(body), 400))
	}
}

func TestImageViolationFeeMissingSnapshotKeepsFundsPending(t *testing.T) {
	truncate(t)
	seedImageTaskUser(t, 9918, 1000)
	seedChannel(t, 42)
	task := newWorkerImageTask(9918, 100)
	task.PrivateData.AsyncBilling = &model.TaskAsyncBillingContext{State: model.TaskBillingStatePending}
	task.PrivateData.BillingContext = &model.TaskBillingContext{GroupRatio: 1}
	require.NoError(t, model.InsertImageTask(model.ImageTaskInsertParams{Task: task,
		GlobalScope: model.ImageTaskAdmissionScopeGlobal(), AppScope: model.ImageTaskAdmissionScopeApp(task.UserId, task.AppID)}))
	won, err := model.FinishImageTaskFailureWithEvidence(task, model.TaskStatusFailure, "provider_rejected", model.TaskImageFailureEvidence{UpstreamStatus: 400, ViolationMarker: true})
	require.NoError(t, err)
	require.True(t, won)
	settleImageTaskBilling(t.Context(), task)
	ReconcileTaskBilling(t.Context(), 100)
	assert.Equal(t, 900, getUserQuota(t, task.UserId))
	persisted := reloadTask(t, task.ID)
	assert.Equal(t, 100, persisted.Quota)
	assert.Equal(t, model.TaskBillingStatePending, persisted.PrivateData.AsyncBilling.State)
}
