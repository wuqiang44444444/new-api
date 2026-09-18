package controller

import (
	"context"
	"fmt"
	"net/http/httptest"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
	"github.com/gin-gonic/gin"
	"github.com/glebarez/sqlite"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

func TestBillingStatementVersionReadFailureNeverFallsBack(t *testing.T) {
	old := model.DB
	db, err := gorm.Open(sqlite.Open("file:"+t.Name()+"?mode=memory&cache=shared"), &gorm.Config{})
	require.NoError(t, err)
	model.DB = db
	t.Cleanup(func() { model.DB = old; raw, _ := db.DB(); _ = raw.Close() })
	require.NoError(t, db.AutoMigrate(&model.BillingStatementMonth{}))
	id := int64(99)
	require.NoError(t, db.Create(&model.BillingStatementMonth{UserId: 11, PeriodStart: 100, CurrentVersionId: &id}).Error)
	recorder := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(recorder)
	c.Request = httptest.NewRequest("GET", "/", nil)
	// 版本存储读失败，绝不能返回 false 让调用方改查实时账单。
	assert.True(t, tryRespondBillingStatementVersion(c, billingReconciliationPeriod{StartTimestamp: 100}, 11, "api_key", false))
	var response map[string]interface{}
	require.NoError(t, common.Unmarshal(recorder.Body.Bytes(), &response))
	assert.Equal(t, false, response["success"])
}

func TestBillingStatementLineWirePreservesIntegerAndHidesChannel(t *testing.T) {
	facts, err := common.Marshal(model.CustomerExportRow{ContractApplicable: "unknown", InputTokensUnavailable: true})
	require.NoError(t, err)
	views, err := billingStatementLineViews([]model.BillingStatementVersionLine{{ID: 1, ChannelId: 998, InputTokens: 9007199254740993, Quota: 17, LogType: model.LogTypeRefund, Facts: string(facts)}})
	require.NoError(t, err)
	require.Len(t, views, 1)
	assert.Equal(t, "9007199254740993", views[0]["input_tokens"])
	assert.Equal(t, "-17", views[0]["quota"])
	assert.NotContains(t, views[0], "channel_id")
	assert.NotContains(t, views[0], "source_log_id")
	_, err = billingStatementLineViews([]model.BillingStatementVersionLine{{Facts: "broken"}})
	assert.Error(t, err)
}

func TestBillingStatementFrozenReadSupportsZeroKeyAndChannel(t *testing.T) {
	old := model.DB
	db, err := gorm.Open(sqlite.Open("file:"+t.Name()+"?mode=memory&cache=shared"), &gorm.Config{})
	require.NoError(t, err)
	model.DB = db
	t.Cleanup(func() { model.DB = old; raw, _ := db.DB(); _ = raw.Close() })
	require.NoError(t, db.AutoMigrate(&model.User{}, &model.BillingStatementVersion{}))
	statement := model.BillingCustomerStatement{UserId: 11, Dimension: "api_key", Groups: []model.BillingReconciliationGroupSummary{{Id: 0, Models: []model.BillingReconciliationModelSummary{{ModelName: "one", BillingMode: "token", Usage: model.BillingReconciliationUsage{GrossQuota: 20, NetQuota: 20}}}}, {Id: 7, Models: []model.BillingReconciliationModelSummary{{ModelName: "two", BillingMode: "token", Usage: model.BillingReconciliationUsage{GrossQuota: 30, NetQuota: 30}}}}}}
	snapshot, err := model.FreezeBillingStatementProjection(statement)
	require.NoError(t, err)
	version := model.BillingStatementVersion{UserId: 11, DraftPublicId: "bsv_read_test", PeriodStart: 100, Status: model.BillingStatementVersionConfirmed, SummaryProjection: snapshot, ChannelProjection: snapshot, QuotaPerUnit: 1000000, Currency: "USD", CurrencyRate: 1}
	require.NoError(t, db.Create(&version).Error)
	for _, dimension := range []string{"api_key", "channel"} {
		recorder := httptest.NewRecorder()
		c, _ := gin.CreateTestContext(recorder)
		c.Request = httptest.NewRequest("GET", fmt.Sprintf("/?version=bsv_read_test&group_id=0&dimension=%s", dimension), nil)
		require.True(t, tryRespondBillingStatementVersion(c, billingReconciliationPeriod{StartTimestamp: 100}, 11, dimension, true))
		var response struct {
			Success bool `json:"success"`
			Data    struct {
				Result model.BillingCustomerStatement `json:"result"`
			} `json:"data"`
		}
		require.NoError(t, common.Unmarshal(recorder.Body.Bytes(), &response))
		require.True(t, response.Success, recorder.Body.String())
		require.Len(t, response.Data.Result.Groups, 1)
		assert.EqualValues(t, 0, response.Data.Result.Groups[0].Id)
		assert.EqualValues(t, 20, response.Data.Result.Summary.NetQuota)
	}
	_, err = model.GetBillingStatementVersion(context.Background(), 999)
	assert.Error(t, err)
}
