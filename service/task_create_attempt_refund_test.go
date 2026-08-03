package service

import (
	"net/http/httptest"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
	relaycommon "github.com/QuantumNous/new-api/relay/common"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestTaskAttemptBillingSessionRefundUsesDurableRelease(t *testing.T) {
	truncate(t)
	user := model.User{Id: 9701, Username: "attempt-refund", Quota: 100}
	require.NoError(t, model.DB.Create(&user).Error)
	token := model.Token{
		Id:          9702,
		UserId:      user.Id,
		Key:         "attempt-refund-token",
		Status:      common.TokenStatusEnabled,
		RemainQuota: 100,
	}
	require.NoError(t, model.DB.Create(&token).Error)
	attempt, err := model.CreatePreparedTaskAttempt(model.TaskCreateAttemptParams{
		PublicTaskID:   "task-attempt-refund",
		UserID:         user.Id,
		TokenID:        token.Id,
		ClientProtocol: model.TaskClientProtocolModelArkV3,
		RequestHash:    "attempt-refund-request",
	})
	require.NoError(t, err)
	hold, err := model.HoldTaskCreateAttempt(model.TaskAttemptHoldParams{
		AttemptID:     attempt.ID,
		FundingSource: BillingSourceWallet,
		Quota:         20,
	})
	require.NoError(t, err)
	info := &relaycommon.RelayInfo{
		UserId:  user.Id,
		TokenId: token.Id,
	}
	session := billingSessionFromTaskAttempt(info, attempt, hold)
	context, _ := gin.CreateTestContext(httptest.NewRecorder())

	session.Refund(context)
	session.Refund(context)

	require.NoError(t, model.DB.First(attempt, attempt.ID).Error)
	assert.Equal(t, model.TaskCreateAttemptRejected, attempt.Status)
	assert.Equal(t, model.TaskCreateAttemptBillingReleased, attempt.BillingHoldState)
	require.NoError(t, model.DB.First(&user, user.Id).Error)
	require.NoError(t, model.DB.First(&token, token.Id).Error)
	assert.Equal(t, 100, user.Quota)
	assert.Equal(t, 100, token.RemainQuota)
	assert.Zero(t, token.UsedQuota)
}
