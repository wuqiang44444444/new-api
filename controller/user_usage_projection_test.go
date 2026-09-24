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

func TestUsageStartupQueriesReturnCurrentProjection(t *testing.T) {
	db := setupManageUserTestDB(t)
	require.NoError(t, db.AutoMigrate(&model.Task{}, &model.TaskCreateAttempt{}, &model.TaskBillingDelivery{}, &model.QuotaData{}, &model.Channel{}, &model.CustomerContract{}))
	require.NoError(t, db.Create(&model.User{Id: 91, Username: "usage-user", UsedQuota: 100, Quota: 300, RequestCount: 1, Role: common.RoleCommonUser, AffCode: "usage91"}).Error)
	require.NoError(t, db.Create(&model.Log{UserId: 91, Type: model.LogTypeConsume, Quota: 100, RequestId: "usage-create", Other: "{}"}).Error)
	require.NoError(t, db.Create(&model.Log{UserId: 91, Type: model.LogTypeRefund, Quota: 80, RequestId: "usage-refund", Other: "{}"}).Error)
	require.NoError(t, model.InitUserUsageRepair())
	// The database and existing read APIs continue to carry the normal increment.
	require.NoError(t, db.Model(&model.User{}).Where("id = ?", 91).UpdateColumn("used_quota", 27).Error)
	for _, entry := range []struct {
		name, path string
		handler    gin.HandlerFunc
		list       bool
	}{
		{"self", "/api/user/self", GetSelf, false},
		{"detail", "/api/user/91", GetUser, false},
		{"list", "/api/user/?p=1&size=10", GetAllUsers, true},
		{"search", "/api/user/search?keyword=usage-user&p=1&size=10", SearchUsers, true},
	} {
		t.Run(entry.name, func(t *testing.T) {
			recorder := httptest.NewRecorder()
			ctx, _ := gin.CreateTestContext(recorder)
			ctx.Request = httptest.NewRequest(http.MethodGet, entry.path, nil)
			ctx.Set("id", 91)
			ctx.Set("role", common.RoleRootUser)
			ctx.Params = gin.Params{{Key: "id", Value: "91"}}
			entry.handler(ctx)
			var response struct {
				Success bool           `json:"success"`
				Data    map[string]any `json:"data"`
			}
			require.NoError(t, common.Unmarshal(recorder.Body.Bytes(), &response))
			require.True(t, response.Success, recorder.Body.String())
			data := response.Data
			if entry.list {
				items, ok := data["items"].([]any)
				require.True(t, ok)
				require.Len(t, items, 1)
				data = items[0].(map[string]any)
			}
			assert.Equal(t, float64(27), data["used_quota"])
			assert.Equal(t, float64(300), data["quota"])
			assert.Contains(t, recorder.Header().Get("Cache-Control"), "no-store")
		})
	}
	user, err := model.GetSelfUserById(91)
	require.NoError(t, err)
	payload, err := common.Marshal(buildSelfUserData(user))
	require.NoError(t, err)
	var loginData map[string]any
	require.NoError(t, common.Unmarshal(payload, &loginData))
	assert.Equal(t, float64(27), loginData["used_quota"])
}
