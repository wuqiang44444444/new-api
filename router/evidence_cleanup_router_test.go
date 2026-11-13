package router

import (
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/controller"
	"github.com/QuantumNous/new-api/middleware"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/setting/system_setting"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestEvidenceRetentionSettingsRequireRoot(t *testing.T) {
	setupRelayRouterTestDB(t)
	common.OptionMapRWMutex.Lock()
	previousOptions := common.OptionMap
	common.OptionMap = map[string]string{}
	common.OptionMapRWMutex.Unlock()
	t.Cleanup(func() {
		common.OptionMapRWMutex.Lock()
		common.OptionMap = previousOptions
		common.OptionMapRWMutex.Unlock()
	})
	require.NoError(t, model.UpdateOption(system_setting.TaskRequestEvidenceRetentionOptionKey, "7"))
	engine := gin.New()
	SetApiRouter(engine)

	for _, role := range []int{common.RoleAdminUser, common.RoleRootUser} {
		pat := "retention-role-" + strconv.Itoa(role)
		require.NoError(t, model.DB.Create(&model.User{
			Username: pat, Status: common.UserStatusEnabled, Role: role,
			Group: "default", AccessToken: &pat, AuthVersion: 1, AffCode: pat,
		}).Error)
		request := httptest.NewRequest(http.MethodGet, "/api/option/request_evidence_retention", nil)
		request.Header.Set("Authorization", "Bearer "+pat)
		response := httptest.NewRecorder()
		engine.ServeHTTP(response, request)
		write := httptest.NewRequest(http.MethodPut, "/api/option/", strings.NewReader(`{"key":"task_request_evidence.retention_days","value":3}`))
		write.Header.Set("Authorization", "Bearer "+pat)
		write.Header.Set("Content-Type", "application/json")
		written := httptest.NewRecorder()
		engine.ServeHTTP(written, write)
		if role == common.RoleAdminUser {
			assert.Equal(t, http.StatusForbidden, response.Code)
			assert.Equal(t, http.StatusForbidden, written.Code)
			assert.Equal(t, 7, system_setting.GetTaskRequestEvidenceRetentionPolicy().Days)
			continue
		}
		require.Equal(t, http.StatusOK, response.Code)
		var result struct {
			Success bool
			Data    system_setting.TaskRequestEvidenceRetentionPolicy
		}
		require.NoError(t, common.Unmarshal(response.Body.Bytes(), &result))
		assert.True(t, result.Success)
		assert.Equal(t, system_setting.TaskRequestEvidenceRetentionPolicy{Days: 7, Source: "database"}, result.Data)
		require.Equal(t, http.StatusOK, written.Code)
		require.NoError(t, common.Unmarshal(written.Body.Bytes(), &result))
		assert.True(t, result.Success)
		assert.Equal(t, 3, system_setting.GetTaskRequestEvidenceRetentionPolicy().Days)
	}
}

func TestEvidenceCleanupRouteRequiresRootAndFreezesAuthenticatedOperator(t *testing.T) {
	setupRelayRouterTestDB(t)
	require.NoError(t, model.DB.AutoMigrate(&model.SystemTask{}, &model.SystemTaskLock{}))
	engine := gin.New()
	group := engine.Group("/api/system-task")
	group.Use(middleware.RootAuth())
	group.POST("/evidence-cleanup", controller.CreateEvidenceCleanupSystemTask)
	for _, role := range []int{0, common.RoleCommonUser, common.RoleAdminUser, common.RoleRootUser} {
		t.Run(strconv.Itoa(role), func(t *testing.T) {
			pat := "cleanup-role-" + strconv.Itoa(role)
			user := &model.User{Username: pat, Status: common.UserStatusEnabled, Role: role, Group: "default", AccessToken: &pat, AuthVersion: 1, AffCode: pat}
			if role > 0 {
				require.NoError(t, model.DB.Create(user).Error)
			}
			body := `{"target_timestamp":` + strconv.FormatInt(common.GetTimestamp()-60, 10) + `,"operator_user_id":9999}`
			request := httptest.NewRequest(http.MethodPost, "/api/system-task/evidence-cleanup", strings.NewReader(body))
			request.Header.Set("Content-Type", "application/json")
			if role > 0 {
				request.Header.Set("Authorization", "Bearer "+pat)
			}
			response := httptest.NewRecorder()
			engine.ServeHTTP(response, request)
			if role == 0 {
				assert.Equal(t, http.StatusUnauthorized, response.Code)
				return
			}
			if role != common.RoleRootUser {
				assert.Equal(t, http.StatusForbidden, response.Code)
				return
			}
			require.Equal(t, http.StatusOK, response.Code)
			task, err := model.GetActiveSystemTask(model.SystemTaskTypeTaskRequestEvidenceRetention)
			require.NoError(t, err)
			require.NotNil(t, task)
			var payload struct {
				OperatorUserID int `json:"operator_user_id"`
			}
			require.NoError(t, task.DecodePayload(&payload))
			assert.Equal(t, user.Id, payload.OperatorUserID)
			get := httptest.NewRecorder()
			engine.ServeHTTP(get, httptest.NewRequest(http.MethodGet, "/api/system-task/evidence-cleanup", nil))
			assert.Equal(t, http.StatusNotFound, get.Code)
		})
	}
}
