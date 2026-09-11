package controller

import (
	"net/http"
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

func TestBillingStatementLogAPIUsesAuthenticatedCustomerAndExactZeroKey(t *testing.T) {
	previousDB, previousLogDB := model.DB, model.LOG_DB
	previousMain, previousLog := common.MainDatabaseType(), common.LogDatabaseType()
	db, err := gorm.Open(sqlite.Open(filepath.Join(t.TempDir(), "billing.db")), &gorm.Config{})
	require.NoError(t, err)
	sqlDB, err := db.DB()
	require.NoError(t, err)
	t.Cleanup(func() {
		model.DB, model.LOG_DB = previousDB, previousLogDB
		common.SetDatabaseTypes(previousMain, previousLog)
		_ = sqlDB.Close()
	})
	model.DB, model.LOG_DB = db, db
	common.SetDatabaseTypes(common.DatabaseTypeSQLite, common.DatabaseTypeSQLite)
	require.NoError(t, db.AutoMigrate(&model.Log{}, &model.Channel{}))
	logs := []model.Log{
		{UserId: 1, Quota: 100, TokenName: "playground-default"},
		{UserId: 1, Quota: 30, Type: model.LogTypeRefund},
		{UserId: 1, Quota: 900, TokenName: "模型测试", Content: "模型测试"},
		{UserId: 1, Quota: 800, TokenId: 90},
		{UserId: 2, Quota: 700},
	}
	for i := range logs {
		logs[i].CreatedAt = 1788192100
		logs[i].ModelName = "model"
		logs[i].Other = `{"model_ratio":1,"group_ratio":1}`
		if logs[i].Type == 0 {
			logs[i].Type = model.LogTypeConsume
		}
	}
	require.NoError(t, db.Create(&logs).Error)
	gin.SetMode(gin.TestMode)
	router := gin.New()
	router.Use(func(c *gin.Context) { c.Set("id", 1); c.Set("role", common.RoleRootUser) })
	router.GET("/self", GetSelfBillingStatementLogs)
	router.GET("/self/stat", GetSelfBillingStatementLogs)
	router.GET("/admin", GetAdminBillingStatementLogs)
	query := "?start_timestamp=1788192000&end_timestamp=1790783999&token_id=0&model_name=model&billing_mode=token"
	for _, path := range []string{"/self", "/self/stat", "/admin"} {
		recorder := httptest.NewRecorder()
		user := "2"
		if path == "/admin" {
			user = "1"
		}
		router.ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, path+query+"&user_id="+user, nil))
		require.Equal(t, http.StatusOK, recorder.Code, recorder.Body.String())
		var response struct {
			Success bool
			Data    model.BillingStatementLogs
		}
		require.NoError(t, common.Unmarshal(recorder.Body.Bytes(), &response))
		require.True(t, response.Success, recorder.Body.String())
		assert.EqualValues(t, 70, response.Data.Quota)
		if path != "/self/stat" {
			assert.EqualValues(t, 2, response.Data.Total)
			require.Len(t, response.Data.Items, 2)
			for _, log := range response.Data.Items {
				assert.Equal(t, 1, log.UserId)
				assert.Zero(t, log.TokenId)
			}
		}
	}
	for _, suffix := range []string{"&token_id=-1", "&token_id=0.5", "&token_id=oops", "&channel=-1", "&p=-1", "&page_size=-1"} {
		// Replace rather than duplicate the key so the parser receives the invalid value.
		recorder := httptest.NewRecorder()
		invalid := "?start_timestamp=1788192000&end_timestamp=1790783999" + suffix
		router.ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, "/self"+invalid, nil))
		assert.Equal(t, http.StatusBadRequest, recorder.Code, suffix)
	}
	recorder := httptest.NewRecorder()
	router.ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, "/admin"+query, nil))
	assert.Equal(t, http.StatusBadRequest, recorder.Code)
}
