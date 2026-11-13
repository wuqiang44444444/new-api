package controller

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestDrawingLogChannelVisibility(t *testing.T) {
	db, _ := newAuditTestDatabase(t, "sqlite", "")
	useLogExportDatabases(t, db, db, common.DatabaseTypeSQLite, common.DatabaseTypeSQLite)
	require.NoError(t, db.AutoMigrate(&model.Midjourney{}))
	require.NoError(t, db.Create(&model.Midjourney{UserId: 1, MjId: "drawing-1", ChannelId: 9, Quota: 27}).Error)
	require.NoError(t, db.Create(&model.Midjourney{UserId: 2, MjId: "drawing-2", ChannelId: 9}).Error)

	for _, tc := range []struct {
		name    string
		handler gin.HandlerFunc
		role    int
		channel int
	}{
		{"personal", GetUserMidjourney, common.RoleCommonUser, 0},
		{"admin", GetAllMidjourney, common.RoleAdminUser, 9},
	} {
		t.Run(tc.name, func(t *testing.T) {
			response := callLogHandler(t, tc.handler, 1, tc.role, "mj_id=drawing-1")[0]
			require.Equal(t, true, response["success"])
			data := response["data"].(map[string]any)
			assert.EqualValues(t, 1, data["total"])
			items := data["items"].([]any)
			require.Len(t, items, 1)
			row := items[0].(map[string]any)
			assert.EqualValues(t, tc.channel, row["channel_id"])
			assert.EqualValues(t, 27, row["quota"])
		})
	}
}

func TestHistoricalExportJobHidesOperationalFiltersFromOrdinaryUser(t *testing.T) {
	db, _ := newAuditTestDatabase(t, "sqlite", "")
	useLogExportDatabases(t, db, db, common.DatabaseTypeSQLite, common.DatabaseTypeSQLite)
	require.NoError(t, db.AutoMigrate(&model.User{}, &model.CustomerExportJob{}))
	for _, role := range []int{common.RoleCommonUser, common.RoleAdminUser} {
		user := model.User{Id: role, Role: role, Status: common.UserStatusEnabled}
		if role == common.RoleAdminUser {
			user.Username, user.AffCode = "admin", "admin"
		}
		require.NoError(t, db.Create(&user).Error)
	}
	filters := `{"channel_id":9,"upstream_request_id":"provider-1","request_id":"platform-1"}`
	require.NoError(t, db.Create(&[]model.CustomerExportJob{
		{JobID: "legacy-own", UserId: 1, TargetUserId: 1, JobType: model.CustomerExportJobTypeUsageLogs, Status: model.CustomerExportJobStatusSucceeded, Filters: filters},
		{JobID: "legacy-admin", UserId: 10, TargetUserId: 1, JobType: model.CustomerExportJobTypeUsageLogs, Status: model.CustomerExportJobStatusSucceeded, Filters: filters},
	}).Error)

	for _, tc := range []struct {
		name  string
		role  int
		jobID string
	}{
		{"personal", common.RoleCommonUser, "legacy-own"},
		{"admin", common.RoleAdminUser, "legacy-admin"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			router := gin.New()
			router.Use(func(c *gin.Context) {
				c.Set("id", tc.role)
				c.Set("role", tc.role)
			})
			router.GET("/exports", ListSelfCustomerExports)
			router.GET("/exports/:job_id", GetSelfCustomerExport)
			for _, path := range []string{"/exports", "/exports/" + tc.jobID} {
				rec := httptest.NewRecorder()
				router.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, path, nil))
				require.Equal(t, http.StatusOK, rec.Code)
				var response struct {
					Success bool
					Data    map[string]any
				}
				require.NoError(t, common.Unmarshal(rec.Body.Bytes(), &response))
				require.True(t, response.Success)
				job := response.Data
				if path == "/exports" {
					items := job["items"].([]any)
					require.Len(t, items, 1)
					job = items[0].(map[string]any)
				}
				view := job["filters"].(map[string]any)
				assert.Equal(t, "platform-1", view["request_id"])
				if tc.role == common.RoleAdminUser {
					assert.EqualValues(t, 9, view["channel_id"])
					assert.Equal(t, "provider-1", view["upstream_request_id"])
				} else {
					assert.NotContains(t, view, "channel_id")
					assert.NotContains(t, view, "upstream_request_id")
				}
			}
		})
	}
	var stored model.CustomerExportJob
	require.NoError(t, db.Where("job_id = ?", "legacy-own").First(&stored).Error)
	assert.Equal(t, filters, stored.Filters)
}
