package service

import (
	"fmt"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/i18n"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/relaykit/dto"
	"github.com/QuantumNous/new-api/setting/operation_setting"
	"github.com/bytedance/gopkg/util/gopool"
)

// NotifyAccountCreated fires a welcome email when a user account is created.
// Toggle-gated, async fire-and-forget; failures are logged but never block the caller.
func NotifyAccountCreated(userId int) {
	if !operation_setting.GetNotificationSetting().SendEmailOnAccountCreated {
		return
	}
	gopool.Go(func() {
		sendLifecycleEmail(userId, dto.NotifyTypeAccountCreated,
			"notify.account_created.subject", "notify.account_created.body")
	})
}

// NotifyPasswordChanged fires a security email when a user's password is changed.
func NotifyPasswordChanged(userId int) {
	if !operation_setting.GetNotificationSetting().SendEmailOnPasswordChanged {
		return
	}
	gopool.Go(func() {
		sendLifecycleEmail(userId, dto.NotifyTypePasswordChanged,
			"notify.password_changed.subject", "notify.password_changed.body")
	})
}

// NotifyTokenCreated fires an email when a new API token is created by the user.
func NotifyTokenCreated(userId int) {
	if !operation_setting.GetNotificationSetting().SendEmailOnTokenCreated {
		return
	}
	gopool.Go(func() {
		sendLifecycleEmail(userId, dto.NotifyTypeTokenCreated,
			"notify.token_created.subject", "notify.token_created.body")
	})
}

// sendLifecycleEmail resolves the user + language, renders the bilingual subject/body
// via i18n, and dispatches through NotifyUser (which handles address resolution,
// NotificationEmail override, per-user delivery channel and rate limiting).
func sendLifecycleEmail(userId int, notifyType, subjectKey, bodyKey string) {
	user, err := model.GetUserById(userId, false)
	if err != nil || user == nil {
		common.SysError(fmt.Sprintf("lifecycle notify: load user %d failed: %v", userId, err))
		return
	}
	data := map[string]any{
		"Username": user.DisplayName,
		"SiteName": common.SystemName,
	}
	lang := user.GetSetting().Language
	subject := i18n.Translate(lang, subjectKey, data)
	body := i18n.Translate(lang, bodyKey, data)
	notify := dto.NewNotify(notifyType, subject, body, nil)
	if err := NotifyUser(userId, user.Email, user.GetSetting(), notify); err != nil {
		common.SysError(fmt.Sprintf("failed to send %s notify to user %d: %s", notifyType, userId, err.Error()))
	}
}
