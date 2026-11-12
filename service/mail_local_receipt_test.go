package service

import (
	"context"
	"io"
	"mime"
	"net"
	"net/mail"
	"net/textproto"
	"strings"
	"testing"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/model"
	relaycommon "github.com/QuantumNous/new-api/relay/common"
	"github.com/QuantumNous/new-api/relaykit/dto"
	"github.com/QuantumNous/new-api/relaykit/types"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type inboxMail struct{ subject, to, body string }

func newLocalMailInbox(t *testing.T) <-chan inboxMail {
	t.Helper()
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	require.NoError(t, err)
	port := listener.Addr().(*net.TCPAddr).Port
	raw, err := common.Marshal(common.MailTransportConfig{Mode: common.MailTransportSMTP, SMTP: &common.SMTPMailConfig{
		Server: "127.0.0.1", Port: port, From: "sender@example.com",
	}})
	require.NoError(t, err)
	common.OptionMapRWMutex.Lock()
	old := common.OptionMap
	common.OptionMap = map[string]string{common.MailTransportOption: string(raw)}
	common.OptionMapRWMutex.Unlock()
	t.Cleanup(func() {
		_ = listener.Close()
		common.OptionMapRWMutex.Lock()
		common.OptionMap = old
		common.OptionMapRWMutex.Unlock()
	})
	inbox := make(chan inboxMail, 32)
	go func() {
		for {
			conn, err := listener.Accept()
			if err != nil {
				return
			}
			go receiveLocalSMTP(conn, inbox)
		}
	}()
	return inbox
}

func receiveLocalSMTP(conn net.Conn, inbox chan<- inboxMail) {
	defer conn.Close()
	_ = conn.SetDeadline(time.Now().Add(10 * time.Second))
	wire := textproto.NewConn(conn)
	if wire.PrintfLine("220 local inbox") != nil {
		return
	}
	for {
		line, err := wire.ReadLine()
		if err != nil {
			return
		}
		switch strings.ToUpper(strings.Fields(line)[0]) {
		case "DATA":
			if wire.PrintfLine("354 send message") != nil {
				return
			}
			message, err := mail.ReadMessage(wire.DotReader())
			if err != nil {
				return
			}
			body, err := io.ReadAll(message.Body)
			if err != nil {
				return
			}
			title, err := new(mime.WordDecoder).DecodeHeader(message.Header.Get("Subject"))
			if err != nil || wire.PrintfLine("250 received") != nil {
				return
			}
			inbox <- inboxMail{title, message.Header.Get("To"), string(body)}
		case "QUIT":
			_ = wire.PrintfLine("221 closing")
			return
		default:
			if wire.PrintfLine("250 local inbox") != nil {
				return
			}
		}
	}
}

func requireLocalMail(t *testing.T, inbox <-chan inboxMail, subject, recipient string) inboxMail {
	t.Helper()
	select {
	case mail := <-inbox:
		assert.Contains(t, mail.subject, subject)
		assert.Equal(t, recipient, mail.to)
		return mail
	case <-time.After(8 * time.Second):
		require.FailNow(t, "SMTP inbox did not receive "+subject)
		return inboxMail{}
	}
}

func TestOperationalMailReachesLocalInbox(t *testing.T) {
	oldCount, oldDuration := constant.NotifyLimitCount, constant.NotificationLimitDurationMinute
	constant.NotifyLimitCount, constant.NotificationLimitDurationMinute = 2, 10
	t.Cleanup(func() { constant.NotifyLimitCount, constant.NotificationLimitDurationMinute = oldCount, oldDuration })
	db := withErrorReportTestDB(t)
	inbox := newLocalMailInbox(t)
	require.NoError(t, db.AutoMigrate(&model.User{}, &model.Ability{}, &model.Channel{}, &model.AuditLog{}))
	root := model.User{Id: 7042, Username: "mail-root", Email: "ops@example.com", Role: common.RoleRootUser, Status: common.UserStatusEnabled}
	root.SetSetting(dto.UserSetting{Language: "en", QuotaWarningThreshold: 10})
	require.NoError(t, db.Create(&root).Error)
	require.NoError(t, db.Create(&model.Option{Key: "error_report_setting.recipients", Value: root.Email}).Error)

	checkAndSendSubscriptionQuotaNotify(&relaycommon.RelayInfo{
		UserId: root.Id, UserEmail: root.Email, UserSetting: root.GetSetting(),
		SubscriptionId: 1, SubscriptionAmountTotal: 100, SubscriptionAmountUsedAfterPreConsume: 95,
	})
	requireLocalMail(t, inbox, "subscription quota", root.Email)
	NotifyVideoFundFailure()
	requireLocalMail(t, inbox, "Video refund needs attention", root.Email)

	channel := model.Channel{Name: "fixture", Type: 1, Status: common.ChannelStatusEnabled, Models: "gpt-4o-mini", Group: "default"}
	require.NoError(t, db.Create(&channel).Error)
	DisableChannel(*types.NewChannelError(channel.Id, channel.Type, channel.Name, false, "", true), "fixture failure")
	requireLocalMail(t, inbox, "has been disabled", root.Email)
	EnableChannel(channel.Id, "", channel.Name)
	requireLocalMail(t, inbox, "has been enabled", root.Email)
	rootSetting := root.GetSetting()
	rootSetting.UpstreamModelUpdateNotifyEnabled = true
	root.SetSetting(rootSetting)
	require.NoError(t, db.Model(&root).Update("setting", root.Setting).Error)
	NotifyUpstreamModelUpdateWatchers(func(string) (string, string) {
		return "Models changed", "Model <a href=\"https://example.test\">link</a>"
	})
	modelMail := requireLocalMail(t, inbox, "Models changed", root.Email)
	assert.Contains(t, modelMail.body, "&lt;a href=")
	assert.NotContains(t, modelMail.body, "<a href=")

	ctx := context.Background()
	for _, resource := range []string{"cpu", "memory", "disk_data"} {
		now := time.Now().Unix()
		sample := resourceObservation{key: resource, label: resource, usage: 85, threshold: 85}
		_, err := observeResourceAlert(ctx, "fixture-node", sample, now)
		require.NoError(t, err)
		require.NoError(t, deliverResourceAlerts(time.Now, common.SendEmailContext))
		first := requireLocalMail(t, inbox, "Resource threshold alert", root.Email)
		assert.Contains(t, first.body, "85.0%")
		_, err = observeResourceAlert(ctx, "fixture-node", sample, now+1)
		require.NoError(t, err)
		sample.usage = 84
		_, err = observeResourceAlert(ctx, "fixture-node", sample, now+2)
		require.NoError(t, err)
		sample.usage = 86
		_, err = observeResourceAlert(ctx, "fixture-node", sample, now+3)
		require.NoError(t, err)
		require.NoError(t, deliverResourceAlerts(time.Now, common.SendEmailContext))
		second := requireLocalMail(t, inbox, "Resource threshold alert", root.Email)
		assert.Contains(t, second.body, "86.0%")
		var count int64
		require.NoError(t, db.Model(&model.ResourceAlertEpisode{}).Where("resource = ?", resource).Count(&count).Error)
		assert.EqualValues(t, 2, count, "episode count for "+resource)
	}
}

func TestUpstreamBalanceMailReachesLocalInbox(t *testing.T) {
	setupUpstreamBalanceAlertTest(t)
	inbox := newLocalMailInbox(t)
	h := &upstreamBalanceAlertHandler{send: common.SendEmailContext, now: time.Now,
		query: func(context.Context, *model.Channel, int, string) UpstreamBalanceResult {
			return UpstreamBalanceResult{Status: "ok", Amounts: []UpstreamBalanceAmount{{Amount: "99", Unit: "USD"}}}
		},
	}
	result, err := h.checkAndNotify(context.Background(), "smtp-receipt")
	require.NoError(t, err)
	require.Equal(t, 2, result.Sent)
	requireLocalMail(t, inbox, "Upstream balance below USD 100", "one@example.com")
	requireLocalMail(t, inbox, "Upstream balance below USD 100", "two@example.com")
}
