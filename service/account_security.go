package service

import (
	"strings"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/oauth"
	"github.com/QuantumNous/new-api/setting/system_setting"
)

func UnbindAccountOAuth(identity AuthIdentity, providerID int) error {
	enabled := model.AccountLoginMethods{
		Password: common.PasswordLoginEnabled,
		Passkey:  system_setting.PasskeySettingsSnapshot().Enabled,
		WeChat:   common.WeChatAuthEnabled,
	}
	for _, provider := range oauth.GetAllProviders() {
		if !provider.IsEnabled() {
			continue
		}
		if custom, ok := provider.(*oauth.GenericOAuthProvider); ok {
			enabled.CustomProviderIDs = append(enabled.CustomProviderIDs, custom.GetProviderId())
		} else {
			enabled.OAuthColumns = append(enabled.OAuthColumns, provider.ProviderUserIDColumn())
		}
	}
	return model.UnbindUserOAuthForSession(identity, providerID, enabled)
}

// NotifyAccountSecurityChange never includes credentials or tokens. The caller
// records delivery failure independently from the already-committed change.
func NotifyAccountSecurityChange(email, event string) error {
	if email == "" {
		return nil
	}
	queuedAt := time.Now()
	subject, content := renderAccountSecurityMail(event)
	var body strings.Builder
	body.WriteString(content)
	writeOperationsMailRows(&body, [][2]string{{"通知时间（北京时间） / Notified at (Beijing time)", queuedAt.In(errorReportLocation()).Format("2006-01-02 15:04:05")}})
	if link := consoleMailLink("/security"); link != "" {
		writeReportMailLink(&body, link, "检查账户安全 / Review account security")
	}
	return sendManagedNotice(model.MailAccountSecurity, queuedAt, subject, email, RenderMailTemplate(subject, body.String()))
}
