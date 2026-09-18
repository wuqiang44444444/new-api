package model

import (
	"context"
	"encoding/base64"
	"path/filepath"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/pkg/billingexpr"
	"github.com/glebarez/sqlite"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

func TestStatementClassifiesDurationWithoutInventingTokenOrCallUsage(t *testing.T) {
	for _, tc := range []struct {
		name, expression, mode string
		units                  map[string]string
	}{
		{"historical duration", `param("_task.has_video_input") == true ? tier("video", param("_task.duration_seconds") * 200000) : tier("base", param("_task.duration_seconds") * 100000)`, "per_second", nil},
		{"declared seconds", `tier("base", u("meter") * 0.4)`, "per_second", map[string]string{"meter": "second"}},
		{"duration with enum", `u("resolution") == "720p" ? tier("hd", u("meter") * 0.4) : tier("sd", u("meter") * 0.2)`, "per_second", map[string]string{"meter": "second", "resolution": "enum"}},
		{"undeclared seconds", `tier("base", u("seconds") * 0.4)`, "unknown", nil},
		{"client parameter is not a unit", `tier("base", param("duration_seconds") * 0.4)`, "unknown", nil},
		{"mixed token and seconds", `tier("base", c + param("_task.duration_seconds") * 100000)`, "unknown", nil},
		{"mixed declared and token variables", `tier("base", c + u("meter"))`, "unknown", map[string]string{"meter": "second"}},
		{"dynamic meter", `tier("base", u(param("unit")))`, "unknown", map[string]string{"seconds": "second"}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			assert.Equal(t, tc.mode, BillingStatementExpressionMode(tc.expression, tc.units))
		})
	}
}

func TestDurationStatementRecoversRefundModeFromFrozenTaskAcrossPeriods(t *testing.T) {
	db := setupBillingReconciliationTestDB(t)
	sqlDB, err := db.DB()
	require.NoError(t, err)
	sqlDB.SetMaxOpenConns(1)
	require.NoError(t, db.Create(&User{Id: 7, Username: "customer"}).Error)
	expression := `tier("seconds", param("_task.duration_seconds") * 100000)`
	task := Task{TaskID: "duration-task", UserId: 7, AppID: 4, ChannelId: 76,
		Properties: Properties{OriginModelName: "video"},
		PrivateData: TaskPrivateData{TokenId: 4, AsyncBilling: &TaskAsyncBillingContext{
			TieredSnapshot: &billingexpr.BillingSnapshot{ExprString: expression},
		}},
	}
	require.NoError(t, db.Create(&task).Error)
	other, err := common.Marshal(map[string]any{"contract_applicable": false, "task_id": task.TaskID, "is_task": true, "model_price": 0, "group_ratio": 0.87,
		"expr_b64": base64.StdEncoding.EncodeToString([]byte(expression))})
	require.NoError(t, err)
	require.NoError(t, db.Create(&[]Log{
		{UserId: 7, TokenId: 4, ChannelId: 76, ModelName: "video", CreatedAt: 900, Type: LogTypeConsume, Quota: 870, Other: string(other)},
		{UserId: 7, TokenId: 4, ChannelId: 76, ModelName: "video", CreatedAt: 1100, Type: LogTypeRefund, Quota: 870, Other: `{"contract_applicable":false,"task_id":"duration-task","model_price":0,"group_ratio":0.87,"admin_info":{"statement_snapshot":{"billing_mode":"per_call"}}}`},
	}).Error)
	for _, start := range []int64{800, 1000} {
		s, err := GetBillingCustomerStatement(context.Background(), 7, start, 1200, "api_key", 4, "video", "per_second")
		require.NoError(t, err)
		require.Len(t, s.Groups, 1)
		require.Len(t, s.Groups[0].Models, 1)
		item := s.Groups[0].Models[0]
		assert.Equal(t, "per_second", item.BillingMode)
		assert.Zero(t, item.Usage.BillableCalls)
		assert.Zero(t, item.Usage.RefundedCalls)
		assert.Zero(t, item.Usage.OutputTokens)
		assert.Equal(t, "complete", item.DataQuality.Status)
		token := 4
		detail, err := GetBillingStatementLogs(context.Background(), BillingStatementLogFilter{UserId: 7, Start: start, End: 1200, TokenId: &token, ModelName: "video", BillingMode: "per_second"}, 1, 10, common.RoleCommonUser)
		require.NoError(t, err)
		assert.Equal(t, item.Usage.NetQuota, detail.Quota)
		list, err := GetBillingCustomerStatementList(context.Background(), start, 1200, "", "", "net_quota", "desc", 1, 20)
		require.NoError(t, err)
		require.Len(t, list.Items, 1)
		assert.Equal(t, s.Summary, list.Items[0].Usage)
		assert.Equal(t, "complete", list.Items[0].DataQuality.Status)
		if start == 1000 {
			assert.EqualValues(t, -870, item.Usage.NetQuota)
			assert.EqualValues(t, -1000, *item.OriginalQuota)
			assert.EqualValues(t, 1, detail.Total)
		} else {
			assert.Zero(t, item.Usage.NetQuota)
			assert.EqualValues(t, 1, item.Usage.Requests)
			assert.EqualValues(t, 2, detail.Total)
		}
	}
	var persisted Log
	require.NoError(t, db.Where("type = ?", LogTypeRefund).First(&persisted).Error)
	assert.NotContains(t, persisted.Other, "per_second", "statement reads must not rewrite logs")
}

