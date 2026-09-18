package operation_setting

import (
	"fmt"
	"net/mail"
	"strings"

	"github.com/QuantumNous/new-api/setting/config"
)

// ErrorReportSetting 控制「每小时系统运行与错误邮件报告」
// （docs/80-dev/2026-09-17-每小时系统运行与错误邮件报告开发方案.md）。
// 复用现有 SMTP 配置；频率固定为每小时，时区固定 Asia/Shanghai，
// 不扩展成通用计划任务编辑器。通过 config.GlobalConfig 注册后自动
// 接入 GET/PUT /api/option，键名为 error_report_setting.<field>。
type ErrorReportSetting struct {
	Enabled bool `json:"enabled"`
	// Recipients 为分号/逗号/空白分隔的邮箱列表；保存前须经
	// ValidateErrorReportRecipients 校验，运行时经 ParseErrorReportRecipients 规范化。
	Recipients string `json:"recipients"`
}

// 默认配置：关闭且无收件人，管理员按需开启。
var errorReportSetting = ErrorReportSetting{
	Enabled:    false,
	Recipients: "",
}

func init() {
	config.GlobalConfig.Register("error_report_setting", &errorReportSetting)
}

func GetErrorReportSetting() *ErrorReportSetting {
	return &errorReportSetting
}

// ParseErrorReportRecipients 把原始字符串按分隔符拆分、去空白并去重；
// 空输入返回空列表。拆分规则是运行时使用，不改变邮箱本地部分。
func ParseErrorReportRecipients(raw string) []string {
	fields := strings.FieldsFunc(raw, func(r rune) bool {
		switch r {
		case ';', ',', '\n', '\r', '\t', ' ':
			return true
		}
		return false
	})
	seen := make(map[string]bool, len(fields))
	out := make([]string, 0, len(fields))
	for _, field := range fields {
		field = strings.TrimSpace(field)
		if field == "" || seen[field] {
			continue
		}
		seen[field] = true
		out = append(out, field)
	}
	return out
}

// ValidateErrorReportRecipients 拒绝 CR/LF 注入与格式非法的地址，
// 返回规范化后的收件人列表；raw 允许为空（表示暂未配置）。
func ValidateErrorReportRecipients(raw string) ([]string, error) {
	if strings.ContainsAny(raw, "\r\n") {
		return nil, fmt.Errorf("收件人列表不允许包含换行符")
	}
	list := ParseErrorReportRecipients(raw)
	for _, addr := range list {
		parsed, err := mail.ParseAddress(addr)
		if err != nil || parsed.Address != addr {
			return nil, fmt.Errorf("收件人邮箱格式非法: %s", addr)
		}
	}
	return list, nil
}
