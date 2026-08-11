package operation_setting

import (
	"github.com/QuantumNous/new-api/setting/config"
)

// NotificationSetting controls the per-event transactional email toggles for
// user-lifecycle events. Registered via config.GlobalConfig (Pattern b), so it
// is auto-wired into GET/PUT /api/option as notification_setting.<field>.
type NotificationSetting struct {
	SendEmailOnAccountCreated  bool `json:"send_email_on_account_created"`
	SendEmailOnPasswordChanged bool `json:"send_email_on_password_changed"`
	SendEmailOnTokenCreated    bool `json:"send_email_on_token_created"`
}

// 默认配置：全部关闭，管理员按需开启
var notificationSetting = NotificationSetting{
	SendEmailOnAccountCreated:  false,
	SendEmailOnPasswordChanged: false,
	SendEmailOnTokenCreated:    false,
}

func init() {
	// 注册到全局配置管理器
	config.GlobalConfig.Register("notification_setting", &notificationSetting)
}

func GetNotificationSetting() *NotificationSetting {
	return &notificationSetting
}
