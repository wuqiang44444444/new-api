package service

import (
	"sync/atomic"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/i18n"
	"github.com/QuantumNous/new-api/logger"
	"github.com/QuantumNous/new-api/model"
	relaycommon "github.com/QuantumNous/new-api/relay/common"
	"github.com/QuantumNous/new-api/relaykit/dto"
	"github.com/bytedance/gopkg/util/gopool"
)

var quotaMailWork = mailWorkQueue{jobs: make(chan func(), 16), workers: 1}
var quotaMailDropped atomic.Uint64

func checkAndSendQuotaNotify(relayInfo *relaycommon.RelayInfo, quota, preConsumedQuota int) {
	remaining := relayInfo.UserQuota - (quota + preConsumedQuota)
	threshold := common.EffectiveQuotaWarningThreshold(relayInfo.UserSetting.QuotaWarningThreshold)
	if remaining >= threshold {
		return
	}
	if relayInfo.UserSetting.NotifyType == "" || relayInfo.UserSetting.NotifyType == dto.NotifyTypeEmail {
		var pendingBalance *int
		if common.BatchUpdateEnabled && !common.RedisEnabled {
			pendingBalance = &remaining
		}
		enqueueWalletNotice(relayInfo.UserId, false, pendingBalance)
		return
	}
	enqueueQuotaNotification(model.MailWalletLow, relayInfo.UserId, relayInfo.UserEmail, relayInfo.UserSetting, i18n.MsgNotifyQuotaLowTitle, remaining, threshold)
}

func checkAndSendSubscriptionQuotaNotify(relayInfo *relaycommon.RelayInfo) {
	if relayInfo == nil || relayInfo.SubscriptionId == 0 || relayInfo.SubscriptionAmountTotal <= 0 {
		return
	}
	remaining := relayInfo.SubscriptionAmountTotal - (relayInfo.SubscriptionAmountUsedAfterPreConsume + relayInfo.SubscriptionPostDelta)
	threshold := common.EffectiveQuotaWarningThreshold(relayInfo.UserSetting.QuotaWarningThreshold)
	if remaining >= int64(threshold) {
		return
	}
	enqueueQuotaNotification(model.MailSubscriptionLow, relayInfo.UserId, relayInfo.UserEmail, relayInfo.UserSetting, i18n.MsgNotifySubscriptionQuotaLowTitle, int(remaining), threshold)
}

func enqueueQuotaNotification(kind model.MailType, userID int, email string, setting dto.UserSetting, titleKey string, remaining, threshold int) {
	queuedAt := time.Now()
	send := func() {
		if titleKey == i18n.MsgNotifyQuotaLowTitle && (!common.BatchUpdateEnabled || common.RedisEnabled) {
			// Legacy wallet deductions can be waiting for the batch writer.
			currentQuota, err := model.GetUserQuota(userID, !common.BatchUpdateEnabled)
			if err != nil {
				common.SysError(common.LogText("failed to read wallet balance for quota notification (user %d)", userID))
				return
			}
			if currentQuota >= threshold {
				return
			}
			remaining = currentQuota
		}
		lang := setting.Language
		prompt := i18n.Translate(lang, titleKey)
		balance, warning := logger.FormatQuota(remaining), logger.FormatQuota(threshold)
		content, values := quotaNotifyContent(lang, setting.NotifyType, prompt, balance, warning)
		if kind == model.MailSubscriptionLow && (setting.NotifyType == "" || setting.NotifyType == dto.NotifyTypeEmail) {
			content, values = renderSubscriptionQuotaMail(balance, warning, queuedAt), nil
		}
		if err := notifyUser(kind, queuedAt, userID, email, setting, dto.NewNotify(dto.NotifyTypeQuotaExceed, prompt, content, values), false); err != nil {
			common.SysError(common.LogText("failed to send quota notify to user %d: %s", userID, err.Error()))
		}
	}
	if setting.NotifyType != "" && setting.NotifyType != dto.NotifyTypeEmail {
		gopool.Go(send)
		return
	}
	if !quotaMailWork.enqueue(func() {
		logDroppedQuotaMail()
		send()
	}) {
		quotaMailDropped.Add(1)
	}
}

func logDroppedQuotaMail() {
	if dropped := quotaMailDropped.Swap(0); dropped != 0 {
		common.SysError(common.LogText("quota mail queue full, skipped %d notifications", dropped))
	}
}

// quotaNotifyContent renders a low-quota notice in the user's saved language.
// Bark and Gotify get plain text; email and webhook get HTML with the top-up
// link. values stay in the notify payload for webhook receivers.
func quotaNotifyContent(lang string, notifyType string, prompt string, remaining, threshold string) (string, []any) {
	switch notifyType {
	case dto.NotifyTypeBark:
		// Bark推送使用简短文本，不支持HTML
		return i18n.Translate(lang, i18n.MsgNotifyQuotaLowBark, map[string]any{"Prompt": prompt, "Quota": remaining}), []any{prompt, remaining}
	case dto.NotifyTypeGotify:
		return i18n.Translate(lang, i18n.MsgNotifyQuotaLowGotify, map[string]any{"Prompt": prompt, "Quota": remaining}), []any{prompt, remaining}
	default:
		topUpLink := PaymentReturnURL("/wallet")
		return i18n.Translate(lang, i18n.MsgNotifyQuotaLowEmail, map[string]any{"Prompt": prompt, "Quota": remaining, "Threshold": threshold, "Link": topUpLink}), []any{prompt, remaining, topUpLink, topUpLink}
	}
}
