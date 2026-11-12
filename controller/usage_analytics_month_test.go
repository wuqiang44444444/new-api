package controller

import (
	"net/http/httptest"
	"path/filepath"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
	"github.com/gin-gonic/gin"
	"github.com/glebarez/sqlite"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

func TestUsageAdminUpstreamSummaryAcceptsNaturalMonth(t *testing.T) {
	database, err := gorm.Open(sqlite.Open(filepath.Join(t.TempDir(), "usage.db")), &gorm.Config{})
	require.NoError(t, err)
	sqlDB, err := database.DB()
	require.NoError(t, err)
	previousDB := model.DB
	model.DB = database
	t.Cleanup(func() {
		model.DB = previousDB
		require.NoError(t, sqlDB.Close())
	})
	require.NoError(t, model.MigrateDailyReports(database))

	recorder := httptest.NewRecorder()
	ctx, _ := gin.CreateTestContext(recorder)
	ctx.Request = httptest.NewRequest("GET", "/api/usage/admin/upstream-summary?period=month&date=2024-02-19", nil)
	GetUsageAdminUpstreamSummary(ctx)
	var response struct {
		Success bool   `json:"success"`
		Message string `json:"message"`
		Data    struct {
			Period model.UsageAnalyticsPeriod `json:"period"`
		} `json:"data"`
	}
	require.NoError(t, common.Unmarshal(recorder.Body.Bytes(), &response))
	require.True(t, response.Success, response.Message)
	period := response.Data.Period
	assert.Equal(t, "month", period.Period)
	assert.Equal(t, "Asia/Shanghai", period.Timezone)
	require.Len(t, period.Days, 29)
	assert.Equal(t, "2024-02-01", period.Days[0].Date)
	assert.Equal(t, "2024-02-29", period.Days[28].Date)
}
