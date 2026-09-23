package service

import (
	"errors"
	"github.com/QuantumNous/new-api/model"
	"github.com/stretchr/testify/require"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/relaykit/types"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
)

func TestChannelStatusAuditFreezesRequestAndProbeFacts(t *testing.T) {
	c, _ := gin.CreateTestContext(httptest.NewRecorder())
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/chat/completions", nil)
	c.Set(common.RequestIdKey, "request-original")
	upstreamErr := types.NewOpenAIError(errors.New("Bearer fixture-secret"), types.ErrorCodeBadResponse, http.StatusUnauthorized)
	requestAudit := ChannelStatusAuditForRequest(c, upstreamErr)
	c.Set(common.RequestIdKey, "request-reused")
	assert.Equal(t, "request-original", requestAudit.RequestID)
	assert.Equal(t, "relay_error", requestAudit.Trigger)
	PrepareChannelTestAudit(c)
	assert.Equal(t, "channel_test_failed", ChannelStatusAuditForRequest(c, upstreamErr).Trigger)
	thresholdErr := types.NewOpenAIError(errors.New("private threshold message"), types.ErrorCodeChannelResponseTimeExceeded, http.StatusRequestTimeout)
	assert.Equal(t, "response_time_exceeded", ChannelStatusAuditForRequest(c, thresholdErr).Trigger)
	assert.Equal(t, "channel_test_recovered", ChannelStatusAuditForRequest(c, nil).Trigger)
	firstID := ChannelStatusAuditForRequest(c, nil).RequestID
	PrepareChannelTestAudit(c)
	assert.Equal(t, firstID, ChannelStatusAuditForRequest(c, nil).RequestID)
}

func TestChannelTestEventAndStatusAuditShareRequestID(t *testing.T) {
	db := withAssetGroupPolicyDB(t)
	require.NoError(t, db.AutoMigrate(&model.ErrorEvent{}, &model.AuditLog{}, &model.User{}, &model.Ability{}))
	previousLogDB, previousCache := model.LOG_DB, common.MemoryCacheEnabled
	model.LOG_DB = db
	common.MemoryCacheEnabled = false
	t.Cleanup(func() { model.LOG_DB = previousLogDB; common.MemoryCacheEnabled = previousCache })
	channel := model.Channel{Status: common.ChannelStatusEnabled, Key: "fixture"}
	require.NoError(t, db.Create(&channel).Error)
	c, _ := gin.CreateTestContext(httptest.NewRecorder())
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/chat/completions", nil)
	PrepareChannelTestAudit(c)
	apiError := types.NewOpenAIError(errors.New("private provider message"), types.ErrorCodeBadResponse, http.StatusUnauthorized)
	observation := ChannelStatusAuditForRequest(c, apiError)
	RecordAutoChannelTestFailureEvent(&channel, "model-a", c, nil, apiError, 401, 1, false, false, nil)
	require.True(t, model.UpdateChannelStatus(channel.Id, "", common.ChannelStatusAutoDisabled, "private provider message", observation))
	require.Eventually(t, func() bool {
		var count int64
		return db.Model(&model.ErrorEvent{}).Where("request_id = ?", observation.RequestID).Count(&count).Error == nil && count == 1
	}, time.Second, time.Millisecond)
	var audit model.AuditLog
	require.NoError(t, db.Where("request_id = ?", observation.RequestID).First(&audit).Error)
	assert.Equal(t, "channel_status_change", audit.Action)
	assert.Equal(t, observation.RequestID, audit.RequestId)
}
