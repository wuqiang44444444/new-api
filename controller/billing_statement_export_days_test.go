package controller

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/setting/system_setting"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestAdminCustomerStatementExportDaysOffersPriorMonthWhenCurrentHasNoCompleteDay(t *testing.T) {
	db := setupBillingURLNameControllerTest(t)
	require.NoError(t, db.AutoMigrate(&model.Task{}, &model.TaskBillingDelivery{}, &model.SystemTask{}, &model.SystemTaskLock{}))
	now := time.Now().In(billingSettlementLocation)
	month := time.Date(now.Year(), now.Month(), 1, 0, 0, 0, 0, billingSettlementLocation)
	previous := month.AddDate(0, -1, 0)
	require.NoError(t, db.Create(&model.Log{Type: model.LogTypeConsume, UserId: 7, CreatedAt: previous.Unix() + 1, Quota: 50}).Error)
	require.NoError(t, db.Create(&model.SystemTaskLock{
		Type: model.UsageReportTaskType, TaskID: "export-days", LockedBy: "runner", LockedUntil: common.GetTimestamp() + 60,
	}).Error)
	require.NoError(t, model.BuildBillingReportDay(context.Background(), previous.Unix(), "export-days", "runner", nil, nil))

	router := newBillingAdminTestRouter()
	router.GET("/days", GetAdminCustomerStatementExportDays)
	recorder := httptest.NewRecorder()
	path := fmt.Sprintf("/days?start_timestamp=%d&end_timestamp=%d", month.Unix(), month.AddDate(0, 1, 0).Unix()-1)
	router.ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, path, nil))

	require.Equal(t, http.StatusOK, recorder.Code)
	var response struct {
		Success bool                                  `json:"success"`
		Data    model.CustomerStatementExportCalendar `json:"data"`
	}
	require.NoError(t, common.Unmarshal(recorder.Body.Bytes(), &response))
	require.True(t, response.Success, recorder.Body.String())
	require.NotEmpty(t, response.Data.Days)
	assert.Equal(t, now.Format(time.DateOnly), response.Data.Days[0].Date)
	assert.Equal(t, "in_progress", response.Data.Days[0].Status)
	assert.Empty(t, response.Data.Days[0].ReportVersion)
	assert.Equal(t, previous.Format(time.DateOnly), response.Data.DefaultDate)
	day := response.Data.Days[len(response.Data.Days)-1]
	assert.Equal(t, response.Data.DefaultDate, day.Date)
	assert.Equal(t, "complete", day.Status)
	assert.NotEmpty(t, day.ReportVersion)
}

func TestAdminFrozenStatementExportsWithoutCurrentCustomerOrLogBackend(t *testing.T) {
	db := setupBillingURLNameControllerTest(t)
	require.NoError(t, db.AutoMigrate(&model.User{}, &model.BillingStatementVersion{}, &model.BillingStatementVersionLine{},
		&model.CustomerExportJob{}, &model.CustomerExportSlot{}, &model.SystemTask{}, &model.SystemTaskLock{}))
	require.NoError(t, db.Create(&model.User{Id: 9, Username: "billing-admin", AffCode: "badmin",
		Role: common.RoleRootUser, Status: common.UserStatusEnabled}).Error)
	require.NoError(t, db.Create(&model.CustomerExportSlot{ID: 1}).Error)
	config, err := common.Marshal(system_setting.ObjectStorageConfig{
		Backend: system_setting.ObjectStorageBackendS3, Endpoint: "https://storage.example.test", Bucket: "fixture-exports",
		Region: "us-east-1", AccountName: "fixture-account", Credential: "fixture-secret", Revision: "frozen-export-test",
	})
	require.NoError(t, err)
	model.NotifyObjectStorageSettingUpdate(string(config))
	t.Cleanup(func() { model.NotifyObjectStorageSettingUpdate("") })

	start := time.Date(2026, 9, 1, 0, 0, 0, 0, billingSettlementLocation)
	version := model.BillingStatementVersion{
		UserId: 119, DraftPublicId: "retained-statement",
		PeriodStart: start.Unix(), PeriodEndExclusive: start.AddDate(0, 1, 0).Unix(),
		Status: model.BillingStatementVersionConfirmed, Currency: "USD", CurrencyRate: 1, QuotaPerUnit: 100,
	}
	require.NoError(t, db.Create(&version).Error)
	require.NoError(t, db.Create(&model.BillingStatementVersionLine{
		VersionId: version.ID, CreatedAt: start.Unix() + 1, LogType: model.LogTypeConsume, Quota: 12,
	}).Error)
	common.SetDatabaseTypes(common.DatabaseTypeSQLite, common.DatabaseTypeClickHouse)

	router := newBillingAdminTestRouter()
	router.GET("/days", GetAdminCustomerStatementDetailExportDays)
	router.POST("/exports", CreateAdminCustomerExport)
	path := fmt.Sprintf("/days?user_id=%d&start_timestamp=%d&end_timestamp=%d&statement_version=%s",
		version.UserId, version.PeriodStart, version.PeriodEndExclusive-1, version.DraftPublicId)
	calendar := httptest.NewRecorder()
	router.ServeHTTP(calendar, httptest.NewRequest(http.MethodGet, path, nil))
	require.Equal(t, http.StatusOK, calendar.Code)
	require.Contains(t, calendar.Body.String(), "2026-09-01")

	body := fmt.Sprintf(`{"job_type":"statement_details","statement_version":"%s","start_timestamp":%d,"end_timestamp":%d,"language":"en"}`,
		version.DraftPublicId, version.PeriodStart, start.AddDate(0, 0, 1).Unix())
	request := httptest.NewRequest(http.MethodPost, fmt.Sprintf("/exports?user_id=%d", version.UserId), strings.NewReader(body))
	request.Header.Set("Content-Type", "application/json")
	completed := httptest.NewRecorder()
	router.ServeHTTP(completed, request)
	require.Equal(t, http.StatusAccepted, completed.Code, completed.Body.String())
	var response struct {
		Data model.CustomerExportJobView `json:"data"`
	}
	require.NoError(t, common.Unmarshal(completed.Body.Bytes(), &response))
	assert.Equal(t, version.UserId, response.Data.TargetUserId)
	assert.Equal(t, version.DraftPublicId, response.Data.Filters.StatementDraftId)
	assert.Equal(t, "zh", response.Data.Filters.Language)
}
