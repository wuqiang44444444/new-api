package model

import (
	"errors"
	"testing"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

func TestNativeImageFundingPreferenceAndAtomicSettlement(t *testing.T) {
	for _, tc := range []struct {
		name, pref, source      string
		wallet, subTotal, quota int
		overflow, reject        bool
	}{
		{name: "wallet_zero_hold", pref: "wallet_only", source: "wallet", wallet: 100, subTotal: 100, quota: 0},
		{name: "wallet_only", pref: "wallet_only", source: "wallet", wallet: 100, subTotal: 100, quota: 40},
		{name: "subscription_only", pref: "subscription_only", source: "subscription", wallet: 100, subTotal: 100, quota: 40},
		{name: "subscription_first", pref: "subscription_first", source: "subscription", wallet: 100, subTotal: 100, quota: 40},
		{name: "wallet_first_fallback", pref: "wallet_first", source: "subscription", wallet: 10, subTotal: 100, quota: 40},
		{name: "wallet_overflow", pref: "subscription_first", source: "wallet", wallet: 100, subTotal: 10, quota: 40, overflow: true},
		{name: "no_wallet_overflow", pref: "subscription_first", wallet: 100, subTotal: 10, quota: 40, reject: true},
		{name: "no_subscription_fallback", pref: "subscription_only", wallet: 100, subTotal: 10, quota: 40, reject: true},
		{name: "subscription_minimum_hold", pref: "subscription_only", source: "subscription", wallet: 100, subTotal: 100, quota: 0},
	} {
		t.Run(tc.name, func(t *testing.T) {
			resetNativeImageFundingTables(t)
			user := User{Id: 1791, Username: "native-funding", Quota: tc.wallet}
			require.NoError(t, DB.Create(&user).Error)
			token := Token{Id: 1791, UserId: user.Id, Key: "native-funding-token", RemainQuota: 100}
			require.NoError(t, DB.Create(&token).Error)
			plan := SubscriptionPlan{Title: "native-funding", TotalAmount: 100}
			require.NoError(t, DB.Create(&plan).Error)
			sub := UserSubscription{UserId: user.Id, PlanId: plan.Id, AmountTotal: int64(tc.subTotal), Status: "active", StartTime: time.Now().Add(-time.Hour).Unix(), EndTime: time.Now().Add(time.Hour).Unix(), AllowWalletOverflow: tc.overflow}
			require.NoError(t, DB.Create(&sub).Error)
			task := &Task{TaskID: GenerateTaskID(), UserId: user.Id, AppID: token.Id, Quota: tc.quota, ClientProtocol: TaskClientProtocolImageOpenAIV1, Status: TaskStatusQueued, PrivateData: TaskPrivateData{TokenId: token.Id, ImageTask: &TaskImageExecutionData{NativeRequest: &TaskNativeImageRequest{}}, AsyncBilling: &TaskAsyncBillingContext{State: TaskBillingStatePending}}}
			params := ImageTaskInsertParams{Task: task, FundingPreference: tc.pref, GlobalScope: ImageTaskAdmissionScopeGlobal(), AppScope: ImageTaskAdmissionScopeApp(user.Id, token.Id)}
			err := InsertImageTask(params)
			if tc.reject {
				require.ErrorIs(t, err, ErrTaskAttemptSubscriptionUnavailable)
				var count int64
				require.NoError(t, DB.Model(&Task{}).Count(&count).Error)
				assert.Zero(t, count)
				require.NoError(t, DB.First(&user, user.Id).Error)
				require.NoError(t, DB.First(&sub, sub.Id).Error)
				require.NoError(t, DB.First(&token, token.Id).Error)
				assert.Equal(t, tc.wallet, user.Quota)
				assert.Zero(t, sub.AmountUsed)
				assert.Equal(t, 100, token.RemainQuota)
				return
			}
			require.NoError(t, err)
			assert.Equal(t, tc.source, task.PrivateData.BillingSource)
			held := tc.quota
			if tc.source == "subscription" && held == 0 {
				held = 1
			}
			assert.Equal(t, held, task.PrivateData.ImageTask.HeldQuota)
			require.NoError(t, DB.First(&sub, sub.Id).Error)
			require.NoError(t, DB.First(&user, user.Id).Error)
			require.NoError(t, DB.First(&token, token.Id).Error)
			assert.Equal(t, 100-held, token.RemainQuota)
			if tc.source == "subscription" {
				assert.EqualValues(t, held, sub.AmountUsed)
				assert.Equal(t, tc.wallet, user.Quota)
				require.NoError(t, RefundSubscriptionPreConsume(task.TaskID))
				require.NoError(t, DB.First(&sub, sub.Id).Error)
				assert.EqualValues(t, held, sub.AmountUsed, "request refund cannot release a task-owned hold")
			} else {
				assert.Equal(t, tc.wallet-held, user.Quota)
				assert.Zero(t, sub.AmountUsed)
			}
			won, err := FinishImageTaskFailure(task, TaskStatusFailure, "test-rejection")
			require.NoError(t, err)
			require.True(t, won)
			applied, _, err := ApplyTaskBillingTarget(task, 0)
			require.NoError(t, err)
			require.True(t, applied)
			applied, _, err = ApplyTaskBillingTarget(task, 0)
			require.NoError(t, err)
			assert.False(t, applied)
			require.NoError(t, DB.First(&user, user.Id).Error)
			require.NoError(t, DB.First(&sub, sub.Id).Error)
			require.NoError(t, DB.First(&token, token.Id).Error)
			assert.Equal(t, tc.wallet, user.Quota)
			assert.Zero(t, sub.AmountUsed)
			assert.Equal(t, 100, token.RemainQuota)
		})
	}
}

func TestNativeImageAdmissionRollsBackSubscriptionWhenTaskWriteFails(t *testing.T) {
	resetNativeImageFundingTables(t)
	user := User{Id: 1792, Username: "native-rollback", Quota: 100}
	require.NoError(t, DB.Create(&user).Error)
	token := Token{Id: 1792, UserId: user.Id, Key: "rollback-token", RemainQuota: 100}
	require.NoError(t, DB.Create(&token).Error)
	plan := SubscriptionPlan{Title: "rollback", TotalAmount: 100}
	require.NoError(t, DB.Create(&plan).Error)
	sub := UserSubscription{UserId: user.Id, PlanId: plan.Id, AmountTotal: 100, Status: "active", EndTime: common.GetTimestamp() + 3600}
	require.NoError(t, DB.Create(&sub).Error)
	const hook = "native-image-task-write-failure"
	require.NoError(t, DB.Callback().Create().Before("gorm:create").Register(hook, func(tx *gorm.DB) {
		if tx.Statement.Table == "tasks" {
			tx.AddError(errors.New("injected write failure"))
		}
	}))
	t.Cleanup(func() { DB.Callback().Create().Remove(hook) })
	task := &Task{TaskID: GenerateTaskID(), UserId: user.Id, AppID: token.Id, Quota: 40, ClientProtocol: TaskClientProtocolImageOpenAIV1, Status: TaskStatusQueued, PrivateData: TaskPrivateData{TokenId: token.Id, ImageTask: &TaskImageExecutionData{NativeRequest: &TaskNativeImageRequest{}}}}
	require.Error(t, InsertImageTask(ImageTaskInsertParams{Task: task, FundingPreference: "subscription_only", GlobalScope: ImageTaskAdmissionScopeGlobal(), AppScope: ImageTaskAdmissionScopeApp(user.Id, token.Id)}))
	require.NoError(t, DB.First(&sub, sub.Id).Error)
	require.NoError(t, DB.First(&token, token.Id).Error)
	assert.Zero(t, sub.AmountUsed)
	assert.Equal(t, 100, token.RemainQuota)
	for _, table := range []any{&Task{}, &SubscriptionPreConsumeRecord{}} {
		var count int64
		require.NoError(t, DB.Model(table).Count(&count).Error)
		assert.Zero(t, count)
	}
}

func resetNativeImageFundingTables(t *testing.T) {
	t.Helper()
	t.Cleanup(func() {
		for _, table := range []any{&Task{}, &TaskCreateIdempotency{}, &ImageTaskSlot{}, &SubscriptionPreConsumeRecord{}, &UserSubscription{}, &SubscriptionPlan{}, &Token{}, &User{}} {
			require.NoError(t, DB.Session(&gorm.Session{AllowGlobalUpdate: true}).Unscoped().Delete(table).Error)
		}
	})
}
