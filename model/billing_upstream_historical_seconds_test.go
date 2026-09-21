package model

import (
	"context"
	"encoding/base64"
	"testing"

	"github.com/QuantumNous/new-api/pkg/billingexpr"
	hosttypes "github.com/QuantumNous/new-api/types"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestHistoricalBillableSecondsRequireVerifiedSettlement(t *testing.T) {
	for _, tc := range []struct {
		name    string
		change  func(*Task, *Log)
		source  string
		missing int64
	}{
		{name: "successful settlement", source: "billing_parameters"},
		{name: "contract discount", source: "billing_parameters", change: func(task *Task, log *Log) {
			task.PrivateData.BillingContext = &TaskBillingContext{ContractFact: &hosttypes.ContractBillingFact{RatioUnits: 80000000}}
			task.Quota = 480
			task.PrivateData.AsyncBilling.TargetQuota = &task.Quota
			log.Quota = 480
		}},
		{name: "failed and refunded hold", source: "refunded_hold", change: func(task *Task, _ *Log) {
			task.Status = TaskStatusFailure
			task.Quota = 0
			task.PrivateData.AsyncBilling.TargetQuota = &task.Quota
			task.PrivateData.AsyncBilling.Operation = "refund"
		}},
		{name: "unsettled budget", missing: 1, change: func(task *Task, _ *Log) { task.PrivateData.AsyncBilling.State = TaskBillingStatePending }},
		{name: "charge mismatch", missing: 1, change: func(_ *Task, log *Log) { log.Quota++ }},
		{name: "final amount mismatch", missing: 1, change: func(task *Task, _ *Log) { task.Quota++ }},
		{name: "different user", missing: 1, change: func(task *Task, _ *Log) { task.UserId = 8 }},
		{name: "different channel", missing: 1, change: func(task *Task, _ *Log) { task.ChannelId = 22 }},
		{name: "different customer model", missing: 1, change: func(task *Task, _ *Log) { task.Properties.OriginModelName = "different" }},
		{name: "missing task link", missing: 1, change: func(_ *Task, log *Log) {
			log.Other = `{"is_task":true,"group_ratio":1,"model_price":0,"expr_b64":"` + base64.StdEncoding.EncodeToString([]byte(`tier("video", param("_task.duration_seconds") * 200)`)) + `"}`
		}},
		{name: "corrupt frozen expression identity", missing: 1, change: func(task *Task, _ *Log) { task.PrivateData.AsyncBilling.TieredSnapshot.ExprHash = "bad" }},
		{name: "invalid duration", missing: 1, change: func(task *Task, _ *Log) {
			task.PrivateData.AsyncBilling.BillingProbe.Body = []byte(`{"_task":{"duration_seconds":-6}}`)
		}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			db := setupBillingReconciliationTestDB(t)
			expression := `tier("video", param("_task.duration_seconds") * 200)`
			target := 600
			task := Task{TaskID: "historical-video", UserId: 7, ChannelId: 21, Status: TaskStatusSuccess, BillingState: TaskBillingStateSettled, Quota: 600, Properties: Properties{OriginModelName: "video"}, PrivateData: TaskPrivateData{AsyncBilling: &TaskAsyncBillingContext{State: TaskBillingStateSettled, Operation: "settle", TargetQuota: &target, TieredSnapshot: &billingexpr.BillingSnapshot{ExprString: expression, ExprHash: billingexpr.ExprHashString(expression), GroupRatio: 1, QuotaPerUnit: 500000, ExprVersion: 1}, BillingProbe: &billingexpr.RequestInput{Body: []byte(`{"_task":{"duration_seconds":6}}`)}}}}
			log := Log{UserId: 7, ChannelId: 21, Type: LogTypeConsume, CreatedAt: 1100, ModelName: "video", Quota: 600, Other: `{"task_id":"historical-video","is_task":true,"group_ratio":1,"model_price":0,"expr_b64":"` + base64.StdEncoding.EncodeToString([]byte(expression)) + `"}`}
			if tc.change != nil {
				tc.change(&task, &log)
			}
			require.NoError(t, db.Create(&Channel{Id: 21, BaseURL: urlPtr("https://video.test")}).Error)
			require.NoError(t, db.Create(&task).Error)
			require.NoError(t, db.Create(&log).Error)
			summary, err := GetProviderBillingURLSummary(1000, 1200, 1000, "")
			require.NoError(t, err)
			details, err := GetUpstreamBillingDetails(context.Background(), UpstreamBillingDetailFilter{Start: 1000, End: 1200}, 1, 10, false)
			require.NoError(t, err)
			require.Len(t, details.Items, 1)
			for _, category := range []string{"incomplete", "recovered_billing_seconds_rows", "refunded_task_hold_rows"} {
				filtered, filterErr := GetUpstreamBillingDetails(context.Background(), UpstreamBillingDetailFilter{Start: 1000, End: 1200, EvidenceFilter: category}, 1, 10, false)
				require.NoError(t, filterErr)
				expected := int64(0)
				if category == "incomplete" {
					expected = tc.missing
				}
				if category == "recovered_billing_seconds_rows" && tc.source == "billing_parameters" {
					expected = 1
				}
				if category == "refunded_task_hold_rows" && tc.source == "refunded_hold" {
					expected = 1
				}
				assert.Equal(t, expected, filtered.Total, category)
			}
			row := details.Items[0]
			assert.Equal(t, tc.source, row.SecondsSource)
			assert.Equal(t, tc.missing, row.DataQuality.SecondsUnavailableRows)
			assert.Equal(t, tc.missing, summary.DataQuality.SecondsUnavailableRows)
			assert.Equal(t, tc.missing, summary.DataQuality.EvidenceCoverage.GapRows)
			if tc.source == "billing_parameters" {
				require.NotNil(t, row.Seconds)
				assert.Equal(t, "6", row.Seconds.String())
				require.NotNil(t, summary.Groups[0].Usage.Seconds)
				assert.Equal(t, "6", summary.Groups[0].Usage.Seconds.String())
				assert.EqualValues(t, 1, summary.DataQuality.RecoveredBillingSecondsRows)
			} else {
				assert.Nil(t, row.Seconds)
				assert.Nil(t, summary.Groups[0].Usage.Seconds)
			}
			if tc.source == "refunded_hold" {
				assert.Equal(t, "task_refunded_hold", row.Event)
				assert.EqualValues(t, 1, summary.DataQuality.RefundedTaskHoldRows)
			}
			require.NotNil(t, row.OriginalAmount)
			assert.EqualValues(t, log.Quota, *row.OriginalAmount, "recovering billing inputs must not change money")
		})
	}
}

func TestUpstreamSecondsRejectsAmbiguousTaskIdentity(t *testing.T) {
	db := setupBillingReconciliationTestDB(t)
	for i := 0; i < 2; i++ {
		require.NoError(t, db.Create(&Task{TaskID: "same", UserId: 7, ChannelId: 21, Status: TaskStatusSuccess}).Error)
	}
	result, err := resolveUpstreamTaskSeconds(context.Background(), []upstreamTaskSecondsKey{{7, 21, "same"}})
	require.NoError(t, err)
	assert.Empty(t, result, "ambiguous historical identities cannot supply billing facts")
	require.NoError(t, db.Migrator().DropTable(&Task{}))
	_, err = resolveUpstreamTaskSeconds(context.Background(), []upstreamTaskSecondsKey{{7, 21, "same"}})
	require.Error(t, err, "a failed source read must fail the report, not return partial recovery")
}
