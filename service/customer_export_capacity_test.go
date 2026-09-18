package service

import (
	"context"
	"encoding/csv"
	"io"
	"os"
	"strings"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// Explicit acceptance run: real SQLite scans, production batching/throttling,
// CSV serialization and publication; object storage is the test adapter.
// This proves the >100k delivery boundary, not production latency or OSS SLA.
func TestCustomerExportLargeScopeCompletesWithoutTruncation(t *testing.T) {
	if os.Getenv("CUSTOMER_EXPORT_CAPACITY_ACCEPTANCE") != "1" {
		t.Skip("explicit 100001-row acceptance run")
	}
	store := setupCustomerExportServiceTest(t)
	const userID, count = 98762, 100001
	require.NoError(t, model.DB.Create(&model.User{Id: userID, Username: "export-capacity", AffCode: "exp98762", Status: common.UserStatusEnabled, Role: common.RoleCommonUser}).Error)
	t.Cleanup(func() {
		model.LOG_DB.Where("user_id = ?", userID).Delete(&model.Log{})
		model.DB.Unscoped().Delete(&model.User{}, userID)
	})
	for offset := 0; offset < count; offset += 500 {
		rows := make([]model.Log, min(500, count-offset))
		for i := range rows {
			rows[i] = model.Log{UserId: userID, TokenId: 7, Type: model.LogTypeConsume, CreatedAt: 1000, ModelName: "capacity-model", Quota: 1, PromptTokens: 1, Other: `{"model_ratio":1,"group_ratio":1,"contract_applicable":false}`}
		}
		require.NoError(t, model.LOG_DB.Create(&rows).Error)
	}
	filters := model.CustomerExportFilters{FieldVersion: customerExportFieldVersion, StartTimestamp: 900, EndTimestamp: 1100, Timezone: "Asia/Shanghai", QuotaPerUnit: 500000, Currency: "USD", CurrencyRate: 1}
	job, _, err := model.CreateCustomerExportJob(userID, userID, model.CustomerExportJobTypeUsageLogs, filters)
	require.NoError(t, err)
	job, err = model.ClaimNextQueuedCustomerExportJob("capacity", common.GetTimestamp()+120, 1800)
	require.NoError(t, err)
	runCustomerExportJob(context.Background(), job, "capacity", nil)
	finished, err := model.GetCustomerExportJob(job.JobID)
	require.NoError(t, err)
	require.Equal(t, model.CustomerExportJobStatusSucceeded, finished.Status)
	artifact := finished.DecodeArtifact()
	require.NotNil(t, artifact)
	assert.EqualValues(t, count, artifact.LineCount)
	var delivered int
	for _, file := range artifact.Files {
		reader := csv.NewReader(strings.NewReader(strings.TrimPrefix(string(store.uploaded[file.ObjectKey]), customerExportCsvBOM)))
		_, err := reader.Read()
		require.NoError(t, err)
		for {
			row, err := reader.Read()
			if err == io.EOF {
				break
			}
			require.NoError(t, err)
			require.Equal(t, "1", row[31])
			delivered++
		}
	}
	assert.Equal(t, count, delivered)
	statement, err := model.GetBillingCustomerStatement(context.Background(), userID, 900, 1100, "api_key", 0, "", "")
	require.NoError(t, err)
	assert.EqualValues(t, count, statement.Summary.NetQuota)
	assert.EqualValues(t, count, statement.Summary.Requests)
}
