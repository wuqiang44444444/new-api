package service

import (
	"sync/atomic"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/i18n"
	"github.com/QuantumNous/new-api/logger"
	relaycommon "github.com/QuantumNous/new-api/relay/common"
	"github.com/QuantumNous/new-api/relaykit/dto"
	"github.com/bytedance/gopkg/util/gopool"
)

var quotaMailWork = mailWorkQueue{jobs: make(chan func(), 16), workers: 1}
var quotaMailDropped atomic.Uint64

func checkAndSendQuotaNotify(relayInfo *relaycommon.RelayInfo, quota, preConsumedQuota int) {
	threshold := quotaNotifyThreshold(relayInfo.UserSetting)
	if relayInfo.UserQuota-(quota+preConsumedQuota) >= threshold {
		return
	}
	enqueueQuotaNotification(relayInfo.UserId, relayInfo.UserEmail, relayInfo.UserSetting, i18n.MsgNotifyQuotaLowTitle, relayInfo.UserQuota)
}

func checkAndSendSubscriptionQuotaNotify(relayInfo *relaycommon.RelayInfo) {
	if relayInfo == nil || relayInfo.SubscriptionId == 0 || relayInfo.SubscriptionAmountTotal <= 0 {
		return
	}
	remaining := relayInfo.SubscriptionAmountTotal - (relayInfo.SubscriptionAmountUsedAfterPreConsume + relayInfo.SubscriptionPostDelta)
	if remaining >= int64(quotaNotifyThreshold(relayInfo.UserSetting)) {
		return
	}
	enqueueQuotaNotification(relayInfo.UserId, relayInfo.UserEmail, relayInfo.UserSetting, i18n.MsgNotifySubscriptionQuotaLowTitle, int(remaining))
}

func quotaNotifyThreshold(setting dto.UserSetting) int {
	if setting.QuotaWarningThreshold != 0 {
		return int(setting.QuotaWarningThreshold)
	}
	return common.QuotaRemindThreshold
}

func enqueueQuotaNotification(userID int, email string, setting dto.UserSetting, titleKey string, remaining int) {
	send := func() {
		lang := setting.Language
		prompt := i18n.Translate(lang, titleKey)
		content, values := quotaNotifyContent(lang, setting.NotifyType, prompt, logger.FormatQuota(remaining))
		if err := NotifyUser(userID, email, setting, dto.NewNotify(dto.NotifyTypeQuotaExceed, prompt, content, values)); err != nil {
			common.SysError(common.LogText("failed to send quota notify to user %d: %s", userID, err.Error()))
		}
	}
	if setting.NotifyType != "" && setting.NotifyType != dto.NotifyTypeEmail {
		gopool.Go(send)
		return
	}
	if !quotaMailWork.enqueue(func() {
		if dropped := quotaMailDropped.Swap(0); dropped != 0 {
			common.SysError(common.LogText("quota mail queue full, skipped %d notifications", dropped))
		}
		send()
	}) {
		quotaMailDropped.Add(1)
	}
}

// quotaNotifyContent renders a low-quota notice in the user's saved language.
// Bark and Gotify get plain text; email and webhook get HTML with the top-up
// link. values stay in the notify payload for webhook receivers.
func quotaNotifyContent(lang string, notifyType string, prompt string, remaining string) (string, []any) {
	switch notifyType {
	case dto.NotifyTypeBark:
		// Bark推送使用简短文本，不支持HTML
		return i18n.Translate(lang, i18n.MsgNotifyQuotaLowBark, map[string]any{"Prompt": prompt, "Quota": remaining}), []any{prompt, remaining}
	case dto.NotifyTypeGotify:
		return i18n.Translate(lang, i18n.MsgNotifyQuotaLowGotify, map[string]any{"Prompt": prompt, "Quota": remaining}), []any{prompt, remaining}
	default:
		topUpLink := PaymentReturnURL("/wallet")
		return i18n.Translate(lang, i18n.MsgNotifyQuotaLowEmail, map[string]any{"Prompt": prompt, "Quota": remaining, "Link": topUpLink}), []any{prompt, remaining, topUpLink, topUpLink}
	}
}
