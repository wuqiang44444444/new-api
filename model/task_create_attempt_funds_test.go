package model

import (
	"testing"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func createFundsTestUser(t *testing.T, id int, quota int) *User {
	t.Helper()
	user := User{Id: id, Username: "funds-user", Quota: 0}
	user.Quota = quota
	require.NoError(t, DB.Create(&user).Error)
	return &user
}

func createFundsTestToken(t *testing.T, userID int, key string, remain int) *Token {
	t.Helper()
	token := Token{
		UserId: userID, Key: key,
		Status: common.TokenStatusEnabled, RemainQuota: remain,
	}
	require.NoError(t, DB.Create(&token).Error)
	return &token
}

func createHeldTestAttempt(t *testing.T, userID, tokenID int, suffix string, quota int, source string) *TaskCreateAttempt {
	t.Helper()
	attempt, err := CreatePreparedTaskAttempt(TaskCreateAttemptParams{
		PublicTaskID:   GenerateTaskID(),
		UserID:         userID,
		TokenID:        tokenID,
		ClientProtocol: TaskClientProtocolModelArkV3,
		RequestHash:    "funds-" + suffix,
	})
	require.NoError(t, err)
	holdParams := TaskAttemptHoldParams{
		AttemptID: attempt.ID, FundingSource: source, Quota: quota,
	}
	if source == "subscription" {
		holdParams.ModelName = "test-model"
	}
	_, err = HoldTaskCreateAttempt(holdParams)
	require.NoError(t, err)
	return attempt
}

func setTestAttemptUnknown(t *testing.T, attemptID int64) {
	t.Helper()
	require.NoError(t, DB.Model(&TaskCreateAttempt{}).
		Where("id = ?", attemptID).
		Updates(map[string]any{
			"status":     TaskCreateAttemptUnknown,
			"updated_at": common.GetTimestamp(),
		}).Error)
}

func expireTestAttemptDeadline(t *testing.T, attemptID int64) {
	t.Helper()
	require.NoError(t, DB.Model(&TaskCreateAttempt{}).
		Where("id = ?", attemptID).
		Updates(map[string]any{"funds_deadline_at": time.Now().Unix() - 60}).Error)
}

func TestTaskCreateAttemptWarrantyRefundReleasesWalletHoldAfterDeadline(t *testing.T) {
	truncateTables(t)
	user := createFundsTestUser(t, 1501, 100)
	token := createFundsTestToken(t, user.Id, "funds-token-deadline", 50)
	attempt := createHeldTestAttempt(t, user.Id, token.Id, "deadline", 25, "wallet")
	setTestAttemptUnknown(t, attempt.ID)

	// 期限前：不属于资金待办，也不被释放。
	now := time.Now().Unix()
	assert.False(t, HasTaskCreateAttemptFundWork(now))
	empty, scanErr := GetTaskCreateAttemptFundDebts(now, 10)
	require.NoError(t, scanErr)
	assert.Empty(t, empty)

	expireTestAttemptDeadline(t, attempt.ID)
	assert.True(t, HasTaskCreateAttemptFundWork(now))
	debts, scanErr := GetTaskCreateAttemptFundDebts(now, 10)
	require.NoError(t, scanErr)
	require.Len(t, debts, 1)

	result, err := ReleaseTaskCreateAttemptHoldWarranty(
		attempt.ID, TaskCreateAttemptReleaseWarrantyDeadline, 0)
	require.NoError(t, err)
	assert.Equal(t, 25, result.ReleasedQuota)

	require.NoError(t, DB.First(&user, user.Id).Error)
	assert.Equal(t, 100, user.Quota)
	require.NoError(t, DB.First(attempt, attempt.ID).Error)
	assert.Equal(t, TaskCreateAttemptUnknown, attempt.Status)
	assert.Equal(t, TaskCreateAttemptBillingReleased, attempt.BillingHoldState)
	assert.Equal(t, TaskCreateAttemptReleaseWarrantyDeadline, attempt.ReleaseReason)
	assert.NotZero(t, attempt.RefundCompletedAt)
	assert.Empty(t, attempt.FrozenConnectionSnapshot)

	var exposure ProviderCostExposure
	require.NoError(t, DB.First(&exposure, "source_kind = ? AND source_id = ?",
		ProviderCostExposureSourceTaskCreateAttempt, attempt.AttemptID).Error)
	assert.Equal(t, "warranty_deadline", exposure.Reason)
	assert.Equal(t, 25, exposure.CustomerQuotaReleased)
	assert.Nil(t, exposure.ProviderAmount)

	// 幂等：重复调用不再重复退款。
	result, err = ReleaseTaskCreateAttemptHoldWarranty(
		attempt.ID, TaskCreateAttemptReleaseWarrantyDeadline, 0)
	require.NoError(t, err)
	assert.Zero(t, result.ReleasedQuota)
	require.NoError(t, DB.First(&user, user.Id).Error)
	assert.Equal(t, 100, user.Quota)
}

func TestTaskCreateAttemptReleaseIgnoresSoftDeletedToken(t *testing.T) {
	truncateTables(t)
	user := createFundsTestUser(t, 1504, 100)
	token := createFundsTestToken(t, user.Id, "funds-token-softdel", 50)
	attempt := createHeldTestAttempt(t, user.Id, token.Id, "softdel", 25, "wallet")
	require.NoError(t, DB.Delete(&token).Error)

	result, err := ReleaseTaskCreateAttemptHold(attempt.ID, TaskCreateAttemptRejected)
	require.NoError(t, err)
	assert.Equal(t, 25, result.ReleasedQuota)
	assert.False(t, result.TokenReleased)
	require.NoError(t, DB.First(&user, user.Id).Error)
	assert.Equal(t, 100, user.Quota)

	require.NoError(t, DB.First(attempt, attempt.ID).Error)
	assert.Equal(t, TaskCreateAttemptRejected, attempt.Status)
	assert.Equal(t, TaskCreateAttemptReleaseVerifiedRejection, attempt.ReleaseReason)
}

func TestInitTaskCreateAttemptFundsDeadlineBackfill(t *testing.T) {
	truncateTables(t)
	user := createFundsTestUser(t, 1505, 50)
	token := createFundsTestToken(t, user.Id, "funds-token-legacy", 60)
	attempt := createHeldTestAttempt(t, user.Id, token.Id, "legacy", 30, "wallet")
	require.NoError(t, DB.Model(&TaskCreateAttempt{}).Where("id = ?", attempt.ID).
		Update("funds_deadline_at", 0).Error)
	createdAt := attempt.CreatedAt

	previousMaster := common.IsMasterNode
	t.Cleanup(func() { common.IsMasterNode = previousMaster })
	common.IsMasterNode = true
	require.NoError(t, InitTaskCreateAttemptFundsDeadline())
	require.NoError(t, DB.First(attempt, attempt.ID).Error)
	assert.Equal(t, createdAt+TaskCreateFundsGuaranteeSeconds, attempt.FundsDeadlineAt)

	// 已释放记录不回填，避免改变历史事实。
	released := createHeldTestAttempt(t, user.Id, token.Id, "legacy2", 10, "wallet")
	_, err := ReleaseTaskCreateAttemptHold(released.ID, TaskCreateAttemptRejected)
	require.NoError(t, err)
	require.NoError(t, DB.Model(&TaskCreateAttempt{}).Where("id = ?", released.ID).
		Update("funds_deadline_at", 0).Error)
	require.NoError(t, InitTaskCreateAttemptFundsDeadline())
	require.NoError(t, DB.First(released, released.ID).Error)
	assert.Zero(t, released.FundsDeadlineAt)
}
