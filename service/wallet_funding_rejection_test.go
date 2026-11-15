package service

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/QuantumNous/new-api/i18n"
	"github.com/QuantumNous/new-api/model"
	relaycommon "github.com/QuantumNous/new-api/relay/common"
	"github.com/QuantumNous/new-api/relaykit/dto"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestWalletFirstSubscriptionFallbackPreservesTaskRejectionAndFunds(t *testing.T) {
	db := withErrorReportTestDB(t)
	require.NoError(t, i18n.Init())
	require.NoError(t, db.AutoMigrate(&model.User{}, &model.SubscriptionPlan{}, &model.UserSubscription{}, &model.SubscriptionPreConsumeRecord{}, &model.TaskCreateAttempt{}))
	for _, success := range []bool{false, true} {
		t.Run(fmt.Sprint(success), func(t *testing.T) {
			id := 42
			if success {
				id = 43
			}
			account := model.User{Id: id, Username: fmt.Sprint(id), AffCode: fmt.Sprint(id), Quota: 2}
			require.NoError(t, db.Create(&account).Error)
			if success {
				plan := model.SubscriptionPlan{Title: "fallback", TotalAmount: 100}
				require.NoError(t, db.Create(&plan).Error)
				sub := model.UserSubscription{UserId: id, PlanId: plan.Id, Status: "active", AmountTotal: 100,
					StartTime: time.Now().Add(-time.Hour).Unix(), EndTime: time.Now().Add(time.Hour).Unix()}
				require.NoError(t, db.Create(&sub).Error)
			}
			attempt, err := model.CreatePreparedTaskAttempt(model.TaskCreateAttemptParams{UserID: id, PublicTaskID: fmt.Sprint(id), ClientProtocol: "audit", RequestHash: "fixture"})
			require.NoError(t, err)
			info := &relaycommon.RelayInfo{UserId: id, IsPlayground: true, UserSetting: dto.UserSetting{BillingPreference: "wallet_first"},
				RequestId: fmt.Sprint("rejection-test-", id)}
			info.PriceData.Quota = 5
			hold, err := holdTaskAttemptForBilling(info, attempt.ID)
			if success {
				require.NoError(t, err)
				assert.Equal(t, BillingSourceSubscription, hold.BillingSource)
				c, _ := gin.CreateTestContext(httptest.NewRecorder())
				c.Request = httptest.NewRequest(http.MethodPost, "/v1/chat/completions", nil)
				require.Nil(t, PreConsumeBilling(c, 5, info))
				assert.Equal(t, BillingSourceSubscription, info.BillingSource)
			} else {
				require.ErrorIs(t, err, model.ErrTaskAttemptWalletFallbackRejected)
				require.ErrorIs(t, err, model.ErrTaskAttemptSubscriptionUnavailable)
				var stored model.TaskCreateAttempt
				require.NoError(t, db.First(&stored, attempt.ID).Error)
				assert.Equal(t, model.TaskCreateAttemptBillingUnheld, stored.BillingHoldState)
			}
			require.NoError(t, db.First(&account, id).Error)
			assert.Equal(t, 2, account.Quota)
		})
	}
}
