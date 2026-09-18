package service

import (
	"context"
	"encoding/base64"
	"encoding/csv"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestCustomerExportSchedulerFinishesAndCanWakeAgain(t *testing.T) {
	setupCustomerExportServiceTest(t)
	require.NoError(t, model.DB.AutoMigrate(&model.SystemTask{}, &model.SystemTaskLock{}))
	require.NoError(t, model.DB.Where("type = ?", SystemTaskTypeCustomerExport).Delete(&model.SystemTask{}).Error)
	for i := 0; i < 2; i++ {
		task, _, err := EnqueueSystemTask(SystemTaskTypeCustomerExport, nil)
		require.NoError(t, err)
		claimed, ok, err := model.ClaimSystemTask(task.ID, task.Type, "test-runner", common.GetTimestamp()+120)
		require.NoError(t, err)
		require.True(t, ok)
		customerExportHandler{}.Run(context.Background(), claimed, "test-runner")
		finished, err := model.GetSystemTaskByTaskID(task.TaskID)
		require.NoError(t, err)
		assert.Equal(t, model.SystemTaskStatusSucceeded, finished.Status)
		assert.Nil(t, finished.ActiveKey)
	}
}

func TestCustomerExportSummaryEmptyMonthAndFrozenCurrency(t *testing.T) {
	files, _, count, err := writeCustomerExportSummaryCsv(t.TempDir(), customerExportScopeColumns{}, "en", model.BillingCustomerStatement{})
	require.NoError(t, err)
	assert.Empty(t, files)
	assert.Zero(t, count)
	scope := customerExportScopeColumns{GeneratedAt: 1000, QuotaPerUnit: 500000, Currency: "CNY", CurrencyRate: 7}
	assert.Equal(t, "-1.40000000", exportCurrencyAmount("-100000", scope))
	original, discount := int64(1000000), int64(500000)
	statement := model.BillingCustomerStatement{Summary: model.BillingReconciliationUsage{InputTokens: 0, NetQuota: 500000}, OriginalQuota: &original, DiscountQuota: &discount, Groups: []model.BillingReconciliationGroupSummary{{Id: 1, Models: []model.BillingReconciliationModelSummary{{ModelName: "model", OriginalQuota: &original, Usage: model.BillingReconciliationUsage{NetQuota: 500000}, DataQuality: &model.BillingReconciliationDataQuality{Status: "partial", InputTokensUnavailableRequests: 1}}}}}, DataQuality: &model.BillingReconciliationDataQuality{Status: "partial", InputTokensUnavailableRequests: 1}}
	_, paths, _, err := writeCustomerExportSummaryCsv(t.TempDir(), scope, "en", statement)
	require.NoError(t, err)
	raw, err := os.ReadFile(paths[0])
	require.NoError(t, err)
	rows, err := csv.NewReader(strings.NewReader(strings.TrimPrefix(string(raw), customerExportCsvBOM))).ReadAll()
	require.NoError(t, err)
	values := map[string]string{}
	for i, key := range customerExportSummaryColumnOrder {
		values[key] = rows[1][i]
	}
	assert.Empty(t, values["input_tokens"])
	assert.Equal(t, "7.00000000", values["net_amount"])
	assert.Equal(t, "7.00000000", values["discount_amount"])
	assert.Equal(t, formatExportTimestamp(1000), values["generated_at"])
}

func TestCustomerBillingExplanationUsesFrozenTokenAndTaskUnits(t *testing.T) {
	for _, tc := range []struct {
		name, other               string
		prompt, completion, quota int
		label                     string
		quantity, price, subtotal float64
	}{
		{"plain", `{"model_ratio":1,"completion_ratio":5,"group_ratio":1,"contract_applicable":false}`, 1000, 100, 1500, "Input", 1000, 2, 0.002},
		{"tiered cache", `{"billing_mode":"tiered_expr","usage_semantic":"openai","cache_tokens":300,"group_ratio":1,"contract_applicable":false,"matched_tier":"base","expr_b64":"` + base64.StdEncoding.EncodeToString([]byte(`tier("base", p * 2 + c * 10 + cr * 0.2)`)) + `"}`, 1000, 100, 1230, "Input", 700, 2, 0.0014},
		{"seconds", `{"billing_mode":"tiered_expr","matched_tier":"base","usage_units":{"seconds":"second"},"usage_facts":{"seconds":5},"group_ratio":1,"contract_applicable":false,"expr_b64":"` + base64.StdEncoding.EncodeToString([]byte(`tier("base", u("seconds") * 0.4)`)) + `"}`, 0, 0, 1000000, "seconds", 5, 0.4, 2},
	} {
		t.Run(tc.name, func(t *testing.T) {
			log := &model.Log{Type: model.LogTypeConsume, Quota: tc.quota, PromptTokens: tc.prompt, CompletionTokens: tc.completion, Other: tc.other}
			var other map[string]any
			require.NoError(t, common.UnmarshalJsonStr(log.Other, &other))
			lines := customerBillingLines(log, other, model.CustomerBillingLogRow(log), common.QuotaPerUnit)
			var found *customerBillingLine
			for i := range lines {
				if lines[i].Label == tc.label {
					found = &lines[i]
				}
			}
			require.NotNil(t, found)
			assert.Equal(t, tc.quantity, found.Quantity)
			assert.Equal(t, tc.price, found.UnitPrice)
			assert.InDelta(t, tc.subtotal, found.Subtotal, 1e-10)
		})
	}
}

type cancellingExportStore struct {
	*stubExportStore
	jobID  string
	userID int
}

func (s *cancellingExportStore) ExportPutFile(ctx context.Context, key, mime, path string, size int64) error {
	if err := s.stubExportStore.ExportPutFile(ctx, key, mime, path, size); err != nil {
		return err
	}
	_, err := model.CancelCustomerExportJob(s.jobID, s.userID)
	return err
}

func TestCustomerExportCancellationDuringUploadNeverPublishes(t *testing.T) {
	store := setupCustomerExportServiceTest(t)
	const userID = 98761
	require.NoError(t, model.DB.Create(&model.User{Id: userID, Username: "export-cancellation", AffCode: "exp98761", Status: common.UserStatusEnabled, Role: common.RoleCommonUser}).Error)
	t.Cleanup(func() {
		model.DB.Unscoped().Delete(&model.User{}, userID)
		model.LOG_DB.Where("user_id = ?", userID).Delete(&model.Log{})
	})
	require.NoError(t, model.LOG_DB.Create(&model.Log{UserId: userID, Type: model.LogTypeConsume, CreatedAt: 1000, Other: `{"model_ratio":1,"group_ratio":1,"contract_applicable":false}`}).Error)
	filters := model.CustomerExportFilters{FieldVersion: customerExportFieldVersion, StartTimestamp: 900, EndTimestamp: 1100, QuotaPerUnit: 500000, Currency: "USD", CurrencyRate: 1}
	job, _, err := model.CreateCustomerExportJob(userID, userID, model.CustomerExportJobTypeUsageLogs, filters)
	require.NoError(t, err)
	job, err = model.ClaimNextQueuedCustomerExportJob("executor", common.GetTimestamp()+120, 1800)
	require.NoError(t, err)
	customerExportStoreOverride = &cancellingExportStore{store, job.JobID, userID}
	runCustomerExportJob(context.Background(), job, "executor", nil)
	finished, err := model.GetCustomerExportJob(job.JobID)
	require.NoError(t, err)
	assert.Equal(t, model.CustomerExportJobStatusCancelled, finished.Status)
	assert.Nil(t, finished.ToView().Artifact)
	require.NotEmpty(t, finished.Artifact)
	customerExportCleanupPass(context.Background())
	assert.Empty(t, store.uploaded)
	_, _, err = PresignCustomerExportURL("file", 1, "other-store")
	assert.True(t, errors.Is(err, ErrCustomerExportStorageUnavailable))
}

func TestCustomerExportTemporaryCleanupPreservesActiveWork(t *testing.T) {
	setupCustomerExportServiceTest(t)
	filters := model.CustomerExportFilters{FieldVersion: customerExportFieldVersion, StartTimestamp: 1000, EndTimestamp: 2000}
	job, _, err := model.CreateCustomerExportJob(42, 42, model.CustomerExportJobTypeUsageLogs, filters)
	require.NoError(t, err)
	job, err = model.ClaimNextQueuedCustomerExportJob("cleanup", common.GetTimestamp()+120, 1800)
	require.NoError(t, err)
	work := filepath.Join(customerExportTemporaryRoot(), job.JobID)
	require.NoError(t, os.MkdirAll(work, 0700))
	require.NoError(t, os.WriteFile(filepath.Join(work, "shard"), []byte("partial export"), 0600))
	cleanupCustomerExportTemporaryFiles(context.Background())
	_, err = os.Stat(work)
	require.NoError(t, err)
	require.NoError(t, model.FinishCustomerExportJob(job.JobID, "cleanup", model.CustomerExportJobStatusFailed, "interrupted", "", nil))
	cleanupCustomerExportTemporaryFiles(context.Background())
	_, err = os.Stat(work)
	assert.True(t, os.IsNotExist(err))
}
