package service

import (
	"context"
	"errors"
	"fmt"
	"html"
	"net/http"
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
	if err == nil && delivery == nil {
		if err = model.RetryIncidentMail(ctx, true); err == nil {
			delivery, err = model.ClaimChannelAlertDelivery(ctx, owner, at.UnixMilli(), at.Add(30*time.Second).UnixMilli())
		}
	}
	if err != nil || delivery == nil {
		cancel()
		return false, err
	}
	var episode model.ChannelAlertEpisode
	var currentChannelName string
	addresses, readErr := model.GetOperationsMailRecipients(ctx)
	if readErr == nil && slices.Contains(addresses, delivery.Recipient) {
		readErr = model.DB.WithContext(ctx).First(&episode, delivery.EpisodeID).Error
		if readErr == nil && episode.ChannelName == "" {
			names, _ := model.ReadDailyReportChannelNames(ctx, []int{episode.ChannelID})
			currentChannelName = common.SanitizeTaskDiagnostic(names[episode.ChannelID])
		}
	}
	cancel()

	status, stopReason := model.MailDeliveryStopped, ""
	var submitted, accepted, next int64
	if readErr != nil {
		status = model.MailDeliveryRetryWait
	} else if episode.ID > 0 {
		if episode.MailEvidenceHTML == "" && episode.MailSamplesJSON != "" {
			enrichChannelAlertMail(&episode)
		}
		subject, body := channelAlertMessage(episode, currentChannelName)
		sendCtx, cancelSend := context.WithTimeout(context.Background(), common.ShortMailSubmissionTimeout)
		sendCtx = common.WithMailSubmissionObserver(sendCtx, func(start time.Time) { submitted = start.UnixMilli() })
		sendErr := submitManagedMail(sendCtx, send, model.ChannelAlertMailType(episode.Kind), model.QueuedMailTime(delivery.QueuedAt, episode.ConfirmedAt/1000), subject, delivery.Recipient, body)
		cancelSend()
		switch {
		case sendErr == nil:
			status = model.MailDeliveryAccepted
			accepted = now().UnixMilli()
		case errors.Is(sendErr, errMailPreferenceDisabled):
			stopReason = model.MailStopPreferenceDisabled
		case errors.Is(sendErr, common.ErrMailPermanentlyRejected):
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
	return true, model.FinishChannelAlertDelivery(ctx, *delivery, owner, status, next, submitted, accepted, stopReason)
}

func channelAlertMessage(event model.ChannelAlertEpisode, currentChannelName string) (string, string) {
	zh, en := "渠道异常", "Channel alert"
	reasonZH, reasonEN := "观察到多次上游服务错误", "Repeated upstream service errors observed"
	switch channelAlertKind(event.Kind) {
	case channelAlertBalance:
		zh, en = "渠道上游欠费", "Upstream channel balance alert"
		reasonZH, reasonEN = "上游因欠费或余额不足拒绝请求，请检查渠道上游账单和余额", "The upstream rejected a request due to an overdue or insufficient balance. Check the channel's upstream billing and balance."
	case channelAlertNetwork:
		reasonZH, reasonEN = "本网关节点观察到多次连接或读取失败", "Repeated connection or read failures observed by this gateway"
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
	nameLabel, channelName := "渠道名称 / Channel name", event.ChannelName
	if channelName == "" {
		channelName = "未记录 / Not recorded"
		if currentChannelName != "" {
			nameLabel, channelName = "当前渠道名称 / Current channel name", currentChannelName
		}
	}
	location := errorReportLocation()
	first, last, confirmed := time.UnixMilli(event.FirstAt), time.UnixMilli(event.LastAt), time.UnixMilli(event.ConfirmedAt)
	rows := [][2]string{
		{"事件 / Incident", strconv.FormatInt(event.ID, 10)},
		{"渠道 ID / Channel ID", strconv.Itoa(event.ChannelID)},
		{nameLabel, channelName},
		{"模型 / Model", common.SanitizeTaskDiagnostic(event.Model)},
		{"操作 / Operation", event.Operation},
		{"原因 / Reason", reasonZH + " / " + reasonEN},
	}
	if event.EvidenceCount > 0 {
		rows = append(rows, [2]string{"计入告警的失败数 / Failures counted", strconv.Itoa(event.EvidenceCount)})
	}
	rows = append(rows, [2]string{"首次失败 / First failure", first.In(location).Format(time.RFC3339)})
	if event.LastAt > 0 {
		rows = append(rows, [2]string{"最近一次采样失败 / Latest sampled failure", last.In(location).Format(time.RFC3339)})
	}
	rows = append(rows, [2]string{"确认 / Confirmed", confirmed.In(location).Format(time.RFC3339)})
	if event.Node != "" {
		rows = append(rows, [2]string{"节点 / Gateway", event.Node})
	}
	if event.KeySlot >= 0 {
		rows = append(rows, [2]string{"凭据槽位 / Credential slot", strconv.Itoa(event.KeySlot + 1)})
	}
	if event.StatusCode > 0 {
		status := strings.TrimSpace(strconv.Itoa(event.StatusCode) + " " + http.StatusText(event.StatusCode))
		rows = append(rows, [2]string{"最近样本的上游 HTTP / Latest sample upstream HTTP", status})
	}
	if detail := channelAlertErrorReason(event.LastErrorReason).mailLabel(); detail != "" {
		rows = append(rows, [2]string{"最近错误详情 / Latest error detail", detail})
	}
	if event.LastRequestID != "" {
		rows = append(rows, [2]string{"最近失败的网关请求 ID / Latest failed gateway request ID", event.LastRequestID})
	}
	var body strings.Builder
	writeOperationsMailRows(&body, rows)
	body.WriteString(event.MailEvidenceHTML)

	address, err := url.Parse(strings.TrimSpace(system_setting.ServerAddress))
	if err == nil && (address.Scheme == "http" || address.Scheme == "https") && address.Host != "" && address.User == nil {
		basePath := strings.TrimRight(address.Path, "/")
		address.Path = basePath + "/channels"
		address.RawPath, address.RawQuery, address.Fragment, address.ForceQuery = "", "", "", false
		fmt.Fprintf(&body, `<p><a href="%s" style="color:#075b68;font-weight:600">管理后台 → 渠道 / Admin → Channels</a></p>`, html.EscapeString(address.String()))
		address.Path = basePath + "/usage-logs/error"
		query := url.Values{
			"channel":         {strconv.Itoa(event.ChannelID)},
			"start_timestamp": {strconv.FormatInt(first.Unix(), 10)},
			"end_timestamp":   {strconv.FormatInt(confirmed.Add(channelMailLogGrace).Unix(), 10)},
		}
		if event.Model != "" && common.SanitizeTaskDiagnostic(event.Model) == event.Model {
			if encoded, err := common.Marshal(event.Model); err == nil {
				query.Set("model_name", string(encoded))
			}
		}
		address.RawQuery = query.Encode()
		writeReportMailLink(&body, address.String(), "查看告警附近的错误日志 / Open error logs around this incident")
	} else {
		body.WriteString("<p>在管理后台的错误日志中按渠道 ID 和告警时间筛选。 / In the admin console, filter Error Logs by channel ID and incident time.</p>")
	}
	return subject, RenderMailTemplate(subject, body.String())
}

func ChannelAlertHealth() map[string]any {
	return map[string]any{
		"enabled": channelAlerts.enabled.Load(), "dropped": channelAlerts.stats.Dropped.Load(), "window_resets": channelAlerts.stats.Resets.Load(),
		"confirmed": channelAlerts.stats.Confirmed.Load(), "first_submissions": channelAlerts.stats.Submitted.Load(), "late_first_submissions": channelAlerts.stats.Late.Load(), "overdue_without_submission": channelAlerts.stats.Over60.Load(),
	}
}
