package controller

import (
	"net/http"
	"strings"

	"github.com/QuantumNous/new-api/setting/system_setting"

	"github.com/gin-gonic/gin"
)

// incomingVideoBaseURL 推导调用方本次请求实际访问本服务所用的 base URL（scheme://host[:port]），
// 无法从请求确定时回退到系统配置的 ServerAddress。
//
// 返回给客户端的视频内容 URL（如 /v1/videos/<id>/content）以此为前缀，使其主机部分始终跟随
// 调用方入站请求：线上经公网域名访问自动得到公网域名，本地经 127.0.0.1 访问自动得到本地地址，
// 无需在不同环境间切换 ServerAddress 配置。
//
// 安全说明：返回值仅用于客户端回访本服务自身的端点，不会被本服务用作回源地址，因此即便请求
// Host 头被伪造也不会引入 SSRF 风险；最坏情况是客户端拿到一个不可达的视频 URL。
func incomingVideoBaseURL(c *gin.Context) string {
	if c != nil && c.Request != nil {
		host := strings.TrimSpace(c.GetHeader("X-Forwarded-Host"))
		if host == "" {
			host = strings.TrimSpace(c.Request.Host)
		}
		if host != "" {
			return requestScheme(c.Request) + "://" + host
		}
	}
	return strings.TrimRight(system_setting.ServerAddress, "/")
}

// requestScheme 从请求推导协议（http/https），优先信任反向代理转发的 X-Forwarded-Proto。
func requestScheme(r *http.Request) string {
	if proto := strings.TrimSpace(r.Header.Get("X-Forwarded-Proto")); proto != "" {
		return strings.ToLower(strings.TrimSpace(strings.Split(proto, ",")[0]))
	}
	if r.TLS != nil {
		return "https"
	}
	return "http"
}
