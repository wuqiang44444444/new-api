package model

import (
	"errors"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/pkg/billingexpr"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

func TestTaskCalculationFundingAndEvidenceCommitTogether(t *testing.T) {
	truncateTables(t)
	user := User{Username: "calculation-owner", Quota: 1000}
	require.NoError(t, DB.Create(&user).Error)
	task := persistedBillingTargetTask(t, user.Id, 0, "wallet", 0)
	task.PrivateData.AsyncBilling.CalculationVersion = 1
	task.PrivateData.AsyncBilling.CalculationSource = "settlement"
	_, _, err := ApplyTaskBillingTarget(task, 80)
	require.ErrorContains(t, err, "calculation")
	c := billingexpr.NewCalculation()
	c.Add("multiply", "quota", 80, 20, 4)
	task.PrivateData.AsyncBilling.Calculation = c.Finish(80)
	name := "test_calculation_persistence_failure"
	require.NoError(t, DB.Callback().Update().Before("gorm:update").Register(name, func(tx *gorm.DB) {
		if tx.Statement.Table == "tasks" {
			tx.AddError(errors.New("record write failed"))
		}
	}))
	_, _, err = ApplyTaskBillingTarget(task, 80)
	require.ErrorContains(t, err, "record write failed")
	require.NoError(t, DB.Callback().Update().Remove(name))
	var current User
	require.NoError(t, DB.First(&current, user.Id).Error)
	assert.Equal(t, 1000, current.Quota)
	var saved Task
	require.NoError(t, DB.First(&saved, task.ID).Error)
	assert.Nil(t, saved.PrivateData.AsyncBilling.Calculation)
	applied, _, err := ApplyTaskBillingTarget(task, 80)
	require.NoError(t, err)
	require.True(t, applied)
	require.NoError(t, DB.First(&saved, task.ID).Error)
	assert.Equal(t, 80, saved.Quota)
	assert.Equal(t, c, saved.PrivateData.AsyncBilling.Calculation)
	task.PrivateData.AsyncBilling.Calculation = billingexpr.NewCalculation().Finish(80)
	applied, _, err = ApplyTaskBillingTarget(task, 80)
	require.NoError(t, err)
	assert.False(t, applied)
	require.NoError(t, DB.First(&saved, task.ID).Error)
	assert.Equal(t, c, saved.PrivateData.AsyncBilling.Calculation)
}

func TestAcceptedTargetKeepsItsOriginalCalculation(t *testing.T) {
	quota := 80
	c := billingexpr.NewCalculation()
	c.Add("multiply", "quota", 80, 20, 4)
	stored := &TaskAsyncBillingContext{TargetQuota: &quota, CalculationVersion: 1, Calculation: c.Finish(quota)}
	proposed := *stored
	proposed.Calculation = billingexpr.NewCalculation().Finish(quota)
	merged, err := mergeSeedanceBillingFacts(stored, &proposed, false)
	require.NoError(t, err)
	assert.Same(t, stored.Calculation, merged.Calculation)
}

func TestBatchCalculationRejectsIncompleteNewLine(t *testing.T) {
	err := CommitBatchResultLines(&BatchJob{}, []BatchJobLine{{FinalQuota: 80, CalculationVersion: 1}})
	require.ErrorContains(t, err, "calculation")
	encoded, err := common.Marshal(billingexpr.NewCalculation().Finish(79))
	require.NoError(t, err)
	err = CommitBatchResultLines(&BatchJob{}, []BatchJobLine{{FinalQuota: 80, CalculationVersion: 1, Calculation: BillingText(encoded)}})
	require.ErrorContains(t, err, "calculation")
}

func TestBatchInitialCalculationBeforeResults(t *testing.T) {
	require.NoError(t, DB.AutoMigrate(&BatchJobLine{}))
	frozen := BatchFrozenSnapshot{InitialCalculations: billingexpr.CalculationArchive{"request-1": billingexpr.NewCalculation().Finish(80)}}
	encoded, err := common.Marshal(frozen)
	require.NoError(t, err)
	job := &BatchJob{Id: "unsubmitted-calculation-test", FrozenSnapshot: BillingText(encoded)}
	line, err := GetBatchLineCalculation(job, "request-1")
	require.NoError(t, err)
	require.NotNil(t, line)
	assert.Equal(t, 80, line.Quota)
	assert.Equal(t, 80, line.Initial.Quota)
	assert.Nil(t, line.Settlement)
	rows, err := GetBatchBillingPage(job, 0, 100)
	require.NoError(t, err)
	require.Len(t, rows, 1)
	assert.True(t, rows[0].Estimated)
	line, err = GetBatchLineCalculation(job, "missing")
	require.NoError(t, err)
	assert.Nil(t, line)
}

func TestImageAcceptanceRejectsMissingRecordedPrecharge(t *testing.T) {
	task := &Task{Status: TaskStatusQueued, ClientProtocol: TaskClientProtocolImageOpenAIV1, Quota: 80, PrivateData: TaskPrivateData{ImageTask: &TaskImageExecutionData{}, BillingContext: &TaskBillingContext{CalculationVersion: 1}}}
	err := InsertImageTask(ImageTaskInsertParams{Task: task})
	require.ErrorContains(t, err, "calculation")
	assert.Zero(t, task.ID)
}
