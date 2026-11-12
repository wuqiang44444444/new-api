package service

import (
	"context"
	"errors"
	"fmt"
	"html"
	"net/url"
	"slices"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/setting/system_setting"
)

var channelAlertDeliveryOnce sync.Once

func runChannelAlertDelivery() {
	channelAlertDeliveryOnce.Do(func() {
		for range 3 {
			go channelAlertMailWorker()
		}
		ticker := time.NewTicker(5 * time.Second)
		defer ticker.Stop()
		var previousOverdue int64 = -1
		for now := range ticker.C {
			ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
			if err := model.ExpireChannelAlertLeases(ctx, now.UnixMilli()); err != nil {
				common.SysError("channel alert mail lease scan failed")
			}
			var overdue int64
			err := model.DB.WithContext(ctx).Model(&model.ChannelAlertDelivery{}).
				Joins("JOIN channel_alert_episodes ON channel_alert_episodes.id = channel_alert_deliveries.episode_id").
				Where("channel_alert_deliveries.first_submitted_at = 0 AND channel_alert_deliveries.status NOT IN ? AND channel_alert_episodes.confirmed_at <= ?",
					[]string{model.MailDeliveryStopped, model.MailDeliveryAccepted}, now.Add(-time.Minute).UnixMilli()).Count(&overdue).Error
			if err == nil && overdue != previousOverdue {
				previousOverdue = overdue
				channelAlerts.stats.Over60.Store(uint64(overdue))
				if overdue > 0 {
					common.SysLog(fmt.Sprintf("channel alert mail overdue=%d", overdue))
				}
			}
			cancel()
		}
	})
}

func channelAlertMailWorker() {
	ticker := time.NewTicker(5 * time.Second)
	defer ticker.Stop()
	owner := common.GetRandomString(24)
	for {
		select {
		case <-ticker.C:
		case <-channelAlerts.wake:
		}
		for range 8 {
			found, err := deliverChannelAlertMail(owner, common.SendEmailContext, time.Now)
			if err != nil {
				common.SysError("channel alert delivery processing failed")
				break
			}
			if !found {
				break
			}
		}
	}
}

func deliverChannelAlertMail(owner string, send mailSendFunc, now func() time.Time) (bool, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	at := now()
	delivery, err := model.ClaimChannelAlertDelivery(ctx, owner, at.UnixMilli(), at.Add(30*time.Second).UnixMilli())
	if err != nil || delivery == nil {
		cancel()
		return false, err
	}
	var episode model.ChannelAlertEpisode
	addresses, readErr := model.GetOperationsMailRecipients(ctx)
	if readErr == nil && slices.Contains(addresses, delivery.Recipient) {
		readErr = model.DB.WithContext(ctx).First(&episode, delivery.EpisodeID).Error
	}
	cancel()

	status := model.MailDeliveryStopped
	var submitted, accepted, next int64
	if readErr != nil {
		status = model.MailDeliveryRetryWait
	} else if episode.ID > 0 {
		subject, body := channelAlertMessage(episode)
		sendCtx, cancelSend := context.WithTimeout(context.Background(), common.ShortMailSubmissionTimeout)
		sendCtx = common.WithMailSubmissionObserver(sendCtx, func(start time.Time) { submitted = start.UnixMilli() })
		sendErr := send(sendCtx, subject, delivery.Recipient, body)
		cancelSend()
		switch {
		case sendErr == nil:
			status = model.MailDeliveryAccepted
			accepted = now().UnixMilli()
		case errors.Is(sendErr, common.ErrMailSubmissionUnknown):
			status = model.MailDeliveryUnknown
		case errors.Is(sendErr, common.ErrMailBusy):
			status = model.MailDeliveryPending
			next = now().Add(5 * time.Second).UnixMilli()
		default:
			status = model.MailDeliveryRetryWait
		}
		if submitted > 0 && delivery.FirstSubmittedAt == 0 {
			channelAlerts.stats.Submitted.Add(1)
			if submitted-episode.ConfirmedAt > int64(time.Minute/time.Millisecond) {
				channelAlerts.stats.Late.Add(1)
			}
			common.SysLog(fmt.Sprintf("channel alert mail delivery_id=%d first_submission_ms=%d", delivery.ID, submitted-episode.ConfirmedAt))
		}
	}
	if status == model.MailDeliveryRetryWait {
		next = now().Add(time.Duration(mailDeliveryBackoff(delivery.Attempts)) * time.Second).UnixMilli()
	}
	if status == model.MailDeliveryUnknown {
		common.SysLog(fmt.Sprintf("channel alert mail delivery_id=%d outcome=unknown", delivery.ID))
	}
	ctx, cancel = context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	return true, model.FinishChannelAlertDelivery(ctx, *delivery, owner, status, next, submitted, accepted)
}