func TestDurationRefundRequiresUniqueMatchingFrozenEvidence(t *testing.T) {
	for _, scenario := range []string{"missing task", "user", "application", "key", "channel", "model", "missing snapshot", "duplicate task", "existing expression"} {
		t.Run(scenario, func(t *testing.T) {
			db := setupBillingReconciliationTestDB(t)
			require.NoError(t, db.Create(&User{Id: 7, Username: "customer"}).Error)
			task := Task{TaskID: "refund-task", UserId: 7, AppID: 4, ChannelId: 76,
				Properties: Properties{OriginModelName: "video"},
				PrivateData: TaskPrivateData{TokenId: 4, AsyncBilling: &TaskAsyncBillingContext{
					TieredSnapshot: &billingexpr.BillingSnapshot{ExprString: `tier("seconds", param("_task.duration_seconds") * 100000)`},
				}},
			}
			other := map[string]any{"contract_applicable": false, "task_id": task.TaskID, "model_price": 0, "group_ratio": 1}
			switch scenario {
			case "user":
				task.UserId = 8
			case "application":
				task.AppID = 5
			case "key":
				task.PrivateData.TokenId = 5
			case "channel":
				task.ChannelId = 77
			case "model":
				task.Properties.OriginModelName = "other-model"
			case "missing snapshot":
				task.PrivateData.AsyncBilling.TieredSnapshot = nil
			case "existing expression":
				other["expr_b64"] = base64.StdEncoding.EncodeToString([]byte(`tier("credits", u("credits"))`))
			}
			if scenario != "missing task" {
				require.NoError(t, db.Create(&task).Error)
			}
			if scenario == "duplicate task" {
				task.ID = 0
				require.NoError(t, db.Create(&task).Error)
			}
			encoded, err := common.Marshal(other)
			require.NoError(t, err)
			require.NoError(t, db.Create(&Log{UserId: 7, TokenId: 4, ChannelId: 76, ModelName: "video", Type: LogTypeRefund, CreatedAt: 1100, Quota: 100, Other: string(encoded)}).Error)
			s, err := GetBillingCustomerStatement(context.Background(), 7, 1000, 1200, "api_key", 4, "video", "unknown")
			require.NoError(t, err)
			require.Len(t, s.Groups, 1)
			assert.EqualValues(t, -100, s.Summary.NetQuota)
			assert.EqualValues(t, 1, s.DataQuality.UnknownBillingModeRequests)
		})
	}
}

func TestDurationRefundReadsSnapshotFromMainDatabaseAndFailsOnDatabaseError(t *testing.T) {
	main := setupBillingReconciliationTestDB(t)
	logs, err := gorm.Open(sqlite.Open(filepath.Join(t.TempDir(), "logs.db")), &gorm.Config{})
	require.NoError(t, err)
	connection, err := logs.DB()
	require.NoError(t, err)
	t.Cleanup(func() { _ = connection.Close() })
	LOG_DB = logs
	require.NoError(t, logs.AutoMigrate(&Log{}))
	require.NoError(t, main.Create(&User{Id: 7, Username: "customer"}).Error)
	require.NoError(t, main.Create(&Task{TaskID: "task", UserId: 7, AppID: 4, ChannelId: 76,
		Properties: Properties{OriginModelName: "video"}, PrivateData: TaskPrivateData{TokenId: 4,
			BillingContext: &TaskBillingContext{TieredSnapshot: &billingexpr.BillingSnapshot{
				ExprString: `tier("seconds", u("meter") * 0.4)`, UsageUnits: map[string]string{"meter": "second"},
			}},
		}}).Error)
	require.NoError(t, logs.Create(&Log{UserId: 7, TokenId: 4, ChannelId: 76, ModelName: "video", Type: LogTypeRefund, CreatedAt: 1100, Quota: 100, Other: `{"contract_applicable":false,"task_id":"task","model_price":0,"group_ratio":1}`}).Error)
	s, err := GetBillingCustomerStatement(context.Background(), 7, 1000, 1200, "api_key", 4, "video", "per_second")
	require.NoError(t, err)
	assert.EqualValues(t, -100, s.Summary.NetQuota)
	require.NoError(t, main.Migrator().DropTable(&Task{}))
	_, err = GetBillingCustomerStatement(context.Background(), 7, 1000, 1200, "api_key", 4, "video", "per_second")
	require.Error(t, err, "a failed fact lookup must not silently produce a different statement")
}
