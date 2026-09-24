package service

import (
	"context"
	"fmt"
	"runtime"
	"testing"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/dto"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/pkg/billingexpr"
	hosttypes "github.com/QuantumNous/new-api/types"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func persistTaskCalculationFixture(t *testing.T, task *model.Task) {
	t.Helper()
	if task.ID == 0 {
		require.NoError(t, model.DB.Create(task).Error)
	}
}

func TestTaskCalculationProjectionDoesNotReevaluateAfterUpgrade(t *testing.T) {
	expression := `tier("normal", c*3)`
	task := &model.Task{PrivateData: model.TaskPrivateData{AsyncBilling: &model.TaskAsyncBillingContext{
		TieredSnapshot: &billingexpr.BillingSnapshot{ExprString: expression, ExprHash: billingexpr.ExprHashString(expression), GroupRatio: 1, QuotaPerUnit: 1000000},
		ActualTokens:   20, ActualUsageReported: true,
	}}}
	result, _, err := ComputeTaskTieredBilling(task)
	require.NoError(t, err)
	task.PrivateData.AsyncBilling.Calculation = result.Calculation
	task.PrivateData.AsyncBilling.State = model.TaskBillingStateSettled
	task.PrivateData.AsyncBilling.Operation = "settle"
	before := taskBillingOther(task).Snapshot()
	task.PrivateData.AsyncBilling.TieredSnapshot.ExprString = `invalid new code(`
	task.PrivateData.AsyncBilling.TieredSnapshot.GroupRatio = 500
	after := taskBillingOther(task).Snapshot()
	assert.Equal(t, before["billing_calculation"], after["billing_calculation"])
	assert.Equal(t, "normal", after["matched_tier"])
	assert.Equal(t, 60, result.Calculation.Quota)
}

func TestBatchCalculationMaximumLegalFile(t *testing.T) {
	// Measure the actual supported file size, with a representative cache/tier
	// expression and exact per-line rounding. No timing threshold is asserted.
	require.NoError(t, model.DB.AutoMigrate(&model.BatchJobLine{}, &model.BatchJob{}))
	jobID := "batch-calculation-size"
	t.Cleanup(func() { model.DB.Where("job_id = ?", jobID).Delete(&model.BatchJobLine{}) })
	frozen := frozenSnapshot(`p > 2000 ? tier("large", p*3+c*6+cr*0.3) : tier("small", p*2+c*4+cr*0.2)`, &hosttypes.ContractBillingFact{RatioUnits: 90000000})
	lines := make([]BatchLineEstimate, dto.MaxBatchRequestsPerFile)
	for i := range lines {
		lines[i] = BatchLineEstimate{CustomId: fmt.Sprintf("request-%d", i), InputEst: 1000, OutputCap: 100}
	}
	var before, after runtime.MemStats
	runtime.ReadMemStats(&before)
	start := time.Now()
	estimate, err := estimateBatchJobQuota(frozen, lines)
	require.NoError(t, err)
	require.Positive(t, estimate)
	encoded, err := common.Marshal(frozen)
	require.NoError(t, err)
	facts := make([]model.BatchJobLine, 0, len(lines))
	totalBytes := 0
	for _, line := range lines {
		modelQuota, quota, _, c, err := computeBatchLineCalculation(frozen, BatchLineUsage{CustomId: line.CustomId, InputTokens: 1000, OutputTokens: 100, CachedTokens: 200})
		require.NoError(t, err)
		require.Equal(t, 918, quota)
		b, err := common.Marshal(c)
		require.NoError(t, err)
		totalBytes += len(b)
		facts = append(facts, model.BatchJobLine{JobId: jobID, CustomId: line.CustomId, ModelQuota: modelQuota, FinalQuota: quota, Calculation: model.BillingText(b), CalculationVersion: 1})
	}
	require.NoError(t, model.DB.CreateInBatches(facts, 50).Error)
	page, err := model.GetBatchBillingLines(jobID, 0, 100)
	require.NoError(t, err)
	pageJSON, err := common.Marshal(page)
	require.NoError(t, err)
	runtime.ReadMemStats(&after)
	t.Logf("rows=%d estimate_bytes=%d settlement_bytes=%d list_page_bytes=%d allocated_bytes=%d elapsed=%s", len(facts), len(encoded), totalBytes, len(pageJSON), after.TotalAlloc-before.TotalAlloc, time.Since(start))
	// The DTO page intentionally excludes trace. The row-detail endpoint retrieves it.
	assert.NotContains(t, string(pageJSON), "calculation")
}

func TestNativeTaskCalculationIsSavedWithFunding(t *testing.T) {
	truncate(t)
	seedUser(t, 620, 10000)
	task := makeTask(620, 0, 100, 0, BillingSourceWallet, 0)
	persistTaskCalculationFixture(t, task)
	c := billingexpr.NewCalculation()
	c.Add("multiply", "quota", 80, 20, 4)
	setTaskCalculation(task, c.Finish(80), "settlement")
	RecalculateTaskQuota(context.Background(), task, 80, "calculation")
	var saved model.Task
	require.NoError(t, model.DB.First(&saved, task.ID).Error)
	require.NotNil(t, saved.PrivateData.BillingContext.SettlementCalculation)
	assert.Equal(t, 80, saved.Quota)
	assert.Equal(t, 80, saved.PrivateData.BillingContext.SettlementCalculation.Quota)
	assert.Nil(t, saved.PrivateData.AsyncBilling)
	assert.Equal(t, 10020, getUserQuota(t, 620))
	RecalculateTaskQuota(context.Background(), task, 80, "retry")
	assert.Equal(t, 10020, getUserQuota(t, 620))
}

func TestNativeUsageSettlementPreservesWalletAndTokenOverdraft(t *testing.T) {
	truncate(t)
	seedUser(t, 621, 0)
	seedToken(t, 621, 621, "native-settlement-token", 0)
	task := makeTask(621, 0, 100, 621, BillingSourceWallet, 0)
	task.Platform = "suno"
	task.Status = model.TaskStatusSuccess
	persistTaskCalculationFixture(t, task)
	setTaskCalculation(task, billingexpr.NewCalculation().Finish(150), "settlement")
	RecalculateTaskQuota(context.Background(), task, 150, "actual usage")
	var saved model.Task
	require.NoError(t, model.DB.First(&saved, task.ID).Error)
	require.NotNil(t, saved.PrivateData.BillingContext.SettlementCalculation)
	assert.Equal(t, 150, saved.Quota)
	assert.Equal(t, 150, saved.PrivateData.BillingContext.SettlementCalculation.Quota)
	assert.Nil(t, saved.PrivateData.AsyncBilling)
	assert.Equal(t, -50, getUserQuota(t, 621))
	var token model.Token
	require.NoError(t, model.DB.First(&token, 621).Error)
	assert.Equal(t, -50, token.RemainQuota)
	assert.Equal(t, 50, token.UsedQuota)
	RecalculateTaskQuota(context.Background(), &saved, 150, "retry")
	assert.Equal(t, -50, getUserQuota(t, 621))
	require.NoError(t, model.DB.First(&token, 621).Error)
	assert.Equal(t, 50, token.UsedQuota)
}

func TestDelayedCreateLogUsesInitialCalculation(t *testing.T) {
	initial := billingexpr.NewCalculation().Finish(100)
	final := billingexpr.NewCalculation().Finish(60)
	task := &model.Task{PrivateData: model.TaskPrivateData{BillingContext: &model.TaskBillingContext{InitialCalculation: initial}, AsyncBilling: &model.TaskAsyncBillingContext{Calculation: final, CalculationSource: "settlement"}}}
	for _, event := range []string{"create", "complete", "settle"} {
		log, err := BuildTaskBillingDeliveryLog(task, model.TaskBillingDelivery{Event: event, AfterQuota: 60})
		require.NoError(t, err)
		var values struct {
			Calculation billingexpr.Calculation `json:"billing_calculation"`
		}
		require.NoError(t, common.UnmarshalJsonStr(log.Other, &values))
		expected := 60
		if event == "create" {
			expected = 100
		}
		assert.Equal(t, expected, values.Calculation.Quota)
	}
}
