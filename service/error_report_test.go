package service

import (
	"context"
	"fmt"
	"strings"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/setting/operation_setting"
	"github.com/glebarez/sqlite"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

// withErrorReportTestDB 建立内存 SQLite 主库与日志库（同库拓扑），
// 只迁移本功能需要的表，隔离其它测试的模型状态。
func withErrorReportTestDB(t *testing.T) *gorm.DB {
	t.Helper()
	previousOptions := common.OptionMap
	common.OptionMap = map[string]string{}
	t.Cleanup(func() { common.OptionMap = previousOptions })
	previousDB := model.DB
	previousLogDB := model.LOG_DB
	previousType := common.MainDatabaseType()
	previousLogType := common.LogDatabaseType()
	previousCache := common.MemoryCacheEnabled
	dsn := fmt.Sprintf("file:%s?mode=memory&cache=shared", strings.ReplaceAll(t.Name(), "/", "_"))
	db, err := gorm.Open(sqlite.Open(dsn), &gorm.Config{})
	require.NoError(t, err)
	require.NoError(t, db.AutoMigrate(
		&model.Option{},
		&model.ErrorReportSchedule{},
		&model.ErrorReport{},
		&model.ErrorReportPart{},
		&model.ErrorReportDelivery{},
		&model.ErrorEvent{},
		&model.ResourceAlertEpisode{},
		&model.ResourceAlertDelivery{},
		&model.SystemTask{},
		&model.SystemTaskLock{},
	))
	model.DB = db
	model.LOG_DB = db
	common.SetMainDatabaseType(common.DatabaseTypeSQLite)
	common.SetLogDatabaseType(common.DatabaseTypeSQLite)
	common.MemoryCacheEnabled = false
	t.Cleanup(func() {
		model.DB = previousDB
		model.LOG_DB = previousLogDB
		common.SetMainDatabaseType(previousType)
		common.SetLogDatabaseType(previousLogType)
		common.MemoryCacheEnabled = previousCache
	})
	return db
}

func withErrorReportSetting(t *testing.T, recipients string) {
	t.Helper()
	setting := operation_setting.GetErrorReportSetting()
	previous := setting.Recipients
	t.Cleanup(func() { setting.Recipients = previous })
	require.NoError(t, model.UpdateOption("error_report_setting.recipients", recipients))
}

type sentMail struct {
	subject string
	rcpt    string
	body    string
}

func claimErrorReportTask(t *testing.T, runnerID string) *model.SystemTask {
	t.Helper()
	task, err := model.CreateSystemTask(model.SystemTaskTypeErrorReport, nil, nil)
	require.NoError(t, err)
	claimedTask, claimed, err := model.ClaimSystemTask(task.ID, model.SystemTaskTypeErrorReport, runnerID, common.GetTimestamp()+60)
	require.NoError(t, err)
	require.True(t, claimed)
	return claimedTask
}

func seedCompletedDailyWindow(t *testing.T, ctx context.Context, start int64) string {
	t.Helper()
	id := fmt.Sprintf("daily-%d", start)
	report, err := model.LoadDailyReport(ctx, start, id)
	require.NoError(t, err)
	require.NoError(t, model.SaveDailyReportProgress(ctx, report, report.WindowEnd, `{"errors":{},"channels":{}}`, 0))
	return id
}

func publishTestDailyWindow(t *testing.T, h *errorReportHandler, start int64) string {
	t.Helper()
	ctx := context.Background()
	cursor, err := model.DailyReportCursor(ctx, start)
	require.NoError(t, err)
	require.Equal(t, start, cursor)
	id := seedCompletedDailyWindow(t, ctx, start)
	generated, _, err := h.generateDailyReport(ctx)
	require.NoError(t, err)
	require.Equal(t, 1, generated)
	return id
}
