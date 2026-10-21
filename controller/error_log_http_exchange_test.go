package controller

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/QuantumNous/new-api/clienterrlog"
	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
	"github.com/gin-gonic/gin"
	"github.com/glebarez/sqlite"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

func TestErrorLogReadFormatsOnlyReturnedRowsWithoutRewritingStorage(t *testing.T) {
	previousDB, previousLogDB := model.DB, model.LOG_DB
	previousMain, previousLog := common.MainDatabaseType(), common.LogDatabaseType()
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	require.NoError(t, err)
	sqlDB, err := db.DB()
	require.NoError(t, err)
	t.Cleanup(func() {
		model.DB, model.LOG_DB = previousDB, previousLogDB
		common.SetDatabaseTypes(previousMain, previousLog)
		require.NoError(t, sqlDB.Close())
	})
	model.DB, model.LOG_DB = db, db
	common.SetDatabaseTypes(common.DatabaseTypeSQLite, common.DatabaseTypeSQLite)
	require.NoError(t, db.AutoMigrate(&model.ErrorEvent{}))
	exchange, err := common.Marshal(clienterrlog.HTTPExchange{
		Request: clienterrlog.HTTPBody{State: "captured", Body: `{"id":9007199254740993,"n":0,"stream":false}`},
	})
	require.NoError(t, err)
	detail, err := common.Marshal(map[string]string{clienterrlog.HTTPExchangeDetailKey: string(exchange), "upstream_status": "429"})
	require.NoError(t, err)
	rows := []model.ErrorEvent{
		{CreatedAt: 1, RequestId: "older", Module: "relay", Status: 429, Detail: string(detail)},
		{CreatedAt: 2, RequestId: "newer", Module: "relay", Status: 429, Detail: string(detail)},
	}
	require.NoError(t, db.Create(&rows).Error)
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = httptest.NewRequest(http.MethodGet, "/api/error_log/?page_size=1&p=1&status=429", nil)
	GetErrorLogs(c)
	require.Equal(t, http.StatusOK, w.Code)
	var response struct {
		Success bool `json:"success"`
		Data    struct {
			Total int                `json:"total"`
			Items []model.ErrorEvent `json:"items"`
		} `json:"data"`
	}
	require.NoError(t, common.Unmarshal(w.Body.Bytes(), &response))
	require.True(t, response.Success)
	assert.Equal(t, 2, response.Data.Total)
	require.Len(t, response.Data.Items, 1)
	assert.Equal(t, "newer", response.Data.Items[0].RequestId)
	var fields map[string]string
	require.NoError(t, common.UnmarshalJsonStr(response.Data.Items[0].Detail, &fields))
	assert.Equal(t, "429", fields["upstream_status"])
	var displayed clienterrlog.HTTPExchange
	require.NoError(t, common.UnmarshalJsonStr(fields[clienterrlog.HTTPExchangeDetailKey], &displayed))
	assert.Equal(t, "{\n  \"id\": 9007199254740993,\n  \"n\": 0,\n  \"stream\": false\n}", displayed.Request.Body)
	var stored []model.ErrorEvent
	require.NoError(t, db.Order("id").Find(&stored).Error)
	require.Len(t, stored, 2)
	for _, row := range stored {
		assert.Equal(t, string(detail), row.Detail)
	}
}
