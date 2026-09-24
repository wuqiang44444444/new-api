package router

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/service"
	"github.com/QuantumNous/new-api/service/authz"
	"github.com/gin-gonic/gin"
	"github.com/glebarez/sqlite"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

func TestUpstreamBalanceRoutesRejectUnauthenticatedRequests(t *testing.T) {
	gin.SetMode(gin.TestMode)
	engine := gin.New()
	registerUpstreamBalanceRoutes(engine.Group("/api"))
	for _, path := range []string{"/api/upstream-balances/", "/api/upstream-balances/1/0"} {
		recorder := httptest.NewRecorder()
		engine.ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, path, nil))
		require.Equal(t, http.StatusUnauthorized, recorder.Code)
	}
}

func TestUpstreamBalanceRoutesEnforceRoleAndPermissionsWithoutChangingChannels(t *testing.T) {
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	require.NoError(t, err)
	sqlDB, err := db.DB()
	require.NoError(t, err)
	sqlDB.SetMaxOpenConns(1)
	require.NoError(t, db.AutoMigrate(&model.User{}, &model.Channel{}, &model.AuditLog{}, &model.CasbinRule{}, &model.AuthzRole{}, &model.ProviderURLGroupDisplayName{}, &model.ProviderBillingAudit{}))
	previousDB, previousLogDB, previousMaster := model.DB, model.LOG_DB, common.IsMasterNode
	previousRedis, previousType := common.RedisEnabled, common.MainDatabaseType()
	model.DB, model.LOG_DB, common.IsMasterNode = db, db, true
	common.RedisEnabled = false
	common.SetMainDatabaseType(common.DatabaseTypeSQLite)
	t.Cleanup(func() {
		model.DB, model.LOG_DB, common.IsMasterNode = previousDB, previousLogDB, previousMaster
		common.RedisEnabled = previousRedis
		common.SetMainDatabaseType(previousType)
		if previousDB != nil {
			require.NoError(t, authz.Init(previousDB))
		}
		require.NoError(t, sqlDB.Close())
	})
	require.NoError(t, authz.Init(db))
	base := "https://unconnected.example"
	channel := model.Channel{Name: "disabled balance channel", Type: 1, BaseURL: &base, Key: "sensitive-channel-key", Status: 2, Balance: 12.5, BalanceUpdatedTime: 123}
	require.NoError(t, db.Create(&channel).Error)
	require.NoError(t, model.SaveProviderURLGroupName(base, "Shared reconciliation name", 1))
	engine := gin.New()
	registerUpstreamBalanceRoutes(engine.Group("/api"))
	for _, test := range []struct {
		name     string
		role     int
		deny     string
		expected int
	}{
		{"customer", common.RoleCommonUser, "", http.StatusForbidden},
		{"admin", common.RoleAdminUser, "", http.StatusOK},
		{"no-read", common.RoleAdminUser, authz.ActionRead, http.StatusForbidden},
		{"no-operate", common.RoleAdminUser, authz.ActionOperate, http.StatusForbidden},
	} {
		t.Run(test.name, func(t *testing.T) {
			token := "test-pat-" + test.name
			user := model.User{Username: test.name, Password: "test", Role: test.role, Status: common.UserStatusEnabled, Group: "default", AccessToken: &token, AuthVersion: 1, AffCode: test.name}
			require.NoError(t, db.Create(&user).Error)
			if test.deny != "" {
				require.NoError(t, authz.SetUserPermissions(user.Id, authz.PermissionsMap{authz.ResourceChannel: {test.deny: false}}))
			}
			rows := service.UpstreamBalanceInventory([]*model.Channel{&channel})
			for _, path := range []string{"/api/upstream-balances/", "/api/upstream-balances/1/0?reference=" + rows[0].ID} {
				recorder := httptest.NewRecorder()
				request := httptest.NewRequest(http.MethodGet, path, nil)
				request.Header.Set("Authorization", "Bearer "+token)
				engine.ServeHTTP(recorder, request)
				require.Equal(t, test.expected, recorder.Code, recorder.Body.String())
				assert.NotContains(t, recorder.Body.String(), channel.Key)
				if test.expected == http.StatusOK && path == "/api/upstream-balances/" {
					var body struct {
						Data []service.UpstreamBalanceConnection `json:"data"`
					}
					require.NoError(t, common.Unmarshal(recorder.Body.Bytes(), &body))
					require.Len(t, body.Data, 1)
					assert.Equal(t, base, body.Data[0].URLKey)
					assert.Equal(t, "Shared reconciliation name", body.Data[0].GroupName)
				}
			}
		})
	}
	var after model.Channel
	require.NoError(t, db.First(&after, channel.Id).Error)
	assert.Equal(t, channel.Key, after.Key)
	assert.Equal(t, channel.Status, after.Status)
	assert.Equal(t, channel.Balance, after.Balance)
	assert.Equal(t, channel.BalanceUpdatedTime, after.BalanceUpdatedTime)
}
