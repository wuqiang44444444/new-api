package service

import (
	"net/http/httptest"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/model"
	relaycommon "github.com/QuantumNous/new-api/relay/common"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestMarkTaskCreateAttemptOutcomeUnknownPersistsUpstreamRequestID(t *testing.T) {
	truncate(t)
	attempt, err := model.CreatePreparedTaskAttempt(model.TaskCreateAttemptParams{
		PublicTaskID:   "task-unknown-request-id",
		UserID:         981,
		ClientProtocol: model.TaskClientProtocolModelArkV3,
		RequestHash:    "unknown-request-id",
	})
	require.NoError(t, err)
	transitioned, err := model.TransitionTaskCreateAttempt(
		nil,
		attempt.ID,
		model.TaskCreateAttemptPrepared,
		model.TaskCreateAttemptBillingUnheld,
		model.TaskCreateAttemptSending,
		model.TaskCreateAttemptBillingHeld,
		nil,
	)
	require.NoError(t, err)
	require.True(t, transitioned)

	context, _ := gin.CreateTestContext(httptest.NewRecorder())
	common.SetContextKey(context, constant.ContextKeyTaskCreateAttemptID, int(attempt.ID))
	common.SetContextKey(context, constant.ContextKeyTaskUpstreamStarted, true)
	context.Set(common.UpstreamRequestIdKey, " provider-request-981 ")
	info := &relaycommon.RelayInfo{}

	MarkTaskCreateAttemptOutcomeUnknown(context, info)

	require.NoError(t, model.DB.First(attempt, attempt.ID).Error)
	assert.Equal(t, model.TaskCreateAttemptUnknown, attempt.Status)
	assert.Equal(t, "provider-request-981", attempt.UpstreamRequestID)
	assert.True(t, info.SkipRequestRefund)
}

func TestMarkTaskCreateAttemptOutcomeUnknownDiscardsInvalidRequestIDWithoutLosingUnknownState(t *testing.T) {
	truncate(t)
	attempt, err := model.CreatePreparedTaskAttempt(model.TaskCreateAttemptParams{
		PublicTaskID:   "task-invalid-request-id",
		UserID:         982,
		ClientProtocol: model.TaskClientProtocolModelArkV3,
		RequestHash:    "invalid-request-id",
	})
	require.NoError(t, err)
	transitioned, err := model.TransitionTaskCreateAttempt(
		nil,
		attempt.ID,
		model.TaskCreateAttemptPrepared,
		model.TaskCreateAttemptBillingUnheld,
		model.TaskCreateAttemptSending,
		model.TaskCreateAttemptBillingHeld,
		nil,
	)
	require.NoError(t, err)
	require.True(t, transitioned)

	context, _ := gin.CreateTestContext(httptest.NewRecorder())
	common.SetContextKey(context, constant.ContextKeyTaskCreateAttemptID, int(attempt.ID))
	common.SetContextKey(context, constant.ContextKeyTaskUpstreamStarted, true)
	context.Set(common.UpstreamRequestIdKey, "provider\nrequest")

	MarkTaskCreateAttemptOutcomeUnknown(context, &relaycommon.RelayInfo{})

	require.NoError(t, model.DB.First(attempt, attempt.ID).Error)
	assert.Equal(t, model.TaskCreateAttemptUnknown, attempt.Status)
	assert.Empty(t, attempt.UpstreamRequestID)
}