func channelAlertMessage(event model.ChannelAlertEpisode) (string, string) {
	zh, en := "渠道故障", "Channel outage"
	reasonZH, reasonEN := "上游服务持续失败", "Repeated upstream service failure"
	switch channelAlertKind(event.Kind) {
	case channelAlertNetwork:
		reasonZH, reasonEN = "节点连接持续失败", "Repeated connection failure from this gateway node"
	case channelAlertLimit:
		zh, en = "渠道降级预警", "Channel degradation warning"
		reasonZH, reasonEN = "持续限流，仍有请求成功", "Repeated rate limiting with successful calls"
	case channelAlertKey:
		reasonZH, reasonEN = "上游凭据停用", "Upstream credential inactive"
		if event.KeySlot >= 0 {
			zh, en = "渠道降级预警", "Channel degradation warning"
		}
	case channelAlertConfig:
		zh, en = "渠道配置故障", "Channel configuration fault"
		reasonZH, reasonEN = "渠道配置阻断调用", "Channel configuration blocks this operation"
	}
	subject := fmt.Sprintf("%s / %s — #%d", zh, en, event.ChannelID)
	rows := [][2]string{
		{"事件 / Incident", strconv.FormatInt(event.ID, 10)},
		{"模型 / Model", event.Model},
		{"操作 / Operation", event.Operation},
		{"原因 / Reason", reasonZH + " / " + reasonEN},
		{"首个失败 / First observed", time.UnixMilli(event.FirstAt).In(errorReportLocation()).Format(time.RFC3339)},
		{"确认 / Confirmed", time.UnixMilli(event.ConfirmedAt).In(errorReportLocation()).Format(time.RFC3339)},
	}
	if event.Node != "" {
		rows = append(rows, [2]string{"节点 / Gateway", event.Node})
	}
	if event.KeySlot >= 0 {
		rows = append(rows, [2]string{"凭据槽位 / Credential slot", strconv.Itoa(event.KeySlot + 1)})
	}
	if event.StatusCode > 0 {
		rows = append(rows, [2]string{"上游状态 / Upstream HTTP", strconv.Itoa(event.StatusCode)})
	}
	var body strings.Builder
	writeOperationsMailRows(&body, rows)

	address, err := url.Parse(strings.TrimSpace(system_setting.ServerAddress))
	if err == nil && (address.Scheme == "http" || address.Scheme == "https") && address.Host != "" && address.User == nil {
		address.Path = strings.TrimRight(address.Path, "/") + "/channels"
		address.RawPath, address.RawQuery, address.Fragment = "", "", ""
		fmt.Fprintf(&body, `<p><a href="%s" style="color:#075b68;font-weight:600">管理后台 → 渠道 / Admin → Channels</a></p>`, html.EscapeString(address.String()))
	} else {
		body.WriteString("<p>管理后台 → 渠道 / Admin → Channels</p>")
	}
	return subject, renderOperationsMail(subject, body.String())
}

func ChannelAlertHealth() map[string]any {
	return map[string]any{
		"enabled": channelAlerts.enabled.Load(), "dropped": channelAlerts.stats.Dropped.Load(), "window_resets": channelAlerts.stats.Resets.Load(),
		"confirmed": channelAlerts.stats.Confirmed.Load(), "first_submissions": channelAlerts.stats.Submitted.Load(), "late_first_submissions": channelAlerts.stats.Late.Load(), "overdue_without_submission": channelAlerts.stats.Over60.Load(),
	}
}
