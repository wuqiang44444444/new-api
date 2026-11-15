package service

import (
	"fmt"
	"html"
	"strings"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/i18n"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/relaykit/dto"
)

// NotifyAccountCreated fires a welcome email when a user account is created.
// Toggle-gated, async fire-and-forget; failures are logged but never block the caller.
func NotifyAccountCreated(userId int) {
	queuedAt := time.Now()
	if !EnqueueMailWork(func() {
		sendLifecycleEmail(model.MailWelcome, queuedAt, userId, dto.NotifyTypeAccountCreated,
			i18n.MsgMailAccountCreatedSubject, i18n.MsgMailAccountCreatedBody, "")
	}) {
		common.SysError("lifecycle notification queue is full")
	}
}

// NotifyTokenCreated fires an email when a new API token is created by the user.
func NotifyTokenCreated(userId int) {
	queuedAt := time.Now()
	if !EnqueueMailWork(func() {
		sendLifecycleEmail(model.MailTokenCreated, queuedAt, userId, dto.NotifyTypeTokenCreated,
			i18n.MsgMailTokenCreatedSubject, i18n.MsgMailTokenCreatedBody, "")
	}) {
		common.SysError("lifecycle notification queue is full")
	}
}

// sendLifecycleEmail resolves the user + language, renders the bilingual subject/body
// via i18n, and dispatches through notifyUser (which handles address resolution,
// NotificationEmail override, per-user delivery channel and rate limiting).
func sendLifecycleEmail(kind model.MailType, queuedAt time.Time, userId int, notifyType, subjectKey, bodyKey, detailHTML string) {
	user, err := model.GetUserById(userId, false)
	if err != nil || user == nil {
		common.SysError(fmt.Sprintf("lifecycle notify: load user %d failed: %v", userId, err))
		return
	}
	data := map[string]any{
		"Username": user.DisplayName,
		"SiteName": common.SystemName,
	}
	settings := user.GetSetting()
	lang := settings.Language
	subject := i18n.Translate(lang, subjectKey, data)
	data["Username"] = html.EscapeString(user.DisplayName)
	data["SiteName"] = html.EscapeString(common.SystemName)
	var body strings.Builder
	body.WriteString(i18n.Translate(lang, bodyKey, data))
	if settings.NotifyType == "" || settings.NotifyType == dto.NotifyTypeEmail {
		body.WriteString(detailHTML)
		var path string
		switch kind {
		case model.MailWelcome:
			path = "/security"
		case model.MailTokenCreated:
			path = "/keys"
		case model.MailVideoFunds:
			path = "/system-info"
		}
		if path != "" {
			if link := consoleMailLink(path); link != "" {
				writeReportMailLink(&body, link, i18n.Translate(lang, i18n.MsgMailOpenConsole))
			}
		}
	}
	notify := dto.NewNotify(notifyType, subject, body.String(), nil)
	if err := notifyUser(kind, queuedAt, userId, user.Email, settings, notify, false); err != nil {
		common.SysError(fmt.Sprintf("failed to send %s notify to user %d: %s", notifyType, userId, err.Error()))
	}
}
