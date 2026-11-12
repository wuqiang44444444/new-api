package service

import (
	"context"
	"errors"
	"fmt"
	"html"
	"net/url"
	"strings"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/i18n"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/setting/system_setting"
)

func DeliverPasswordReset(email, fallbackLanguage string) {
	user, err := model.GetUniqueUserByEmail(email)
	if err != nil {
		if !errors.Is(err, model.ErrEmailNotFound) {
			common.SysError("password reset account lookup could not identify one account")
		}
		return
	}
	if user.Status != common.UserStatusEnabled {
		return
	}
	base, err := url.Parse(strings.TrimRight(system_setting.ServerAddress, "/"))
	if err != nil || base.Host == "" || base.User != nil || (base.Scheme != "https" && base.Scheme != "http") {
		common.SysError("password reset public address is invalid")
		return
	}
	token, err := model.CreatePasswordReset(user)
	if errors.Is(err, model.ErrPasswordResetRecent) {
		return
	}
	if err != nil {
		common.SysError("password reset link could not be created")
		return
	}
	base.Path = strings.TrimRight(base.Path, "/") + "/user/reset"
	base.RawPath, base.RawQuery = "", ""
	base.Fragment = "token=" + token
	lang := user.GetSetting().Language
	if lang == "" {
		lang = fallbackLanguage
	}
	site := map[string]any{"SystemName": common.SystemName}
	link := map[string]any{"Link": html.EscapeString(base.String())}
	subject := i18n.Translate(lang, i18n.MsgEmailPasswordResetSubject, site)
	content := fmt.Sprintf("<p>%s</p><p>%s</p><p>%s</p><p>%s</p>",
		i18n.Translate(lang, i18n.MsgEmailPasswordResetGreeting, map[string]any{"SystemName": html.EscapeString(common.SystemName)}),
		i18n.Translate(lang, i18n.MsgEmailPasswordResetLink, link),
		i18n.Translate(lang, i18n.MsgEmailPasswordResetLinkFallback, link),
		i18n.Translate(lang, i18n.MsgEmailPasswordResetValidity, map[string]any{"Minutes": int(model.PasswordResetLifetime / time.Minute)}))
	for range 2 {
		ctx, cancel := context.WithTimeout(context.Background(), common.ShortMailSubmissionTimeout)
		err = common.SendAccountEmail(ctx, subject, email, content)
		cancel()
		if !errors.Is(err, common.ErrMailBusy) {
			break
		}
	}
	if err != nil {
		if !errors.Is(err, common.ErrMailSubmissionUnknown) {
			if discardErr := model.DiscardUnsentPasswordReset(token); discardErr != nil {
				common.SysError("unsent password reset link could not be discarded")
			}
		}
		common.SysError("password reset mail could not be submitted")
	}
}
