package service

import (
	"context"
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/QuantumNous/new-api/clienterrlog"
	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
	"github.com/glebarez/sqlite"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

func TestMailErrorSamplesSeparateStoresAndKeepLatestFiveWithinScope(t *testing.T) {
	db := withErrorReportTestDB(t)
	logs, err := gorm.Open(sqlite.Open(t.TempDir()+"/logs.db"), &gorm.Config{})
	require.NoError(t, err)
	model.LOG_DB = logs
	require.NoError(t, logs.AutoMigrate(&model.ErrorEvent{}))
	require.NoError(t, db.Create(&[]model.User{
		{Id: 1, Username: "customer-one", DisplayName: "星河 <Studio>", AffCode: "c1", Role: common.RoleCommonUser},
		{Id: 2, Username: "root-name", AffCode: "r2", Role: common.RoleRootUser},
	}).Error)
	var events []model.ErrorEvent
	for i := 1; i <= 7; i++ {
		events = append(events, model.ErrorEvent{UserId: 1, CreatedAt: 151, EventType: clienterrlog.EventAPIError, Status: 403,
			Reason: clienterrlog.WalletQuotaInsufficient, RequestId: fmt.Sprintf("request-%02d", i), Route: "/v1/test?token=private-query", Method: "POST", Detail: `{"body":"private-payload"}`})
	}
	events = append(events,
		model.ErrorEvent{UserId: 2, CreatedAt: 152, EventType: clienterrlog.EventAPIError, Status: 403, Reason: clienterrlog.WalletQuotaInsufficient, RequestId: "unrelated-root"},
		model.ErrorEvent{UserId: 1, CreatedAt: 200, EventType: clienterrlog.EventAPIError, Status: 403, Reason: clienterrlog.WalletQuotaInsufficient, RequestId: "unrelated-window"},
		model.ErrorEvent{UserId: 1, CreatedAt: 155, EventType: clienterrlog.EventAPIError, Status: 403, Reason: clienterrlog.SubscriptionQuotaInsufficient, RequestId: "unrelated-reason"},
		model.ErrorEvent{UserId: 3, Username: "historical-name", CreatedAt: 159, EventType: clienterrlog.EventAPIError, Status: 403, Reason: clienterrlog.WalletQuotaInsufficient, RequestId: "orphan"},
	)
	require.NoError(t, logs.Create(&events).Error)
	ctx := context.Background()
	wallet := loadMailErrorDetails(ctx, model.MailErrorScope{Start: 150, End: 200, UserIDs: []int{1}, Reason: clienterrlog.WalletQuotaInsufficient}, nil, false)
	require.Len(t, wallet.rows, 5)
	for i, row := range wallet.rows {
		assert.Equal(t, fmt.Sprintf("request-%02d", 7-i), row.RequestID)
	}
	var body strings.Builder
	writeMailErrorDetails(&body, wallet)
	assert.Contains(t, body.String(), "星河 &lt;Studio&gt; (#1)")
	for _, secret := range []string{"unrelated-", "private-query", "private-payload", "request-01", "request-02"} {
		assert.NotContains(t, body.String(), secret)
	}
	customer := loadMailErrorDetails(ctx, model.MailErrorScope{Start: 150, End: 200, CustomerOnly: true, FailuresOnly: true}, nil, false)
	require.Len(t, customer.rows, 5)
	for _, row := range customer.rows {
		assert.Equal(t, 1, row.UserID)
	}
	all := loadMailErrorDetails(ctx, model.MailErrorScope{Start: 150, End: 200}, nil, false)
	body.Reset()
	writeMailErrorDetails(&body, all)
	assert.Contains(t, body.String(), "historical-name (#3)")
	empty := loadMailErrorDetails(ctx, model.MailErrorScope{Start: 150, End: 200, UserIDs: []int{}}, nil, true)
	require.Empty(t, empty.rows)
	body.Reset()
	writeMailErrorDetails(&body, empty)
	assert.Contains(t, body.String(), "No matching errors were found")
	assert.Contains(t, body.String(), "Details cover the available logs only")
	model.LOG_DB = nil
	unavailable := loadMailErrorDetails(ctx, model.MailErrorScope{Start: 150, End: 200}, nil, false)
	body.Reset()
	writeMailErrorDetails(&body, unavailable)
	assert.Contains(t, body.String(), "Error details are temporarily unavailable")
	assert.NotContains(t, body.String(), "No matching errors were found")
}

func TestChannelMailFreezesOnlyCountedSamples(t *testing.T) {
	db := withErrorReportTestDB(t)
	require.NoError(t, db.AutoMigrate(&model.ChannelAlertDelivery{}))
	withErrorReportSetting(t, "ops@example.com")
	end := time.Date(2026, 11, 15, 18, 10, 0, 0, errorReportLocation())
	channel := model.Channel{Id: 88, Name: "Demo channel", Key: "fixture"}
	require.NoError(t, db.Create(&channel).Error)
	require.NoError(t, db.Create(&model.User{Id: 4, Username: "channel-user", DisplayName: "Frozen name", AffCode: "c4"}).Error)
	var observations []channelAlertSample
	for i := 1; i <= 7; i++ {
		at := end.Add(time.Duration(i-9) * time.Second)
		id := fmt.Sprintf("counted-%d", i)
		observations = append(observations, channelAlertSample{ID: id, At: at.UnixMilli(), Status: 503, HasRequestID: true})
		if i != 4 {
			require.NoError(t, db.Create(&model.ErrorEvent{RequestId: id, ChannelId: 88, UserId: 4, CreatedAt: at.Add(3 * time.Second).Unix()}).Error)
		}
	}
	require.NoError(t, db.Create(&model.ErrorEvent{RequestId: "uncounted", ChannelId: 88, UserId: 4, CreatedAt: end.Add(-time.Second).Unix()}).Error)
	episode, ok := channelAlertCandidate(channelAlertGroup{scope: channelAlertScope{Channel: 88, Revision: model.ChannelAlertRevision(&channel), Model: "channel-model", Operation: "relay"}, kind: channelAlertService}, observations, 0)
	require.True(t, ok)
	ctx := context.Background()
	require.NoError(t, db.Migrator().DropTable(&model.ChannelAlertDelivery{}))
	logs := model.LOG_DB
	model.LOG_DB = nil
	require.NoError(t, confirmChannelAlert(ctx, episode, end))
	var stored model.ChannelAlertEpisode
	require.NoError(t, db.First(&stored).Error)
	assert.True(t, stored.MailPending)
	assert.Empty(t, stored.MailEvidenceHTML)
	require.Error(t, model.RetryIncidentMail(ctx, true))
	require.NoError(t, db.AutoMigrate(&model.ChannelAlertDelivery{}))
	model.LOG_DB = logs
	var body string
	processed, err := deliverChannelAlertMail("first", func(_ context.Context, _, _, html string) error {
		body = html
		return fmt.Errorf("simulated temporary transport failure")
	}, func() time.Time { return end })
	require.NoError(t, err)
	require.True(t, processed)
	require.NoError(t, db.Model(&model.User{}).Where("id = ?", 4).Update("display_name", "Changed after confirmation").Error)
	var retry model.ChannelAlertDelivery
	require.NoError(t, db.First(&retry).Error)
	processed, err = deliverChannelAlertMail("retry", func(_ context.Context, _, _, html string) error {
		assert.Equal(t, body, html)
		return nil
	}, func() time.Time { return time.UnixMilli(retry.NextAttemptAt) })
	require.NoError(t, err)
	require.True(t, processed)
	assert.Contains(t, body, "Frozen name (#4)")
	assert.NotContains(t, body, "Changed after confirmation")
	assert.Contains(t, body, "counted-3")
	assert.Contains(t, body, "counted-4")
	assert.Contains(t, body, "Some incident samples have no matching error log")
	assert.Contains(t, body, "Service error")
	assert.NotContains(t, body, "counted-2")
	assert.NotContains(t, body, "uncounted")
	assert.Contains(t, body, "Failures counted</th>")
}

func TestChannelAlertDeliversWithUnavailableErrorDetails(t *testing.T) {
	db := withErrorReportTestDB(t)
	require.NoError(t, db.AutoMigrate(&model.ChannelAlertDelivery{}))
	withErrorReportSetting(t, "ops@example.com")
	now := time.Now()
	channel := model.Channel{Name: "mail outage", Key: "fixture"}
	require.NoError(t, db.Create(&channel).Error)
	model.LOG_DB = nil
	for i := range 3 {
		key := fmt.Sprintf("mail-outage-%d", i)
		err := confirmChannelAlert(context.Background(), model.ChannelAlertEpisode{
			ChannelID: channel.Id, OpenKey: &key, ScopeKey: key, Revision: model.ChannelAlertRevision(&channel),
			Kind: string(channelAlertService), FirstAt: now.UnixMilli(), LastAt: now.UnixMilli(),
			MailSamples: []model.ChannelMailObservation{{At: now.UnixMilli(), RequestID: key, Status: 503}},
		}, now)
		require.NoError(t, err)
	}
	var count int64
	require.NoError(t, db.Model(&model.ChannelAlertEpisode{}).Count(&count).Error)
	assert.EqualValues(t, 3, count)
	for range 3 {
		found, err := deliverChannelAlertMail("fixture", func(_ context.Context, _, _, body string) error {
			assert.Contains(t, body, "Linked error logs are temporarily unavailable")
			assert.Contains(t, body, "HTTP 503")
			return nil
		}, time.Now)
		require.NoError(t, err)
		require.True(t, found)
	}
}
