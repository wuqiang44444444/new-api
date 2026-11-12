package service

import (
	"fmt"
	"html"
	"strings"

	"github.com/QuantumNous/new-api/common"
)

func renderEmailBindingCode(code string, currentAddress bool) (string, string) {
	subject := common.SystemName + " — 邮箱确认 / Confirm your email address"
	zh, en := "请确认将此邮箱绑定到你的账户。", "Confirm linking this email address to your account."
	if currentAddress {
		zh, en = "有人申请更换你当前绑定的邮箱。请确认是否继续。", "A request was made to replace your current account email address."
	}
	return subject, fmt.Sprintf("<h2>邮箱确认 / Email confirmation</h2><p>%s</p><p>%s</p><p>验证码 / Code: <strong>%s</strong></p><p>验证码 10 分钟内有效。如非本人操作，请勿分享。</p><p>The code expires in 10 minutes. If you did not request this change, keep the code private.</p>", zh, en, html.EscapeString(code))
}

func renderAccountSecurityMail(event string) (string, string) {
	zh := map[string]string{
		"Password updated":                             "密码已更新",
		"Login account unlinked":                       "登录方式已解绑",
		"WeChat account linked":                        "微信账户已绑定",
		"Email address changed":                        "绑定邮箱已更换",
		"Email address confirmed":                      "绑定邮箱已确认",
		"A change of your email address was requested": "已申请更换绑定邮箱",
	}[event]
	if provider, ok := strings.CutPrefix(event, "Login account linked: "); ok {
		zh = "已绑定登录方式 " + provider
	}
	if zh == "" {
		zh = "账户安全设置已更改"
	}
	return common.SystemName + " — 账户安全通知 / Account security notification",
		fmt.Sprintf("<h2>账户安全通知 / Account security notification</h2><p>%s</p><p>%s</p><p>如非本人操作，请撤销其他登录会话并联系管理员。</p><p>If you did not make this change, revoke other login sessions and contact your administrator.</p>", html.EscapeString(zh), html.EscapeString(event))
}
