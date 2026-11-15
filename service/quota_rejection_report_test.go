package service

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/QuantumNous/new-api/clienterrlog"
	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
	relaycommon "github.com/QuantumNous/new-api/relay/common"
	"github.com/QuantumNous/new-api/relaykit/dto"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestDailyReportReceivesWalletSubscriptionAndAPIKeyRejections(t *testing.T) {
	db := withErrorReportTestDB(t)
	require.NoError(t, db.AutoMigrate(&model.User{}, &model.Token{}, &model.UserSubscription{}, &model.SubscriptionPlan{}, &model.SubscriptionPreConsumeRecord{}))
	withErrorReportSetting(t, "ops@example.com")
	inbox := newLocalMailInbox(t)
	require.NoError(t, db.Create(&model.User{Id: 42, Username: "no-funding", AffCode: "q42", Quota: 0}).Error)
	require.NoError(t, db.Create(&model.User{Id: 43, Username: "key-owner", AffCode: "q43", Quota: 100}).Error)
	require.NoError(t, db.Create(&model.Token{Id: 9, UserId: 43, Key: "paper-fixture-key", RemainQuota: 1, Status: common.TokenStatusEnabled}).Error)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	recorded := make(chan clienterrlog.Event, 4)
	sink := clienterrlog.NewSink(ctx, 16, func(event clienterrlog.Event) error { recorded <- event; return nil })
	engine := gin.New()
	engine.Use(sink.Middleware())
	engine.GET("/:funding", func(c *gin.Context) {
		funding := c.Param("funding")
		userID := 42
		if funding == "key" {
			userID = 43
			funding = "wallet_only"
		}
		c.Set("route_tag", "relay")
		c.Set("id", userID)
		clienterrlog.MarkAuthPassed(c, clienterrlog.AuthSourceAPIToken)
		info := &relaycommon.RelayInfo{UserId: userID, TokenId: 9, TokenKey: "paper-fixture-key", RequestId: "quota-test-" + c.Param("funding"), UserSetting: dto.UserSetting{BillingPreference: funding}}
		info.IsPlayground = c.Param("funding") != "key"
		if rejection := PreConsumeBilling(c, 5, info); rejection != nil {
			c.Status(rejection.StatusCode)
			return
		}
		c.Status(http.StatusOK)
	})
	day := time.Date(2026, 11, 9, 0, 0, 0, 0, errorReportLocation())
	start, end := day.Unix(), day.AddDate(0, 0, 1).Unix()
	for route, reason := range map[string]string{
		"wallet_only":       clienterrlog.WalletQuotaInsufficient,
		"wallet_first":      clienterrlog.WalletQuotaInsufficient,
		"subscription_only": clienterrlog.SubscriptionQuotaInsufficient,
		"key":               clienterrlog.APIKeyQuotaInsufficient,
	} {
		response := httptest.NewRecorder()
		engine.ServeHTTP(response, httptest.NewRequest(http.MethodGet, "/"+route, nil))
		require.Equal(t, http.StatusForbidden, response.Code)
		select {
		case event := <-recorded:
			assert.Equal(t, reason, event.Reason)
			require.NoError(t, db.Create(&model.ErrorEvent{
				CreatedAt: end - 1, UserId: event.UserID, EventType: event.EventType, Status: event.Status, Reason: event.Reason,
			}).Error)
		case <-time.After(3 * time.Second):
			t.Fatal("quota rejection was not recorded")
		}
	}
	id := fmt.Sprintf("daily-%d", start)
	report, err := model.LoadDailyReport(ctx, start, id)
	require.NoError(t, err)
	require.NoError(t, model.SaveDailyReportProgress(ctx, report, end-300, `{"signals":{}}`, 0))
	h := newErrorReportHandler(common.SendEmailContext)
	h.now = func() time.Time { return day.Add(30 * time.Hour) }
	generated, _, err := h.generateDailyReport(ctx)
	require.NoError(t, err)
	require.Equal(t, 1, generated)
	_, accepted, err := h.deliverClaims(ctx, "quota-mail")
	require.NoError(t, err)
	require.Equal(t, 1, accepted)
	mail := requireLocalMail(t, inbox, "Daily operations report", "ops@example.com")
	for _, label := range []string{"Insufficient wallet balance", "Subscription unavailable or insufficient quota", "Insufficient API key quota"} {
		assert.Contains(t, mail.body, label)
	}
	assert.Equal(t, 3, strings.Count(mail.body, "1 accounts in retained logs"))
	assert.NotContains(t, mail.body, "paper-fixture-key")
}
